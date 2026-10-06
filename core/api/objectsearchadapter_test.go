package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const searchSpaceId = "space1"

// objectSearchFixture runs the adapter against the real subscription engine.
// Its searcher wraps the engine's, so a test can count unsubscribes and make
// a subscribe fail.
type objectSearchFixture struct {
	*objectSearchAdapter
	subs  *subscription.InternalTestService
	store *objectstore.StoreFixture

	mu           sync.Mutex
	unsubscribes map[string]int
	failSearch   error
}

func newObjectSearchFixture(t *testing.T) *objectSearchFixture {
	subs := subscription.NewInternalTestService(t)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = subs.Close(closeCtx)
	})
	fx := &objectSearchFixture{subs: subs, store: subs.StoreFixture, unsubscribes: map[string]int{}}
	fx.objectSearchAdapter = newObjectSearchAdapter(fx)
	return fx
}

func (fx *objectSearchFixture) Search(req subscription.SubscribeRequest) (*subscription.SubscribeResponse, error) {
	fx.mu.Lock()
	err := fx.failSearch
	fx.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return fx.subs.Search(req)
}

func (fx *objectSearchFixture) Unsubscribe(subIds ...string) error {
	fx.mu.Lock()
	for _, id := range subIds {
		fx.unsubscribes[id]++
	}
	fx.mu.Unlock()
	return fx.subs.Unsubscribe(subIds...)
}

func (fx *objectSearchFixture) unsubscribeCounts() map[string]int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	out := map[string]int{}
	for id, n := range fx.unsubscribes {
		out[id] = n
	}
	return out
}

func (fx *objectSearchFixture) ourSubscriptions() []string {
	var ours []string
	for _, id := range fx.subs.SubscriptionIDs() {
		if strings.HasPrefix(id, objectSearchSubIdPrefix) {
			ours = append(ours, id)
		}
	}
	return ours
}

// mentionedFilter is the stream's mentions recipe: unread mentions above 0.
var mentionedFilter = []database.FilterRequest{{
	RelationKey: bundle.RelationKeyUnreadMentionCount,
	Condition:   model.BlockContentDataviewFilter_Greater,
	Value:       domain.Int64(0),
}}

func (fx *objectSearchFixture) open(t *testing.T, req apicore.ObjectSearchOpen) apicore.ObjectSearchSubscription {
	t.Helper()
	if req.SpaceId == "" {
		req.SpaceId = searchSpaceId
	}
	sub, err := fx.OpenObjectSearch(context.Background(), req)
	require.NoError(t, err)
	t.Cleanup(sub.Close)
	return sub
}

func givenSearchObject(id, name string, mentions int64) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyName:               domain.String(name),
		bundle.RelationKeyType:               domain.String("type-page"),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyUnreadMentionCount: domain.Int64(mentions),
		bundle.RelationKeyLastModifiedDate:   domain.Int64(100),
	}
}

// change is a decoded ObjectSearchChange: just enough to assert on.
type change struct {
	Type    apicore.ObjectSearchChangeType
	Id      string
	Details map[string]any
}

func decodeChanges(changes []apicore.ObjectSearchChange) []change {
	out := make([]change, 0, len(changes))
	for _, c := range changes {
		decoded := change{Type: c.Type, Id: c.Id}
		if c.Details != nil {
			decoded.Details = map[string]any{}
			for key, value := range c.Details.Iterate() {
				decoded.Details[string(key)] = value.Raw()
			}
		}
		out = append(out, decoded)
	}
	return out
}

// waitChanges reads live changes until n have arrived.
func waitChanges(t *testing.T, sub apicore.ObjectSearchSubscription, n int) []change {
	t.Helper()
	deadline := time.After(3 * time.Second)
	var got []change
	for len(got) < n {
		select {
		case <-sub.Ready():
			got = append(got, decodeChanges(sub.Drain())...)
		case <-deadline:
			t.Fatalf("got %d of %d changes: %+v", len(got), n, got)
		}
	}
	return got
}

// assertNoChanges waits a little and asserts nothing more arrived.
func assertNoChanges(t *testing.T, sub apicore.ObjectSearchSubscription) {
	t.Helper()
	select {
	case <-sub.Ready():
		assert.Empty(t, decodeChanges(sub.Drain()))
	case <-time.After(150 * time.Millisecond):
	}
}

func rowDetails(id, name string, extra map[string]any) map[string]any {
	out := map[string]any{"id": id, "name": name, "type": "type-page"}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func TestObjectSearchSnapshot(t *testing.T) {
	t.Run("an empty set opens with snapshot complete alone", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		want := []change{{Type: apicore.ObjectSearchSnapshotComplete}}

		// when
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter})

		// then
		assert.Equal(t, want, decodeChanges(sub.Snapshot()))
	})

	t.Run("the snapshot is every match, in the sort order, carrying the requested keys and the base keys only", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		withDiscussion := givenSearchObject("obj-b", "Beta", 2)
		withDiscussion[bundle.RelationKeyDiscussionId] = domain.String("disc-b")
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{
			withDiscussion,
			givenSearchObject("obj-a", "Alpha", 1),
			givenSearchObject("obj-quiet", "Quiet", 0),
		})
		want := []change{
			{Type: apicore.ObjectSearchAdded, Id: "obj-a", Details: rowDetails("obj-a", "Alpha", map[string]any{"unreadMentionCount": float64(1)})},
			{Type: apicore.ObjectSearchAdded, Id: "obj-b", Details: rowDetails("obj-b", "Beta", map[string]any{"unreadMentionCount": float64(2), "discussionId": "disc-b"})},
			{Type: apicore.ObjectSearchSnapshotComplete},
		}

		// when
		sub := fx.open(t, apicore.ObjectSearchOpen{
			Filters: mentionedFilter,
			Sorts:   []database.SortRequest{{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc}},
			Keys:    []string{bundle.RelationKeyUnreadMentionCount.String()},
		})

		// then
		assert.Equal(t, want, decodeChanges(sub.Snapshot()))
	})

	t.Run("a failing subscribe is an error and leaves nothing subscribed", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		fx.failSearch = errors.New("space not loaded")

		// when
		_, err := fx.OpenObjectSearch(context.Background(), apicore.ObjectSearchOpen{SpaceId: searchSpaceId, Filters: mentionedFilter})

		// then
		require.ErrorContains(t, err, "space not loaded")
		assert.Empty(t, fx.ourSubscriptions())
	})
}

