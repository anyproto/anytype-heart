package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

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
)

// fakeObjectSearch is what the live search port hands back: a snapshot and a
// live queue. It counts closes, so a test can see the stream release it.
type fakeObjectSearch struct {
	snapshot []apicore.ObjectSearchChange
	ready    chan struct{}

	mu     sync.Mutex
	live   []apicore.ObjectSearchChange
	closes int
}

func newFakeObjectSearch(snapshot ...apicore.ObjectSearchChange) *fakeObjectSearch {
	if len(snapshot) == 0 {
		snapshot = []apicore.ObjectSearchChange{{Type: apicore.ObjectSearchSnapshotComplete}}
	}
	return &fakeObjectSearch{snapshot: snapshot, ready: make(chan struct{}, 1)}
}

func (f *fakeObjectSearch) Snapshot() []apicore.ObjectSearchChange { return f.snapshot }
func (f *fakeObjectSearch) Ready() <-chan struct{}                 { return f.ready }

func (f *fakeObjectSearch) Drain() []apicore.ObjectSearchChange {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := f.live
	f.live = nil
	return live
}

func (f *fakeObjectSearch) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

func (f *fakeObjectSearch) push(changes ...apicore.ObjectSearchChange) {
	f.mu.Lock()
	f.live = append(f.live, changes...)
	f.mu.Unlock()
	select {
	case f.ready <- struct{}{}:
	default:
	}
}

func (f *fakeObjectSearch) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func (fx *v2Fixture) withObjectSearch(search apicore.ObjectSearchService) {
	fx.objectSearch = search
}

// unusedObjectSearch is a port with no expectations: reaching it fails the test.
func (fx *v2Fixture) unusedObjectSearch(t *testing.T) {
	fx.withObjectSearch(mock_apicore.NewMockObjectSearchService(t))
}

// openingSearch answers every open with a fresh fake and records the request.
func (fx *v2Fixture) openingSearch(t *testing.T, snapshot ...apicore.ObjectSearchChange) (*[]apicore.ObjectSearchOpen, *[]*fakeObjectSearch) {
	var opens []apicore.ObjectSearchOpen
	var subs []*fakeObjectSearch
	var mu sync.Mutex
	search := mock_apicore.NewMockObjectSearchService(t)
	search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
			mu.Lock()
			defer mu.Unlock()
			sub := newFakeObjectSearch(snapshot...)
			opens = append(opens, req)
			subs = append(subs, sub)
			return sub, nil
		}).Maybe()
	fx.withObjectSearch(search)
	return &opens, &subs
}

func decodeSearchFrames(t *testing.T, frames []StreamFrame) []v2model.SearchStreamEvent {
	t.Helper()
	out := make([]v2model.SearchStreamEvent, 0, len(frames))
	for _, frame := range frames {
		var event v2model.SearchStreamEvent
		require.NoError(t, json.Unmarshal(frame.Data, &event))
		require.Equal(t, frame.Type, event.Type, "the SSE event name is the body's type")
		out = append(out, event)
	}
	return out
}

