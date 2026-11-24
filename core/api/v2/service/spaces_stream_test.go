package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fakeKeyWatch is the KeyWatch the server would hand the stream. A test
// edits the key (the next Current answers the new state) or fails it, and
// the signal is the one RevokeToken sends.
type fakeKeyWatch struct {
	state   KeyState
	changed chan struct{}
	closing chan struct{}

	mu    sync.Mutex
	next  KeyState
	err   error
	reads int
	edits int
}

func newFakeKeyWatch(state KeyState) *fakeKeyWatch {
	return &fakeKeyWatch{state: state, next: state, changed: make(chan struct{}, 1), closing: make(chan struct{})}
}

func (w *fakeKeyWatch) State() KeyState          { return w.state }
func (w *fakeKeyWatch) Changed() <-chan struct{} { return w.changed }
func (w *fakeKeyWatch) Closing() <-chan struct{} { return w.closing }
func (w *fakeKeyWatch) Close()                   {}

func (w *fakeKeyWatch) Current(context.Context) (KeyState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reads++
	return w.next, w.err
}

func (w *fakeKeyWatch) signal() {
	select {
	case w.changed <- struct{}{}:
	default:
	}
}

// edit is a grant edit: the key now reads as state, and its sweep signals.
func (w *fakeKeyWatch) edit(state KeyState) {
	w.mu.Lock()
	w.next = state
	w.edits++
	w.mu.Unlock()
	w.signal()
}

// fail makes the next re-read fail, and signals.
func (w *fakeKeyWatch) fail(err error) {
	w.mu.Lock()
	w.err = err
	w.edits++
	w.mu.Unlock()
	w.signal()
}

// awaitReads waits until every edit so far was followed by a re-read. The
// stream's worker picks among its inputs at random, so a test that feeds the
// subscription right after an edit waits here first to keep the two in order.
func (w *fakeKeyWatch) awaitReads(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w.mu.Lock()
		done := w.reads >= w.edits
		w.mu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the stream did not re-read its key after the change signal")
		}
		time.Sleep(time.Millisecond)
	}
}

func (w *fakeKeyWatch) readCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reads
}

// fakeStreamClock is the stream's clock, driven by the test: Now is what the
// test set, and a timer fires only when the test fires it — as a monotonic
// timer does not fire while the machine sleeps.
type fakeStreamClock struct {
	mu     sync.Mutex
	now    time.Time
	timers chan fakeTimer
}

type fakeTimer struct {
	fire chan time.Time
	wait time.Duration
}

func newFakeStreamClock(now time.Time) *fakeStreamClock {
	return &fakeStreamClock{now: now, timers: make(chan fakeTimer, 64)}
}

func (c *fakeStreamClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeStreamClock) set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func (c *fakeStreamClock) After(d time.Duration) (<-chan time.Time, func()) {
	timer := fakeTimer{fire: make(chan time.Time, 1), wait: d}
	c.timers <- timer
	return timer.fire, func() {}
}

// armed waits for the worker to arm its next timer, which it does at the
// start of every turn: once it returns, the previous turn is complete.
func (c *fakeStreamClock) armed(t *testing.T) fakeTimer {
	t.Helper()
	select {
	case timer := <-c.timers:
		return timer
	case <-time.After(5 * time.Second):
		t.Fatal("the stream armed no expiry timer")
		return fakeTimer{}
	}
}

// Shaped space ids (the production form), two of which share a tail: a
// stream that served short references would differ from these on every one.
const (
	streamSpaceA   = realSpaceTracker
	streamSpaceB   = realSpacePersonal
	streamSpaceC   = twinPersonal // shares realSpacePersonal's tail
	streamSpaceD   = realSpaceEval
	streamSentinel = realSpaceEngineering
)

// spaceViewOf builds a space view as the subscription carries it. Statuses
// are set only when given, so a legacy view carries none.
func spaceViewOf(viewId, target, name string, statuses ...model.SpaceStatus) *domain.Details {
	view := domain.NewDetails()
	view.SetString(bundle.RelationKeyId, viewId)
	view.SetString(bundle.RelationKeyTargetSpaceId, target)
	view.SetString(bundle.RelationKeyName, name)
	keys := []domain.RelationKey{bundle.RelationKeySpaceLocalStatus, bundle.RelationKeySpaceAccountStatus, bundle.RelationKeySpaceRemoteStatus}
	for i, status := range statuses {
		view.SetInt64(keys[i], int64(status))
	}
	return view
}

func viewAdded(view *domain.Details) apicore.ObjectSearchChange {
	return apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: view.GetString(bundle.RelationKeyId), Details: view}
}

func viewUpdated(view *domain.Details) apicore.ObjectSearchChange {
	return apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: view.GetString(bundle.RelationKeyId), Details: view}
}

func viewRemoved(viewId string) apicore.ObjectSearchChange {
	return apicore.ObjectSearchChange{Type: apicore.ObjectSearchRemoved, Id: viewId}
}