func TestObjectSearchLive(t *testing.T) {
	keys := []string{bundle.RelationKeyUnreadMentionCount.String()}

	t.Run("an object entering the set is added, leaving it is removed", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 0)})
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter, Keys: keys})
		want := []change{
			{Type: apicore.ObjectSearchAdded, Id: "obj1", Details: rowDetails("obj1", "Plan", map[string]any{"unreadMentionCount": float64(1)})},
			{Type: apicore.ObjectSearchRemoved, Id: "obj1"},
		}

		// when: mentioned, then the mention is read
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 1)})
		got := waitChanges(t, sub, 1)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 0)})
		got = append(got, waitChanges(t, sub, 1)...)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a carried change updates the member with its whole row, an uncarried one is silent", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 1)})
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter, Keys: keys})
		renamed := givenSearchObject("obj1", "Plan v2", 1)
		uncarried := givenSearchObject("obj1", "Plan v2", 1)
		uncarried[bundle.RelationKeyLastModifiedDate] = domain.Int64(200)
		want := []change{
			{Type: apicore.ObjectSearchUpdated, Id: "obj1", Details: rowDetails("obj1", "Plan v2", map[string]any{"unreadMentionCount": float64(1)})},
		}

		// when
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{renamed})
		got := waitChanges(t, sub, 1)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{uncarried})

		// then
		assert.Equal(t, want, got)
		assertNoChanges(t, sub)
	})

	t.Run("gaining, changing and losing a discussion updates the member", func(t *testing.T) {
		// given: the caller asked for no discussion key; the search carries it
		fx := newObjectSearchFixture(t)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 1)})
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter})
		withDiscussion := func(id string) objectstore.TestObject {
			obj := givenSearchObject("obj1", "Plan", 1)
			if id != "" {
				obj[bundle.RelationKeyDiscussionId] = domain.String(id)
			}
			return obj
		}
		want := []change{
			{Type: apicore.ObjectSearchUpdated, Id: "obj1", Details: rowDetails("obj1", "Plan", map[string]any{"discussionId": "disc1"})},
			{Type: apicore.ObjectSearchUpdated, Id: "obj1", Details: rowDetails("obj1", "Plan", map[string]any{"discussionId": "disc2"})},
			{Type: apicore.ObjectSearchUpdated, Id: "obj1", Details: rowDetails("obj1", "Plan", nil)},
		}

		// when
		var got []change
		for _, id := range []string{"disc1", "disc2", ""} {
			fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{withDiscussion(id)})
			got = append(got, waitChanges(t, sub, 1)...)
		}

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a delivered change is never mutated by a later one", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 1)})
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter, Keys: keys})
		snapshot := sub.Snapshot()
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 2)})
		first := waitChanges(t, sub, 1)

		// when
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 3)})
		waitChanges(t, sub, 1)

		// then
		assert.Equal(t, float64(1), decodeChanges(snapshot)[0].Details["unreadMentionCount"])
		assert.Equal(t, float64(2), first[0].Details["unreadMentionCount"])
	})

	t.Run("a reader that drains late loses nothing", func(t *testing.T) {
		// given: the engine queue is unbounded and so is the subscription's
		fx := newObjectSearchFixture(t)
		sub := fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter, Keys: keys})
		var want []change
		for i := 1; i <= 200; i++ {
			id := fmt.Sprintf("obj%03d", i)
			want = append(want, change{Type: apicore.ObjectSearchAdded, Id: id,
				Details: rowDetails(id, "Busy", map[string]any{"unreadMentionCount": float64(i)})})
		}

		// when: nothing reads while the changes land
		for i := 1; i <= 200; i++ {
			fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject(fmt.Sprintf("obj%03d", i), "Busy", int64(i))})
		}

		// then
		assert.Equal(t, want, waitChanges(t, sub, len(want)))
	})
}

func TestObjectSearchClose(t *testing.T) {
	t.Run("Close unsubscribes exactly once and stops delivery", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)
		sub, err := fx.OpenObjectSearch(context.Background(), apicore.ObjectSearchOpen{SpaceId: searchSpaceId, Filters: mentionedFilter})
		require.NoError(t, err)
		ours := fx.ourSubscriptions()
		require.Len(t, ours, 1)

		// when
		sub.Close()
		sub.Close()
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{givenSearchObject("obj1", "Plan", 1)})

		// then
		assert.Equal(t, map[string]int{ours[0]: 1}, fx.unsubscribeCounts())
		assert.Empty(t, fx.ourSubscriptions())
		assert.Empty(t, sub.Drain())
	})

	t.Run("every search gets its own subscription", func(t *testing.T) {
		// given
		fx := newObjectSearchFixture(t)

		// when
		fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter})
		fx.open(t, apicore.ObjectSearchOpen{Filters: mentionedFilter})

		// then
		assert.Len(t, fx.ourSubscriptions(), 2)
	})
}