func TestOpenSearchStream(t *testing.T) {
	ctx := context.Background()
	mentioned := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"unread_mention_count", "discussion"}}

	t.Run("a space outside the key's grant is refused before anything opens", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.unusedObjectSearch(t)
		granted := util.CtxWithApiGrant(ctx, &util.ApiGrant{Spaces: []string{"other-space"}, Perms: util.GrantPermsRead})

		// when
		_, err := fx.OpenSearchStream(granted, testSpaceId, mentioned)

		// then
		requireV2Code(t, err, v2model.CodeSpaceNotGranted)
	})

	t.Run("an unknown space is a 404 before anything opens", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.unusedObjectSearch(t)

		// when
		_, err := fx.OpenSearchStream(ctx, "no-such-space", mentioned)

		// then
		requireV2Code(t, err, v2model.CodeNotFound)
	})

	for name, tc := range map[string]struct {
		req      v2model.SearchRequest
		wantCode string
		wantPath string
	}{
		"an unknown property":            {req: v2model.SearchRequest{Filter: "unread_mention > 0"}, wantCode: v2model.CodeValidationFailed, wantPath: "/filter"},
		"an unknown field":               {req: v2model.SearchRequest{Fields: []string{"no_such_field"}}, wantCode: v2model.CodeValidationFailed, wantPath: "/fields/0"},
		"both filter forms":              {req: v2model.SearchRequest{Filter: "unread_mention_count > 0", Filters: json.RawMessage(`[]`)}, wantCode: v2model.CodeAmbiguousInput, wantPath: "/filter"},
		"a full-text query":              {req: v2model.SearchRequest{Query: "plan"}, wantCode: v2model.CodeValidationFailed, wantPath: "/query"},
		"a match-everything filter node": {req: v2model.SearchRequest{Filters: json.RawMessage(`[{"operator":"and","filters":[]}]`)}, wantCode: v2model.CodeValidationFailed, wantPath: "/filters/0/filters"},
	} {
		t.Run(name+" is a 400 before anything opens", func(t *testing.T) {
			// given
			fx := bundledKeysSetup(t)
			fx.unusedObjectSearch(t)

			// when
			_, err := fx.OpenSearchStream(ctx, testSpaceId, tc.req)

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			assert.Equal(t, tc.wantCode, apiErr.Code)
			require.NotEmpty(t, apiErr.Issues)
			assert.Equal(t, tc.wantPath, apiErr.Issues[0].Path)
		})
	}

	t.Run("the open carries the plan POST search compiles, unlimited, and the keys the rows read", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		opens, _ := fx.openingSearch(t)
		plan, err := fx.buildSearchPlan(testSpaceId, mentioned, true, errKeysFor(ctx))
		require.NoError(t, err)
		want := apicore.ObjectSearchOpen{
			SpaceId: testSpaceId,
			Filters: plan.filters,
			Sorts:   plan.sorts,
			Keys:    []string{"unread_mention_count", "unreadMentionCount", "discussion"},
		}

		// when
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)

		// then
		require.NoError(t, err)
		defer stream.Close()
		require.Len(t, *opens, 1)
		assert.Equal(t, want, (*opens)[0])
	})

	t.Run("the creator recipe compiles to a creator leaf on the member id", func(t *testing.T) {
		// given: the agent's member id, as GET …/members/me serves it
		fx := bundledKeysSetup(t)
		opens, _ := fx.openingSearch(t)
		memberId := domain.NewParticipantId(testSpaceId, testAccountId)
		req := v2model.SearchRequest{Filter: `creator = "` + memberId + `"`, Fields: []string{"discussion"}}

		// when
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, req)

		// then
		require.NoError(t, err)
		defer stream.Close()
		require.Len(t, *opens, 1)
		filters := (*opens)[0].Filters
		require.NotEmpty(t, filters)
		assert.Equal(t, bundle.RelationKeyCreator, filters[0].RelationKey)
		assert.Equal(t, memberId, filters[0].Value.String())
		// and the same plan finds exactly the agent's object on POST search
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)
		require.NoError(t, err)
		assert.Equal(t, []string{"chore1"}, rowIds(rows))
	})

	t.Run("a failing open is an error and returns the slot", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).Return(nil, errors.New("space index closed")).Times(maxConcurrentSearchStreams + 1)
		fx.withObjectSearch(search)

		// when
		var err error
		for i := 0; i <= maxConcurrentSearchStreams; i++ {
			_, err = fx.OpenSearchStream(ctx, testSpaceId, mentioned)
		}

		// then: every open reached the port, the last one too
		require.ErrorContains(t, err, "space index closed")
	})

	t.Run("a panic in the open returns the slot", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		panicking := true
		var mu sync.Mutex
		search := mock_apicore.NewMockObjectSearchService(t)
		search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).RunAndReturn(
			func(context.Context, apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
				mu.Lock()
				defer mu.Unlock()
				if panicking {
					panic("store gone")
				}
				return newFakeObjectSearch(), nil
			})
		fx.withObjectSearch(search)

		// when
		assert.Panics(t, func() { _, _ = fx.OpenSearchStream(ctx, testSpaceId, mentioned) })
		mu.Lock()
		panicking = false
		mu.Unlock()

		// then: the full cap is still available
		for i := 0; i < maxConcurrentSearchStreams; i++ {
			stream, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)
			require.NoError(t, err, "stream %d", i)
			defer stream.Close()
		}
	})

	t.Run("the cap refuses with 429 naming the search streams, independently of the chat stream caps", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.openingSearch(t)
		chatSub := mock_apicore.NewMockChatSubscriptionService(t)
		chatSub.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).Return(newFakeSpaceChatSubscription(), nil)
		fx.withChatSub(chatSub)
		open := make([]*SearchStream, 0, maxConcurrentSearchStreams)
		for i := 0; i < maxConcurrentSearchStreams; i++ {
			stream, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)
			require.NoError(t, err, "stream %d", i)
			open = append(open, stream)
		}

		// when
		_, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
		assert.Equal(t, v2model.CodeTooManyStreams, apiErr.Code)
		assert.Contains(t, apiErr.Message, "search streams")
		spaceChats, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})
		require.NoError(t, err, "the space chat stream cap is separate")
		spaceChats.Close()

		// and a closed stream returns its slot
		open[0].Close()
		reopened, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)
		require.NoError(t, err)
		reopened.Close()
		for _, stream := range open[1:] {
			stream.Close()
		}
	})

	t.Run("Close ends the live search exactly once", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		_, subs := fx.openingSearch(t)
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, mentioned)
		require.NoError(t, err)

		// when
		stream.Close()
		stream.Close()

		// then
		require.Len(t, *subs, 1)
		assert.Equal(t, 1, (*subs)[0].closeCount())
	})
}