// spacesStreamFixture is one open spaces stream over a fake subscription and
// a fake key watch. A sentinel space rides along: renaming it is a change
// the stream must emit, so "nothing was emitted for X" is asserted as "after
// X and a sentinel rename, only the rename came out" — with a deadline, and
// without sleeping.
type spacesStreamFixture struct {
	t        *testing.T
	fx       *v2Fixture
	sub      *fakeObjectSearch
	watch    *fakeKeyWatch
	stream   *SpacesStream
	snapshot []v2model.SpaceStreamEvent
	renames  int
}

const sentinelViewId = "view-sentinel"

func openSpacesStreamFixture(t *testing.T, key KeyState, views ...*domain.Details) *spacesStreamFixture {
	t.Helper()
	fx := newV2FixtureBare(t)
	return openSpacesStreamOn(t, fx, key, views...)
}

func openSpacesStreamOn(t *testing.T, fx *v2Fixture, key KeyState, views ...*domain.Details) *spacesStreamFixture {
	t.Helper()
	snapshot := []apicore.ObjectSearchChange{viewAdded(spaceViewOf(sentinelViewId, streamSentinel, "sentinel 0"))}
	for _, view := range views {
		snapshot = append(snapshot, viewAdded(view))
	}
	snapshot = append(snapshot, apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete})
	sub := newFakeObjectSearch(snapshot...)
	search := mock_apicore.NewMockObjectSearchService(t)
	search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
	fx.withObjectSearch(search)
	watch := newFakeKeyWatch(key)

	// the request context carries a grant that admits everything: a stream
	// that filtered by it, and not by the watch's, shows every space
	ctx := util.CtxWithApiGrant(context.Background(), &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsReadWrite})
	stream, err := fx.OpenSpacesStream(ctx, watch)
	require.NoError(t, err)
	t.Cleanup(stream.Close)
	sf := &spacesStreamFixture{t: t, fx: fx, sub: sub, watch: watch, stream: stream}
	sf.snapshot = decodeSpaceFrames(t, stream.Snapshot())
	return sf
}

func decodeSpaceFrames(t *testing.T, frames []StreamFrame) []v2model.SpaceStreamEvent {
	t.Helper()
	out := make([]v2model.SpaceStreamEvent, 0, len(frames))
	for _, frame := range frames {
		var event v2model.SpaceStreamEvent
		require.NoError(t, json.Unmarshal(frame.Data, &event))
		require.Equal(t, frame.Type, event.Type, "the SSE event name is the body's type")
		out = append(out, event)
	}
	return out
}

// drain waits for queued frames and returns them with the ended flag.
func (sf *spacesStreamFixture) drain() ([]v2model.SpaceStreamEvent, bool) {
	sf.t.Helper()
	select {
	case <-sf.stream.Ready():
	case <-time.After(5 * time.Second):
		sf.t.Fatal("the stream queued nothing")
	}
	frames, ended := sf.stream.Drain()
	return decodeSpaceFrames(sf.t, frames), ended
}

// events returns what the stream emitted since the last call, in order,
// without the sentinel's own event. It renames the sentinel and reads until
// that rename comes out, so everything pushed before the call was processed.
func (sf *spacesStreamFixture) events() []v2model.SpaceStreamEvent {
	sf.t.Helper()
	sf.watch.awaitReads(sf.t)
	sf.renames++
	name := fmt.Sprintf("sentinel %d", sf.renames)
	sf.sub.push(viewUpdated(spaceViewOf(sentinelViewId, streamSentinel, name)))
	var out []v2model.SpaceStreamEvent
	for {
		events, ended := sf.drain()
		for _, event := range events {
			if event.Space != nil && event.Space.Id == streamSentinel {
				if event.Space.Name == name {
					require.Equal(sf.t, v2model.SpaceStreamEventSpaceUpdated, event.Type)
					return out
				}
				continue
			}
			out = append(out, event)
		}
		require.False(sf.t, ended, "the stream ended while events were expected")
	}
}

func added(id, name string) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceAdded, Space: &v2model.SpaceRow{Id: id, Name: name}}
}

func updated(id, name string) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceUpdated, Space: &v2model.SpaceRow{Id: id, Name: name}}
}

func removed(id, reason string) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceRemoved, SpaceId: id, Reason: reason}
}

func notGranted(flag bool) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventNotGrantedUpdated, HasNotGrantedSpaces: &flag}
}

func snapshotComplete(flag bool) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSnapshotComplete, HasNotGrantedSpaces: &flag}
}

const (
	stUnknown = model.SpaceStatus_Unknown
	stLoading = model.SpaceStatus_Loading
	stOk      = model.SpaceStatus_Ok
	stMissing = model.SpaceStatus_Missing
	stActive  = model.SpaceStatus_SpaceActive
	stJoining = model.SpaceStatus_SpaceJoining
	stDeleted = model.SpaceStatus_SpaceDeleted
)

