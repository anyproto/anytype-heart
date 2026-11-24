package v2service

// spaces_stream.go is the service half of the spaces stream
// (GET /v2/spaces/stream, APIV2_SPACE_STREAM.md): GET /v2/spaces held open.
// It subscribes to every space view of the tech space, keeps them in a map,
// and after every input — a batch of view changes or a re-read of the key —
// runs the list's own visibleSpaces over that map and the key's current
// grant, and emits the difference from what it sent before.
//
// It is the SPACES stream: the set of spaces a key can see. The per-space
// chat stream is chat_space_stream.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// maxConcurrentSpacesStreams caps open spaces streams process-wide, on a
// counter of its own: this stream and the chat and search streams cannot
// starve one another.
const maxConcurrentSpacesStreams = 16

// spacesStreamExpiryCheck is the longest the stream sleeps between two looks
// at the wall clock while its key has an expiry. A timer measures monotonic
// time, which does not advance while the machine sleeps, so a key can expire
// across a closed laptop lid with the timer still far from firing.
const spacesStreamExpiryCheck = time.Minute

// What KeyWatch.Current answers when the key can no longer be used.
var (
	// ErrKeyRevoked: the key was deleted.
	ErrKeyRevoked = errors.New("api key revoked")
	// ErrKeyExpired: the key is past its expiry.
	ErrKeyExpired = errors.New("api key expired")
	// ErrKeyUnverified: the key could not be re-read for any other reason.
	// The stream ends without saying why: the reconnect's own authentication
	// decides.
	ErrKeyUnverified = errors.New("api key could not be verified")
)

// KeyState is what the spaces stream needs of an API key: the grant it
// filters by and the instant it stops being valid (unix seconds, 0 = never).
type KeyState struct {
	Grant    *util.ApiGrant
	ExpireAt int64
}

// KeyWatch follows the API key one stream was opened with. The server
// implements it (it owns the key cache and hears every grant edit and
// revocation); the stream never sees the key itself.
type KeyWatch interface {
	// State is the key as it was when the watch was registered.
	State() KeyState
	// Changed is signaled when the key's grant was edited or the key was
	// revoked. Signals coalesce; the signal does not say which happened.
	Changed() <-chan struct{}
	// Current re-reads the key the way the authentication middleware does.
	// It returns ErrKeyRevoked, ErrKeyExpired or ErrKeyUnverified when the
	// key can no longer be used.
	Current(ctx context.Context) (KeyState, error)
	// Closing is closed when the server that accepted the stream is rebound
	// or stopped: it will hear no later edit of this key.
	Closing() <-chan struct{}
	// Close ends the watch. It is safe to call more than once.
	Close()
}

// streamClock is the wall clock and the timer of the spaces stream's expiry
// check; a field of the service so a test can drive both.
type streamClock interface {
	Now() time.Time
	// After returns a channel that receives once d has passed, and a func
	// that releases the timer.
	After(d time.Duration) (<-chan time.Time, func())
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration) (<-chan time.Time, func()) {
	timer := time.NewTimer(d)
	return timer.C, func() { timer.Stop() }
}

// SpacesStream is one open spaces stream: the subscription to the space
// views, the key watch, and the worker that turns both into frames. Close is
// safe to call more than once.
type SpacesStream struct {
	sub         apicore.ObjectSearchSubscription
	watch       KeyWatch
	clock       streamClock
	techSpaceId string
	snapshot    []StreamFrame

	// The worker's state; nothing else touches it once the worker runs.
	// views is every space view the subscription holds, by view id. sent is
	// the visible set as the client has it, by full space id. unavailable
	// holds the spaces last removed as unavailable, which get one more
	// space_removed when their final reason is known.
	views       map[string]*domain.Details
	key         KeyState
	sent        map[string]v2model.SpaceRow
	sentFlag    bool
	unavailable map[string]bool

	mu    sync.Mutex
	queue []StreamFrame
	ended bool
	ready chan struct{}

	cancel    context.CancelFunc
	stopped   chan struct{}
	release   func()
	closeOnce sync.Once
}

// Snapshot is the opening snapshot: a space_added frame per visible space,
// in full-id order, then snapshot_complete.
func (s *SpacesStream) Snapshot() []StreamFrame { return s.snapshot }

// Ready is signaled whenever frames are queued or the stream ended.
func (s *SpacesStream) Ready() <-chan struct{} { return s.ready }

// Drain returns and removes every queued frame, oldest first, and reports
// whether the server ended the stream: after a drain that says so, nothing
// more will come.
func (s *SpacesStream) Drain() ([]StreamFrame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := s.queue
	s.queue = nil
	return frames, s.ended
}