func TestSearchStreamFrames(t *testing.T) {
	ctx := context.Background()
	mentionedDetails := func(mentions int64) *domain.Details {
		return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:                 domain.String("mentioned"),
			bundle.RelationKeyName:               domain.String("Mentioned page"),
			bundle.RelationKeyType:               domain.String("type-page"),
			bundle.RelationKeyDiscussionId:       domain.String("disc-mentioned"),
			bundle.RelationKeyUnreadMentionCount: domain.Int64(mentions),
		})
	}
	row := func(mentions float64) *v2model.ObjectRow {
		return &v2model.ObjectRow{Id: "mentioned", Name: "Mentioned page", Type: "page", Discussion: "disc-mentioned",
			Properties: map[string]any{"unread_mention_count": mentions}}
	}
	req := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"unread_mention_count"}}

	t.Run("the snapshot is an object_added per object with its search row, then snapshot_complete", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.openingSearch(t,
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: "mentioned", Details: mentionedDetails(2)},
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete},
		)
		want := []v2model.SearchStreamEvent{
			{Type: "object_added", SpaceId: testSpaceId, Object: row(2)},
			{Type: "snapshot_complete", SpaceId: testSpaceId},
		}

		// when
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, req)

		// then
		require.NoError(t, err)
		defer stream.Close()
		assert.Equal(t, want, decodeSearchFrames(t, stream.Snapshot()))
	})

	t.Run("live changes render as object_added, object_updated and object_removed", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		_, subs := fx.openingSearch(t)
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, req)
		require.NoError(t, err)
		defer stream.Close()
		want := []v2model.SearchStreamEvent{
			{Type: "object_added", SpaceId: testSpaceId, Object: row(1)},
			{Type: "object_updated", SpaceId: testSpaceId, Object: row(4)},
			{Type: "object_removed", SpaceId: testSpaceId, ObjectId: "mentioned"},
		}

		// when
		(*subs)[0].push(
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: "mentioned", Details: mentionedDetails(1)},
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: "mentioned", Details: mentionedDetails(4)},
			apicore.ObjectSearchChange{Type: apicore.ObjectSearchRemoved, Id: "mentioned"},
		)
		<-stream.Ready()

		// then
		assert.Equal(t, want, decodeSearchFrames(t, stream.Drain()))
	})

	t.Run("an object that synced before its type renders the type once the type arrives", func(t *testing.T) {
		// given: a row rendered while its type object is not indexed yet
		fx := bundledKeysSetup(t)
		_, subs := fx.openingSearch(t)
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, v2model.SearchRequest{})
		require.NoError(t, err)
		defer stream.Close()
		memo := func(id, name string) *domain.Details {
			return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String(id),
				bundle.RelationKeyName: domain.String(name),
				bundle.RelationKeyType: domain.String("type-memo"),
			})
		}
		(*subs)[0].push(apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: "memo1", Details: memo("memo1", "First memo")})
		<-stream.Ready()
		early := decodeSearchFrames(t, stream.Drain())
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-memo"),
			bundle.RelationKeyName:      domain.String("Memo"),
			bundle.RelationKeyUniqueKey: domain.String("ot-memo"),
		})
		want := []v2model.SearchStreamEvent{{Type: "object_updated", SpaceId: testSpaceId, Object: &v2model.ObjectRow{Id: "memo1", Name: "First memo, edited", Type: "memo"}}}

		// when
		(*subs)[0].push(apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: "memo1", Details: memo("memo1", "First memo, edited")})
		<-stream.Ready()

		// then
		require.Len(t, early, 1)
		assert.Empty(t, early[0].Object.Type, "no type is known yet")
		assert.Equal(t, want, decodeSearchFrames(t, stream.Drain()))
	})

	t.Run("the request's warnings ride snapshot_complete", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.openingSearch(t)

		// when
		stream, err := fx.OpenSearchStream(ctx, testSpaceId, v2model.SearchRequest{Filter: `lastModifiedDate < today()`})

		// then
		require.NoError(t, err)
		defer stream.Close()
		events := decodeSearchFrames(t, stream.Snapshot())
		require.Len(t, events, 1)
		require.Len(t, events[0].Warnings, 1)
		assert.Contains(t, events[0].Warnings[0].Message, "also matches objects with no lastModifiedDate")
	})
}