func TestOpenSpacesStream(t *testing.T) {
	t.Run("the snapshot is the visible set in full-id order, then snapshot_complete", func(t *testing.T) {
		// given: views in no particular order; two targets share a tail
		sf := openSpacesStreamFixture(t, KeyState{},
			spaceViewOf("v-c", streamSpaceC, "Twin", stOk, stActive),
			spaceViewOf("v-a", streamSpaceA, "Tracker"),
			spaceViewOf("v-b", streamSpaceB, "Personal", stLoading, stActive),
			spaceViewOf("v-d", streamSpaceD, "Failed", stMissing, stActive),
		)
		// realSpaceTracker < realSpacePersonal < twinPersonal < realSpaceEngineering by id
		want := []v2model.SpaceStreamEvent{
			added(streamSpaceA, "Tracker"),
			added(streamSpaceB, "Personal"),
			added(streamSpaceC, "Twin"),
			added(streamSentinel, "sentinel 0"),
			snapshotComplete(false),
		}

		// then
		assert.Equal(t, want, sf.snapshot)
		require.True(t, streamSpaceA < streamSpaceB && streamSpaceB < streamSpaceC && streamSpaceC < streamSentinel,
			"the fixture's ids must be in the order the expectation spells")
	})

	t.Run("snapshot_complete carries the flag as false for an empty set too", func(t *testing.T) {
		// given: no space view at all
		fx := newV2FixtureBare(t)
		sub := newFakeObjectSearch()
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
		fx.withObjectSearch(search)

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
		require.NoError(t, err)
		defer stream.Close()

		// then
		frames := stream.Snapshot()
		require.Len(t, frames, 1)
		assert.Equal(t, v2model.SpaceStreamEventSnapshotComplete, frames[0].Type)
		assert.JSONEq(t, `{"type":"snapshot_complete","has_not_granted_spaces":false}`, string(frames[0].Data))
	})

	t.Run("a grant that names no visible space gets an empty snapshot that says spaces are hidden", func(t *testing.T) {
		// given: one live view, and a grant that names another space
		fx := newV2FixtureBare(t)
		sub := newFakeObjectSearch(
			viewAdded(spaceViewOf("v-a", streamSpaceA, "Not granted", stOk, stActive)),
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete},
		)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
		fx.withObjectSearch(search)
		grant := &util.ApiGrant{Spaces: []string{streamSpaceB}, Perms: util.GrantPermsRead}

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{Grant: grant}))
		require.NoError(t, err)
		defer stream.Close()

		// then: no space_added, and the flag is true
		assert.Equal(t, []v2model.SpaceStreamEvent{snapshotComplete(true)}, decodeSpaceFrames(t, stream.Snapshot()))
	})

	t.Run("a grant edit signaled before the snapshot is in the snapshot", func(t *testing.T) {
		// given: the watch registered with {A, B}; an edit to {A} landed while
		// the subscription was opening, so its signal is already pending
		fx := newV2FixtureBare(t)
		sub := newFakeObjectSearch(
			viewAdded(spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive)),
			viewAdded(spaceViewOf("v-b", streamSpaceB, "B", stOk, stActive)),
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete},
		)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
		fx.withObjectSearch(search)
		watch := newFakeKeyWatch(KeyState{Grant: &util.ApiGrant{Spaces: []string{streamSpaceA, streamSpaceB}, Perms: util.GrantPermsRead}})
		watch.edit(KeyState{Grant: &util.ApiGrant{Spaces: []string{streamSpaceA}, Perms: util.GrantPermsRead}})

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), watch)
		require.NoError(t, err)
		defer stream.Close()

		// then: B was never sent, so nothing has to remove it
		want := []v2model.SpaceStreamEvent{added(streamSpaceA, "A"), snapshotComplete(true)}
		assert.Equal(t, want, decodeSpaceFrames(t, stream.Snapshot()))
		assert.Equal(t, 1, watch.readCount())
	})

	t.Run("a key found gone before the snapshot is refused, and nothing is held", func(t *testing.T) {
		for _, gone := range []error{ErrKeyRevoked, ErrKeyExpired, ErrKeyUnverified} {
			// given: the revocation's signal is pending when the stream opens
			fx := newV2FixtureBare(t)
			_, subs := fx.openingSearch(t)
			watch := newFakeKeyWatch(KeyState{})
			watch.fail(fmt.Errorf("mint: %w", gone))

			// when
			stream, err := fx.OpenSpacesStream(context.Background(), watch)

			// then
			require.ErrorIs(t, err, gone)
			assert.Nil(t, stream)
			require.Len(t, *subs, 1)
			assert.Equal(t, 1, (*subs)[0].closeCount(), "the subscription is closed")
			assert.Equal(t, 0, fx.spacesStreams.open, "the slot is returned")
		}
	})

	t.Run("with no signal pending the key is not re-read", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.openingSearch(t)
		watch := newFakeKeyWatch(KeyState{})

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), watch)
		require.NoError(t, err)
		defer stream.Close()

		// then
		assert.Equal(t, 0, watch.readCount())
	})

	t.Run("it subscribes to the list's own query on the tech space", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		opens, _ := fx.openingSearch(t)

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
		require.NoError(t, err)
		defer stream.Close()

		// then
		require.Len(t, *opens, 1)
		open := (*opens)[0]
		assert.Equal(t, objectstore.TestTechSpaceId, open.SpaceId)
		assert.Equal(t, spaceViewFilters(), open.Filters, "the list's filter and nothing else: no liveness, no grant")
		assert.Empty(t, open.Sorts)
		assert.ElementsMatch(t, []string{
			"id", "targetSpaceId", "spaceLocalStatus", "spaceAccountStatus", "spaceRemoteStatus", "name", "description", "iconImage",
		}, open.Keys)
	})

	t.Run("the cap is the stream's own, and a closed stream returns its slot", func(t *testing.T) {
		// given: the search stream's cap is exhausted — it must not count here
		fx := newV2FixtureBare(t)
		_, subs := fx.openingSearch(t)
		for i := 0; i < maxConcurrentSearchStreams; i++ {
			_, ok := fx.searchStreams.acquire(maxConcurrentSearchStreams)
			require.True(t, ok)
		}
		var streams []*SpacesStream
		for i := 0; i < maxConcurrentSpacesStreams; i++ {
			stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
			require.NoError(t, err, "stream %d is under the cap", i+1)
			streams = append(streams, stream)
		}

		// when: one more
		_, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
		assert.Equal(t, v2model.CodeTooManyStreams, apiErr.Code)
		assert.Equal(t, "this process already holds 16 open spaces streams", apiErr.Message)
		require.Len(t, *subs, maxConcurrentSpacesStreams, "a refused open subscribes to nothing")

		// and when one closes (twice: Close is idempotent)
		streams[0].Close()
		streams[0].Close()
		assert.Equal(t, 1, (*subs)[0].closeCount())
		stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
		require.NoError(t, err, "the slot came back")
		streams = append(streams, stream)
		for _, stream := range streams[1:] {
			stream.Close()
		}
	})

	t.Run("a subscription that cannot open returns the slot", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(nil, errors.New("engine down")).Times(maxConcurrentSpacesStreams + 1)
		fx.withObjectSearch(search)

		// when / then: every attempt fails the same way, none with the cap's 429
		for i := 0; i <= maxConcurrentSpacesStreams; i++ {
			_, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
			require.ErrorContains(t, err, "engine down")
			var apiErr *v2model.Error
			require.False(t, errors.As(err, &apiErr))
		}
	})

	t.Run("without live search the stream does not open", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		_, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{}))
		require.ErrorContains(t, err, "live search is not configured")
	})
}