// Close stops the worker, ends the subscription and returns the stream slot.
// It does not close the key watch, which belongs to whoever made it.
func (s *SpacesStream) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
			<-s.stopped
		}
		s.sub.Close()
		s.release()
	})
}

// OpenSpacesStream opens the spaces stream for the key behind watch. Every
// refusal is returned before the stream's first byte.
//
// The subscription is the list's own query (spaceViewFilters, spaceViewKeys)
// on the tech space, filtered by neither liveness nor grant: a grant edit is
// then a re-filter of the view map with no resubscribe, and a status change
// arrives as an update carrying the details a removal reason is read from.
// No caller input reaches that query, which is what makes reading the tech
// space here the same as ListSpaces' own read of it, whatever the key's
// grant says about the tech space.
func (s *Service) OpenSpacesStream(ctx context.Context, watch KeyWatch) (*SpacesStream, error) {
	stream, err := s.newSpacesStream(ctx, watch)
	if err != nil {
		return nil, err
	}
	stream.start()
	return stream, nil
}

// newSpacesStream builds the stream and its snapshot; start runs its worker.
func (s *Service) newSpacesStream(ctx context.Context, watch KeyWatch) (*SpacesStream, error) {
	if s.objectSearch == nil {
		return nil, errors.New("open spaces stream: live search is not configured")
	}
	if watch == nil {
		return nil, errors.New("open spaces stream: no key watch")
	}
	release, ok := s.spacesStreams.acquire(maxConcurrentSpacesStreams)
	if !ok {
		return nil, v2model.NewError(http.StatusTooManyRequests, v2model.CodeTooManyStreams,
			fmt.Sprintf("this process already holds %d open spaces streams", maxConcurrentSpacesStreams),
			v2model.Issue{
				Message: "the cap is on streams held at once, not on how fast they are opened, so retrying the same request cannot succeed",
				Hint:    "close a spaces stream you no longer read before opening another",
			})
	}
	// as in OpenSearchStream: a failure or a panic below must not burn the
	// slot or leak the subscription
	var sub apicore.ObjectSearchSubscription
	handedOff := false
	defer func() {
		if handedOff {
			return
		}
		if sub != nil {
			sub.Close()
		}
		release()
	}()

	keys := make([]string, len(spaceViewKeys))
	for i, key := range spaceViewKeys {
		keys[i] = key.String()
	}
	sub, err := s.objectSearch.OpenObjectSearch(ctx, apicore.ObjectSearchOpen{
		SpaceId: s.techSpaceId,
		Filters: spaceViewFilters(),
		Keys:    keys,
	})
	if err != nil {
		return nil, fmt.Errorf("open space view search: %w", err)
	}

	stream := &SpacesStream{
		sub:         sub,
		watch:       watch,
		clock:       s.clock,
		techSpaceId: s.techSpaceId,
		views:       map[string]*domain.Details{},
		key:         watch.State(),
		sent:        map[string]v2model.SpaceRow{},
		unavailable: map[string]bool{},
		ready:       make(chan struct{}, 1),
		release:     release,
	}
	// The watch was registered before the subscription opened, so an edit or
	// a revocation that landed in between has left a signal. It is consumed
	// here: the snapshot must not carry a space the key has already lost,
	// and a key that is gone is refused before the first byte.
	if stream.keyChanged() {
		key, err := watch.Current(ctx)
		if err != nil {
			return nil, fmt.Errorf("re-read the key before the snapshot: %w", err)
		}
		stream.key = key
	}
	// The authentication middleware admitted the request a moment ago; the
	// key can have expired since (the re-read above checks it, but only
	// runs when a signal is pending). A stream is not opened, and a snapshot
	// not sent, under a key a new request would be refused for.
	if util.KeyExpired(stream.key.ExpireAt, s.clock.Now()) {
		return nil, fmt.Errorf("open spaces stream: %w", ErrKeyExpired)
	}
	stream.applyChanges(sub.Snapshot())
	rows, hasNotGranted := visibleSpaces(stream.viewList(), stream.key.Grant, stream.techSpaceId)
	for _, row := range rows {
		row := row
		stream.sent[row.Id] = row
		stream.snapshot = appendSpaceFrame(stream.snapshot, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceAdded, Space: &row})
	}
	stream.sentFlag = hasNotGranted
	stream.snapshot = appendSpaceFrame(stream.snapshot, v2model.SpaceStreamEvent{
		Type: v2model.SpaceStreamEventSnapshotComplete, HasNotGrantedSpaces: &hasNotGranted,
	})
	handedOff = true
	return stream, nil
}

// start runs the worker. Called once, by OpenSpacesStream.
func (s *SpacesStream) start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stopped = make(chan struct{})
	go s.run(ctx)
}

// run is the worker: it merges the view changes, the key's change signal,
// the server's closing signal and the expiry check into the one output
// queue. It ends the stream when the key is revoked, expired or can no
// longer be verified, or when the server stops; Close ends it silently.
//
// Whatever woke it, the key comes first: a pending change signal is consumed
// and the key re-read, then the wall clock is checked, and only then are view
// changes turned into events. The stream never emits a row under a grant it
// has already been told is stale, nor after its key expired.
func (s *SpacesStream) run(ctx context.Context) {
	defer close(s.stopped)
	for {
		if s.endIfExpired() {
			return
		}
		var tick <-chan time.Time
		stopTick := func() {}
		if s.key.ExpireAt > 0 {
			tick, stopTick = s.clock.After(s.untilExpiryCheck())
		}
		select {
		case <-ctx.Done():
			stopTick()
			return
		case <-s.watch.Closing():
			stopTick()
			s.end("")
			return
		case <-s.sub.Ready():
			stopTick()
			if !s.onViewChanges(ctx) {
				return
			}
		case <-s.watch.Changed():
			stopTick()
			if !s.onKeyChanged(ctx) {
				return
			}
		case <-tick:
			stopTick()
		}
	}
}

// onViewChanges handles a wake by the subscription. It reports false when
// the stream ended.
//
// select picks among ready inputs at random, so a key change can be pending
// when the subscription wins. Recomputing first would emit the view change
// under the grant the key no longer has — a space_updated for a space the
// key just lost, ahead of its removal. The pending signal is taken first.
func (s *SpacesStream) onViewChanges(ctx context.Context) bool {
	changes := s.sub.Drain()
	if s.keyChanged() && !s.reread(ctx) {
		return false
	}
	if s.endIfExpired() {
		return false
	}
	s.applyChanges(changes)
	s.recompute()
	return true
}

// onKeyChanged handles a wake by the key's change signal. It reports false
// when the stream ended.
func (s *SpacesStream) onKeyChanged(ctx context.Context) bool {
	if !s.reread(ctx) {
		return false
	}
	if s.endIfExpired() {
		return false
	}
	s.recompute()
	return true
}

// keyChanged consumes a pending change signal, if there is one.
func (s *SpacesStream) keyChanged() bool {
	select {
	case <-s.watch.Changed():
		return true
	default:
		return false
	}
}

// reread replaces the stream's key with the key as it is now. The signal
// was consumed before the call, so an edit that lands during the re-read
// leaves a signal for the next turn. It reports false when the stream ended:
// with stream_closed for a revoked or expired key, with no frame for one
// that cannot be verified.
func (s *SpacesStream) reread(ctx context.Context) bool {
	key, err := s.watch.Current(ctx)
	switch {
	case errors.Is(err, ErrKeyRevoked):
		s.end(v2model.StreamClosedKeyRevoked)
		return false
	case errors.Is(err, ErrKeyExpired):
		s.end(v2model.StreamClosedKeyExpired)
		return false
	case err != nil:
		if ctx.Err() == nil {
			log.Warn("v2 spaces stream: re-read the key, ending the stream", zap.Error(err))
			s.end("")
		}
		return false
	}
	s.key = key
	return true
}

// endIfExpired ends the stream with key_expired when the key's expiry has
// passed. The predicate is the authentication middleware's and the clock is
// read at every wake, so the stream ends at the first instant a new request
// would be refused — before anything else is emitted.
func (s *SpacesStream) endIfExpired() bool {
	if !util.KeyExpired(s.key.ExpireAt, s.clock.Now()) {
		return false
	}
	s.end(v2model.StreamClosedKeyExpired)
	return true
}

// untilExpiryCheck is how long the worker may sleep before it looks at the
// clock again: until the first instant the key is expired, and never longer
// than spacesStreamExpiryCheck.
func (s *SpacesStream) untilExpiryCheck() time.Duration {
	until := time.Unix(s.key.ExpireAt+1, 0).Sub(s.clock.Now())
	if until > spacesStreamExpiryCheck {
		return spacesStreamExpiryCheck
	}
	if until < time.Millisecond {
		return time.Millisecond
	}
	return until
}

// applyChanges folds view changes into the view map.
func (s *SpacesStream) applyChanges(changes []apicore.ObjectSearchChange) {
	for _, change := range changes {
		switch change.Type {
		case apicore.ObjectSearchAdded, apicore.ObjectSearchUpdated:
			if change.Details != nil {
				s.views[change.Id] = change.Details
			}
		case apicore.ObjectSearchRemoved:
			delete(s.views, change.Id)
		}
	}
}