func TestSpacesStreamLive(t *testing.T) {
	t.Run("a created space is one space_added: Unknown, Loading and Ok are all live", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{})

		// when: the three statuses arrive as three separate engine changes
		sf.sub.push(viewAdded(spaceViewOf("v-a", streamSpaceA, "New", stUnknown, stUnknown)))
		first := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "New", stLoading, stActive)))
		second := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "New", stOk, stActive)))
		third := sf.events()

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{added(streamSpaceA, "New")}, first)
		assert.Empty(t, second, "Loading is live: no removal")
		assert.Empty(t, third, "the row did not change: no event")
	})

	t.Run("a joined space appears when the account becomes active", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{})

		// when
		sf.sub.push(viewAdded(spaceViewOf("v-a", streamSpaceA, "Shared", stUnknown, stJoining)))
		joining := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Shared", stUnknown, stActive)))
		active := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Shared", stLoading, stActive)))
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Shared", stOk, stActive)))
		loaded := sf.events()

		// then
		assert.Empty(t, joining, "never visible, so nothing to remove")
		assert.Equal(t, []v2model.SpaceStreamEvent{added(streamSpaceA, "Shared")}, active)
		assert.Empty(t, loaded)
	})

	t.Run("a load that fails removes the space as unavailable, a retry adds it again", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Flaky", stLoading, stActive))

		// when
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Flaky", stMissing, stActive)))
		failed := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Flaky", stLoading, stActive)))
		retried := sf.events()
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Flaky", stMissing, stActive)))
		failedAgain := sf.events()

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedUnavailable)}, failed)
		assert.Equal(t, []v2model.SpaceStreamEvent{added(streamSpaceA, "Flaky")}, retried)
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedUnavailable)}, failedAgain)
	})

	t.Run("an unavailable space that is then deleted is removed a second time, as gone, and no more", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Doomed", stOk, stActive))
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Doomed", stMissing, stActive)))
		require.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedUnavailable)}, sf.events())

		// when
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Doomed", stMissing, stDeleted)))
		deleted := sf.events()
		sf.sub.push(viewRemoved("v-a"))
		after := sf.events()

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedGone)}, deleted)
		assert.Empty(t, after, "the final reason was sent once")
	})

	t.Run("an unavailable space the key then loses is removed a second time, as access_removed", func(t *testing.T) {
		// given
		grant := &util.ApiGrant{Spaces: []string{streamSpaceA, streamSentinel}, Perms: util.GrantPermsRead}
		sf := openSpacesStreamFixture(t, KeyState{Grant: grant}, spaceViewOf("v-a", streamSpaceA, "Lost", stOk, stActive))
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Lost", stMissing, stActive)))
		require.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedUnavailable)}, sf.events())

		// when
		sf.watch.edit(KeyState{Grant: &util.ApiGrant{Spaces: []string{streamSentinel}, Perms: util.GrantPermsRead}})
		got := sf.events()

		// then: the space is live-less AND de-granted; the grant decides
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedAccessRemoved)}, got)
	})

	t.Run("a deleted view arrives with no details and is gone", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Deleted", stOk, stActive))

		// when: isDeleted takes the view out of the query — a removal, id only
		sf.sub.push(viewRemoved("v-a"))

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedGone)}, sf.events())
	})

	t.Run("Missing with RemoteDeleted, one write, is gone", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Remote", stOk, stActive))

		// when: the loader writes both fields in one change
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Remote", stMissing, stActive, model.SpaceStatus_RemoteDeleted)))

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedGone)}, sf.events())
	})

	t.Run("Missing with a pending network deletion is unavailable", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Pending", stOk, stActive))

		// when
		sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Pending", stMissing, stActive, model.SpaceStatus_RemoteWaitingDeletion)))

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedUnavailable)}, sf.events())
	})

	t.Run("a left or deleted space is gone", func(t *testing.T) {
		for _, account := range []model.SpaceStatus{stDeleted, model.SpaceStatus_SpaceRemoving} {
			// given
			sf := openSpacesStreamFixture(t, KeyState{}, spaceViewOf("v-a", streamSpaceA, "Left", stOk, stActive))

			// when
			sf.sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "Left", stOk, account)))

			// then
			assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedGone)}, sf.events(), account.String())
		}
	})

	t.Run("a served field that changes is space_updated with the whole row; anything else is silent", func(t *testing.T) {
		// given
		view := spaceViewOf("v-a", streamSpaceA, "Work", stOk, stActive)
		view.SetString(bundle.RelationKeyDescription, "Team notes")
		sf := openSpacesStreamFixture(t, KeyState{}, view)

		// when: a key the row does not serve changes
		unserved := view.Copy()
		unserved.SetString(bundle.RelationKeyType, "some-type")
		sf.sub.push(viewUpdated(unserved))
		silent := sf.events()
		// and when the name changes
		renamed := unserved.Copy()
		renamed.SetString(bundle.RelationKeyName, "Work 2")
		sf.sub.push(viewUpdated(renamed))
		got := sf.events()

		// then
		assert.Empty(t, silent)
		want := v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceUpdated,
			Space: &v2model.SpaceRow{Id: streamSpaceA, Name: "Work 2", Description: "Team notes"}}
		assert.Equal(t, []v2model.SpaceStreamEvent{want}, got)
	})

	t.Run("two views of one target: the space stays while a live view is left", func(t *testing.T) {
		// given: damaged data — the dead view sorts first
		sf := openSpacesStreamFixture(t, KeyState{},
			spaceViewOf("v-a1", streamSpaceA, "Dead twin", stOk, stDeleted),
			spaceViewOf("v-a2", streamSpaceA, "Live twin", stOk, stActive),
		)
		require.Contains(t, sf.snapshot, added(streamSpaceA, "Live twin"))

		// when: the dead one goes away
		sf.sub.push(viewRemoved("v-a1"))
		stays := sf.events()
		// and when the live one goes away too
		sf.sub.push(viewRemoved("v-a2"))
		leaves := sf.events()

		// then
		assert.Empty(t, stays)
		assert.Equal(t, []v2model.SpaceStreamEvent{removed(streamSpaceA, v2model.SpaceRemovedGone)}, leaves)
	})

	t.Run("one recomputation's events are in full-id order", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{},
			spaceViewOf("v-c", streamSpaceC, "C", stOk, stActive),
			spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive),
		)

		// when: one batch removes C, adds B and renames A — pushed in that order
		sf.sub.push(
			viewRemoved("v-c"),
			viewAdded(spaceViewOf("v-b", streamSpaceB, "B", stOk, stActive)),
			viewUpdated(spaceViewOf("v-a", streamSpaceA, "A2", stOk, stActive)),
		)

		// then
		want := []v2model.SpaceStreamEvent{
			updated(streamSpaceA, "A2"),
			added(streamSpaceB, "B"),
			removed(streamSpaceC, v2model.SpaceRemovedGone),
		}
		assert.Equal(t, want, sf.events())
	})

	t.Run("the server stopping ends the stream with no event", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{})

		// when
		close(sf.watch.closing)

		// then
		events, ended := sf.drain()
		assert.Empty(t, events)
		assert.True(t, ended)
	})
}

func TestSpacesStreamGrantChange(t *testing.T) {
	// One view set for every case: a space in the old grant only (A), in the
	// new only (B), in both (C), in neither (D), and a live view whose target
	// is the tech space. The sentinel is in every grant.
	views := func() []*domain.Details {
		return []*domain.Details{
			spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive),
			spaceViewOf("v-b", streamSpaceB, "B", stOk, stActive),
			spaceViewOf("v-c", streamSpaceC, "C", stOk, stActive),
			spaceViewOf("v-d", streamSpaceD, "D", stOk, stActive),
			spaceViewOf("v-tech", objectstore.TestTechSpaceId, "Tech", stOk, stActive),
		}
	}
	narrowed := func(perms string, spaces ...string) *util.ApiGrant {
		return &util.ApiGrant{Spaces: append([]string{streamSentinel}, spaces...), Perms: perms}
	}
	oldGrant := narrowed(util.GrantPermsRead, streamSpaceA, streamSpaceC)

	t.Run("the snapshot is filtered by the watch's grant, not the request context's", func(t *testing.T) {
		// given / when
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// then
		want := []v2model.SpaceStreamEvent{
			added(streamSpaceA, "A"), added(streamSpaceC, "C"), added(streamSentinel, "sentinel 0"), snapshotComplete(true),
		}
		assert.Equal(t, want, sf.snapshot)
	})

	t.Run("a narrowed and widened list removes and adds without a reconnect", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// when
		sf.watch.edit(KeyState{Grant: narrowed(util.GrantPermsRead, streamSpaceB, streamSpaceC)})

		// then: D is hidden before and after, so the flag does not move
		want := []v2model.SpaceStreamEvent{
			removed(streamSpaceA, v2model.SpaceRemovedAccessRemoved),
			added(streamSpaceB, "B"),
		}
		assert.Equal(t, want, sf.events())
		assert.Equal(t, 1, sf.watch.readCount())
	})

	t.Run("widening to every space adds the rest and flips the flag, last", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// when
		sf.watch.edit(KeyState{Grant: &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsReadWrite}})

		// then: every user space, never the tech space
		want := []v2model.SpaceStreamEvent{
			added(streamSpaceB, "B"),
			added(streamSpaceD, "D"),
			notGranted(false),
		}
		assert.Equal(t, want, sf.events())
	})

	t.Run("narrowing flips the flag to true", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsRead}}, views()...)
		require.Equal(t, snapshotComplete(false), sf.snapshot[len(sf.snapshot)-1])

		// when
		sf.watch.edit(KeyState{Grant: oldGrant})

		// then
		want := []v2model.SpaceStreamEvent{
			removed(streamSpaceB, v2model.SpaceRemovedAccessRemoved),
			removed(streamSpaceD, v2model.SpaceRemovedAccessRemoved),
			notGranted(true),
		}
		assert.Equal(t, want, sf.events())
	})

	t.Run("a scoped key that becomes unscoped sees everything the list would", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// when: the grant is cleared
		sf.watch.edit(KeyState{Grant: nil})

		// then: an unscoped key is not filtered at all, the tech target included
		want := []v2model.SpaceStreamEvent{
			added(streamSpaceB, "B"),
			added(streamSpaceD, "D"),
			added(objectstore.TestTechSpaceId, "Tech"),
			notGranted(false),
		}
		assert.ElementsMatch(t, want[:3], sf.eventsThen(want[3]))
	})

	t.Run("an unscoped key that becomes scoped loses what the grant does not name", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: nil}, views()...)
		require.Len(t, sf.snapshot, 7, "six spaces and snapshot_complete")

		// when
		sf.watch.edit(KeyState{Grant: oldGrant})

		// then
		got := sf.events()
		require.Len(t, got, 4)
		assert.ElementsMatch(t, []v2model.SpaceStreamEvent{
			removed(streamSpaceB, v2model.SpaceRemovedAccessRemoved),
			removed(streamSpaceD, v2model.SpaceRemovedAccessRemoved),
			removed(objectstore.TestTechSpaceId, v2model.SpaceRemovedAccessRemoved),
		}, got[:3])
		assert.Equal(t, notGranted(true), got[3], "the tech space never sets the flag; B and D do")
	})

	t.Run("a permission-only change emits nothing", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// when
		sf.watch.edit(KeyState{Grant: narrowed(util.GrantPermsReadWrite, streamSpaceA, streamSpaceC)})

		// then
		assert.Empty(t, sf.events())
		assert.Equal(t, 1, sf.watch.readCount(), "the key was re-read all the same")
	})

	t.Run("a revoked key ends the stream with stream_closed as the last frame", func(t *testing.T) {
		for reason, err := range map[string]error{
			v2model.StreamClosedKeyRevoked: fmt.Errorf("mint: %w", ErrKeyRevoked),
			v2model.StreamClosedKeyExpired: fmt.Errorf("mint: %w", ErrKeyExpired),
		} {
			// given
			sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

			// when
			sf.watch.fail(err)

			// then
			events, ended := sf.drain()
			want := []v2model.SpaceStreamEvent{{Type: v2model.SpaceStreamEventStreamClosed, Reason: reason}}
			assert.Equal(t, want, events, reason)
			assert.True(t, ended, reason)
			// and nothing follows: a later change is not processed
			sf.sub.push(viewRemoved("v-a"))
			sf.stream.Close()
			frames, _ := sf.stream.Drain()
			assert.Empty(t, frames, reason)
		}
	})

	t.Run("a key that cannot be re-read ends the stream with no frame", func(t *testing.T) {
		// given
		sf := openSpacesStreamFixture(t, KeyState{Grant: oldGrant}, views()...)

		// when
		sf.watch.fail(fmt.Errorf("mint: %w", ErrKeyUnverified))

		// then
		events, ended := sf.drain()
		assert.Empty(t, events)
		assert.True(t, ended)
	})
}