func (s *SpacesStream) viewList() []*domain.Details {
	views := make([]*domain.Details, 0, len(s.views))
	for _, view := range s.views {
		views = append(views, view)
	}
	return views
}

// recompute runs the list's definition over the current views and grant and
// queues the difference from what was sent: one recomputation's events in
// full-id order, not_granted_updated last.
func (s *SpacesStream) recompute() {
	views := s.viewList()
	rows, hasNotGranted := visibleSpaces(views, s.key.Grant, s.techSpaceId)
	visible := make(map[string]v2model.SpaceRow, len(rows))
	ids := make([]string, 0, len(rows)+len(s.sent)+len(s.unavailable))
	for _, row := range rows {
		visible[row.Id] = row
		ids = append(ids, row.Id)
	}
	for id := range s.sent {
		if _, ok := visible[id]; !ok {
			ids = append(ids, id)
		}
	}
	for id := range s.unavailable {
		if _, ok := visible[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var frames []StreamFrame
	for _, id := range ids {
		row, isVisible := visible[id]
		old, wasSent := s.sent[id]
		switch {
		case isVisible && !wasSent:
			row := row
			frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceAdded, Space: &row})
			s.sent[id] = row
			delete(s.unavailable, id)
		case isVisible && row != old:
			row := row
			frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceUpdated, Space: &row})
			s.sent[id] = row
		case isVisible:
			// unchanged
		case wasSent:
			reason := spaceRemovalReason(id, views, s.key.Grant, s.techSpaceId)
			frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceRemoved, SpaceId: id, Reason: reason})
			delete(s.sent, id)
			if reason == v2model.SpaceRemovedUnavailable {
				s.unavailable[id] = true
			}
		default:
			// last removed as unavailable: once its final reason is known the
			// client is told, so it does not wait for a space that is gone
			// or no longer its to see
			if reason := spaceRemovalReason(id, views, s.key.Grant, s.techSpaceId); reason != v2model.SpaceRemovedUnavailable {
				frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceRemoved, SpaceId: id, Reason: reason})
				delete(s.unavailable, id)
			}
		}
	}
	if hasNotGranted != s.sentFlag {
		s.sentFlag = hasNotGranted
		frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{
			Type: v2model.SpaceStreamEventNotGrantedUpdated, HasNotGrantedSpaces: &hasNotGranted,
		})
	}
	s.push(frames, false)
}

// end queues the closing frame, if the end has a reason the client is told,
// and marks the stream ended.
func (s *SpacesStream) end(reason string) {
	var frames []StreamFrame
	if reason != "" {
		frames = appendSpaceFrame(frames, v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventStreamClosed, Reason: reason})
	}
	s.push(frames, true)
}

func (s *SpacesStream) push(frames []StreamFrame, ended bool) {
	if len(frames) == 0 && !ended {
		return
	}
	s.mu.Lock()
	s.queue = append(s.queue, frames...)
	s.ended = s.ended || ended
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// appendSpaceFrame renders one event. An event that cannot be rendered is
// logged and dropped, as on the search stream.
func appendSpaceFrame(frames []StreamFrame, event v2model.SpaceStreamEvent) []StreamFrame {
	data, err := json.Marshal(event)
	if err != nil {
		log.Warn("v2 spaces stream: marshal event, dropping it", zap.String("type", event.Type), zap.Error(err))
		return frames
	}
	return append(frames, StreamFrame{Type: event.Type, Data: data})
}

// spaceRemovalReason says why spaceId, a space that is not in the visible
// set, is not: the first match of
//
//  1. access_removed — the grant refuses it, by the check visibleSpaces ran.
//     First, so a space the key lost never reveals its later state;
//  2. gone — no view of the space is left (a deleted or archived view leaves
//     the subscription with no details to read), or every one left says the
//     account no longer holds it;
//  3. unavailable — otherwise: the account holds the space and it is not
//     usable on this device, for now.
func spaceRemovalReason(spaceId string, views []*domain.Details, grant *util.ApiGrant, techSpaceId string) string {
	if grant != nil && util.SpaceGrantRefusal(grant, spaceId, techSpaceId) != "" {
		return v2model.SpaceRemovedAccessRemoved
	}
	for _, view := range views {
		if view.GetString(bundle.RelationKeyTargetSpaceId) != spaceId {
			continue
		}
		if spaceViewState(view) != spaceViewGone {
			return v2model.SpaceRemovedUnavailable
		}
	}
	return v2model.SpaceRemovedGone
}