// eventsThen is events() for a recomputation whose last event is known and
// whose others are compared without order (ids that are not space-shaped
// sort among the shaped ones wherever their bytes fall).
func (sf *spacesStreamFixture) eventsThen(last v2model.SpaceStreamEvent) []v2model.SpaceStreamEvent {
	sf.t.Helper()
	got := sf.events()
	require.NotEmpty(sf.t, got)
	require.Equal(sf.t, last, got[len(got)-1])
	return got[:len(got)-1]
}

func TestSpacesStreamExpiry(t *testing.T) {
	const expireAt = int64(1_800_000_000)
	at := func(offset time.Duration) time.Time { return time.Unix(expireAt, 0).Add(offset) }
	open := func(t *testing.T, now time.Time, key KeyState) (*spacesStreamFixture, *fakeStreamClock) {
		fx := newV2FixtureBare(t)
		clock := newFakeStreamClock(now)
		fx.clock = clock
		return openSpacesStreamOn(t, fx, key), clock
	}

	t.Run("open until the expiry second has passed, closed at the first instant after it", func(t *testing.T) {
		// given: opened two seconds before the expiry
		sf, clock := open(t, at(-2*time.Second), KeyState{ExpireAt: expireAt})
		timer := clock.armed(t)
		assert.Equal(t, 3*time.Second, timer.wait, "the timer aims at the first instant the middleware refuses")

		// when: the clock reads exactly the expiry second, and its last instant
		for _, now := range []time.Time{at(0), at(999 * time.Millisecond)} {
			clock.set(now)
			timer.fire <- now
			timer = clock.armed(t)

			// then: still open — the middleware admits `now.Unix() == ExpireAt`
			frames, ended := sf.stream.Drain()
			assert.Empty(t, frames)
			assert.False(t, ended, "closed at %s", now.Sub(at(0)))
		}

		// when: one second later
		clock.set(at(time.Second))
		timer.fire <- at(time.Second)

		// then
		events, ended := sf.drain()
		assert.Equal(t, []v2model.SpaceStreamEvent{{Type: v2model.SpaceStreamEventStreamClosed, Reason: v2model.StreamClosedKeyExpired}}, events)
		assert.True(t, ended)
	})

	t.Run("the wall clock is read at every wake, not only when the timer fires", func(t *testing.T) {
		// given: an hour to go — the timer is capped, and it never fires here,
		// as a monotonic timer does not while the machine sleeps
		sf, clock := open(t, at(-time.Hour), KeyState{ExpireAt: expireAt})
		timer := clock.armed(t)
		assert.Equal(t, spacesStreamExpiryCheck, timer.wait)

		// when: the wall clock jumps past the expiry and any other input wakes the worker
		clock.set(at(time.Second))
		sf.sub.push(viewAdded(spaceViewOf("v-a", streamSpaceA, "Late", stOk, stActive)))

		// then: the stream closes, and the change is not delivered — the
		// key was already expired when it arrived
		var all []v2model.SpaceStreamEvent
		for {
			events, ended := sf.drain()
			all = append(all, events...)
			if ended {
				break
			}
		}
		assert.Equal(t, []v2model.SpaceStreamEvent{
			{Type: v2model.SpaceStreamEventStreamClosed, Reason: v2model.StreamClosedKeyExpired},
		}, all)
	})

	t.Run("a re-read replaces the expiry", func(t *testing.T) {
		// given: a key that never expires arms no timer
		sf, clock := open(t, at(-time.Hour), KeyState{})
		require.Empty(t, sf.events())
		require.Empty(t, clock.timers, "no expiry, no timer")

		// when: an edit gives it one
		sf.watch.edit(KeyState{ExpireAt: expireAt})

		// then: the timer is armed for it
		timer := clock.armed(t)
		assert.Equal(t, spacesStreamExpiryCheck, timer.wait)
		_, ended := sf.stream.Drain()
		assert.False(t, ended)
	})

	t.Run("a key already expired when the stream opens is refused: no snapshot is sent under it", func(t *testing.T) {
		// given: authentication admitted the request in the key's last
		// instant; by the time the stream opens the clock is one second on
		fx := newV2FixtureBare(t)
		fx.clock = newFakeStreamClock(at(time.Second))
		_, subs := fx.openingSearch(t, viewAdded(spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive)),
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete})

		// when
		stream, err := fx.OpenSpacesStream(context.Background(), newFakeKeyWatch(KeyState{ExpireAt: expireAt}))

		// then: the error the handler answers 401 for, and nothing is held
		require.ErrorIs(t, err, ErrKeyExpired)
		assert.Nil(t, stream)
		require.Len(t, *subs, 1)
		assert.Equal(t, 1, (*subs)[0].closeCount())
		assert.Equal(t, 0, fx.spacesStreams.open)
	})

	t.Run("at the expiry second the stream still opens", func(t *testing.T) {
		// given: the control — the middleware admits this instant too
		sf, _ := open(t, at(999*time.Millisecond), KeyState{ExpireAt: expireAt})

		// then
		assert.Equal(t, []v2model.SpaceStreamEvent{added(streamSentinel, "sentinel 0"), snapshotComplete(false)}, sf.snapshot)
	})
}

// TestSpacesStreamKeyComesBeforeViews pins the order inside one wake. The
// worker picks among ready inputs at random, so a key change can be pending
// when the subscription's wake is the one handled. Each case here builds the
// stream without its worker, leaves BOTH inputs pending, and handles the
// subscription's wake directly: the outcome does not depend on scheduling.
func TestSpacesStreamKeyComesBeforeViews(t *testing.T) {
	both := &util.ApiGrant{Spaces: []string{streamSpaceA, streamSpaceB}, Perms: util.GrantPermsRead}
	onlyB := &util.ApiGrant{Spaces: []string{streamSpaceB}, Perms: util.GrantPermsRead}
	build := func(t *testing.T) (*SpacesStream, *fakeObjectSearch, *fakeKeyWatch) {
		fx := newV2FixtureBare(t)
		sub := newFakeObjectSearch(
			viewAdded(spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive)),
			viewAdded(spaceViewOf("v-b", streamSpaceB, "B", stOk, stActive)),
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete},
		)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
		fx.withObjectSearch(search)
		watch := newFakeKeyWatch(KeyState{Grant: both})
		stream, err := fx.newSpacesStream(context.Background(), watch)
		require.NoError(t, err)
		t.Cleanup(stream.Close)
		return stream, sub, watch
	}
	queued := func(t *testing.T, stream *SpacesStream) ([]v2model.SpaceStreamEvent, bool) {
		frames, ended := stream.Drain()
		return decodeSpaceFrames(t, frames), ended
	}

	t.Run("a pending grant edit is applied before the view change is emitted", func(t *testing.T) {
		// given: the key lost A, and A was renamed — both pending
		stream, sub, watch := build(t)
		watch.edit(KeyState{Grant: onlyB})
		sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "A renamed", stOk, stActive)))

		// when
		open := stream.onViewChanges(context.Background())

		// then: A leaves; its new name is never sent to a key that lost it
		require.True(t, open)
		events, ended := queued(t, stream)
		assert.Equal(t, []v2model.SpaceStreamEvent{
			removed(streamSpaceA, v2model.SpaceRemovedAccessRemoved),
			notGranted(true),
		}, events)
		assert.False(t, ended)
		assert.Equal(t, 1, watch.readCount())
	})

	t.Run("a pending revocation ends the stream before the view change is emitted", func(t *testing.T) {
		// given
		stream, sub, watch := build(t)
		watch.fail(fmt.Errorf("mint: %w", ErrKeyRevoked))
		sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "A renamed", stOk, stActive)))

		// when
		open := stream.onViewChanges(context.Background())

		// then
		require.False(t, open)
		events, ended := queued(t, stream)
		assert.Equal(t, []v2model.SpaceStreamEvent{{Type: v2model.SpaceStreamEventStreamClosed, Reason: v2model.StreamClosedKeyRevoked}}, events)
		assert.True(t, ended)
	})

	t.Run("with no key change pending the view change is emitted and the key is not re-read", func(t *testing.T) {
		// given
		stream, sub, watch := build(t)
		sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "A renamed", stOk, stActive)))

		// when
		open := stream.onViewChanges(context.Background())

		// then
		require.True(t, open)
		events, _ := queued(t, stream)
		assert.Equal(t, []v2model.SpaceStreamEvent{updated(streamSpaceA, "A renamed")}, events)
		assert.Equal(t, 0, watch.readCount())
	})

	t.Run("an expired key ends the stream before the view change is emitted", func(t *testing.T) {
		// given: the clock is past the key's expiry when the change arrives
		fx := newV2FixtureBare(t)
		clock := newFakeStreamClock(time.Unix(100, 0))
		fx.clock = clock
		sub := newFakeObjectSearch(
			viewAdded(spaceViewOf("v-a", streamSpaceA, "A", stOk, stActive)),
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete},
		)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(sub, nil).Once()
		fx.withObjectSearch(search)
		stream, err := fx.newSpacesStream(context.Background(), newFakeKeyWatch(KeyState{ExpireAt: 200}))
		require.NoError(t, err)
		t.Cleanup(stream.Close)
		clock.set(time.Unix(201, 0))
		sub.push(viewUpdated(spaceViewOf("v-a", streamSpaceA, "A renamed", stOk, stActive)))

		// when
		open := stream.onViewChanges(context.Background())

		// then
		require.False(t, open)
		events, ended := queued(t, stream)
		assert.Equal(t, []v2model.SpaceStreamEvent{{Type: v2model.SpaceStreamEventStreamClosed, Reason: v2model.StreamClosedKeyExpired}}, events)
		assert.True(t, ended)
	})
}
