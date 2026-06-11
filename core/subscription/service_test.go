package subscription

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const testSpaceId = "space1"

type engineFixture struct {
	Service
	objectStore       *objectstore.StoreFixture
	collectionService *MockCollectionService
	// broadcastEvents collects every pb.Event the engine broadcasts; one
	// element per Broadcast call so payload grouping is assertable
	broadcastEvents *mb.MB[*pb.Event]
}

func newEngineFixture(t *testing.T) *engineFixture {
	ctx := context.Background()
	a := &app.App{}

	broadcastEvents := mb.New[*pb.Event](0)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		_ = broadcastEvents.Add(context.Background(), e)
	}).Maybe()

	objectStore := objectstore.NewStoreFixture(t)
	collService := NewMockCollectionService(t)
	s := New()

	a.Register(objectStore)
	a.Register(kanban.New())
	a.Register(&collectionServiceMock{MockCollectionService: collService})
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(s)
	require.NoError(t, a.Start(ctx))

	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, a.Close(closeCtx))
	})

	return &engineFixture{
		Service:           s,
		objectStore:       objectStore,
		collectionService: collService,
		broadcastEvents:   broadcastEvents,
	}
}

func givenParticipant(id string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
	}
}

func givenParticipantRequest() SubscribeRequest {
	return SubscribeRequest{
		SpaceId:           testSpaceId,
		SubId:             "test-sub",
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeyResolvedLayout.String(), bundle.RelationKeyName.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
		},
	}
}

func detailsSetEvent(subId, spaceId string, obj objectstore.TestObject) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id:      obj.Id(),
			Details: obj.Details().ToProto(),
			SubIds:  []string{subId},
		},
	})
}

func addEvent(subId, spaceId, id string) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: id, SubId: subId},
	})
}

func removeEvent(subId, spaceId, id string) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionRemove{
		SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: id, SubId: subId},
	})
}

func countersEvent(subId, spaceId string, total int64) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{Total: total, SubId: subId},
	})
}

func waitMessages(t *testing.T, queue *mb.MB[*pb.EventMessage], min int) []*pb.EventMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgs, err := queue.NewCond().WithMin(min).Wait(ctx)
	require.NoError(t, err)
	return msgs
}

func assertNoMessages(t *testing.T, queue *mb.MB[*pb.EventMessage]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	msgs, err := queue.NewCond().WithMin(1).Wait(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, msgs)
}

func TestInternalSubscriptionLifecycle(t *testing.T) {
	fx := newEngineFixture(t)

	resp, err := fx.Search(givenParticipantRequest())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "test-sub", resp.SubId)
	assert.Empty(t, resp.Records)
	require.NotNil(t, resp.Counters)
	assert.Zero(t, resp.Counters.Total)
	require.NotNil(t, resp.Output)

	t.Run("matching object appears: Set, Add, Counters", func(t *testing.T) {
		obj := givenParticipant("participant1")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		want := []*pb.EventMessage{
			detailsSetEvent("test-sub", testSpaceId, obj),
			addEvent("test-sub", testSpaceId, "participant1"),
			countersEvent("test-sub", testSpaceId, 1),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 3))
	})

	t.Run("requested key changes: minimal Amend only", func(t *testing.T) {
		obj := givenParticipant("participant1")
		obj[bundle.RelationKeyName] = domain.String("Alice")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		want := []*pb.EventMessage{
			event.NewMessage(testSpaceId, &pb.EventMessageValueOfObjectDetailsAmend{
				ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
					Id: "participant1",
					Details: []*pb.EventObjectDetailsAmendKeyValue{
						{Key: bundle.RelationKeyName.String(), Value: pbtypes.String("Alice")},
					},
					SubIds: []string{"test-sub"},
				},
			}),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 1))
	})

	t.Run("non-requested key changes: no events", func(t *testing.T) {
		obj := givenParticipant("participant1")
		obj[bundle.RelationKeyName] = domain.String("Alice")
		obj[bundle.RelationKeyDescription] = domain.String("not requested")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		assertNoMessages(t, resp.Output)
	})

	t.Run("requested key disappears: Unset", func(t *testing.T) {
		obj := givenParticipant("participant1")
		obj[bundle.RelationKeyDescription] = domain.String("not requested")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		want := []*pb.EventMessage{
			event.NewMessage(testSpaceId, &pb.EventMessageValueOfObjectDetailsUnset{
				ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
					Id:     "participant1",
					Keys:   []string{bundle.RelationKeyName.String()},
					SubIds: []string{"test-sub"},
				},
			}),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 1))
	})

	t.Run("object stops matching: Remove, Counters", func(t *testing.T) {
		obj := givenParticipant("participant1")
		obj[bundle.RelationKeyResolvedLayout] = domain.Int64(int64(model.ObjectType_basic))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		want := []*pb.EventMessage{
			removeEvent("test-sub", testSpaceId, "participant1"),
			countersEvent("test-sub", testSpaceId, 0),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 2))
	})
}

func TestSearchSnapshot(t *testing.T) {
	t.Run("records are projected to requested keys, in query order", func(t *testing.T) {
		fx := newEngineFixture(t)
		obj1 := givenParticipant("participant1")
		obj2 := givenParticipant("participant2")
		obj2[bundle.RelationKeyDescription] = domain.String("not requested")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj1, obj2})

		resp, err := fx.Search(givenParticipantRequest())
		require.NoError(t, err)

		want := []*domain.Details{obj1.Details(), givenParticipant("participant2").Details()}
		assert.Equal(t, want, resp.Records)
		assert.Equal(t, int64(2), resp.Counters.Total)
	})

	t.Run("limit and offset truncate the snapshot, not the live set", func(t *testing.T) {
		fx := newEngineFixture(t)
		objs := []objectstore.TestObject{
			givenParticipant("participant1"),
			givenParticipant("participant2"),
			givenParticipant("participant3"),
		}
		fx.objectStore.AddObjects(t, testSpaceId, objs)

		req := givenParticipantRequest()
		req.Limit = 1
		req.Offset = 1
		resp, err := fx.Search(req)
		require.NoError(t, err)

		require.Len(t, resp.Records, 1)
		assert.Equal(t, "participant2", resp.Records[0].GetString(bundle.RelationKeyId))
		assert.Equal(t, int64(3), resp.Counters.Total)

		// live tracking covers the full set: an out-of-snapshot member emits
		obj := givenParticipant("participant3")
		obj[bundle.RelationKeyName] = domain.String("Charlie")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
		msgs := waitMessages(t, resp.Output, 1)
		require.Len(t, msgs, 1)
		assert.NotNil(t, msgs[0].GetObjectDetailsAmend())
	})
}

func TestAsyncInit(t *testing.T) {
	t.Run("snapshot flows as events", func(t *testing.T) {
		fx := newEngineFixture(t)
		obj := givenParticipant("participant1")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		req := givenParticipantRequest()
		req.AsyncInit = true
		resp, err := fx.Search(req)
		require.NoError(t, err)
		assert.Empty(t, resp.Records)

		want := []*pb.EventMessage{
			detailsSetEvent("test-sub", testSpaceId, obj),
			addEvent("test-sub", testSpaceId, "participant1"),
			countersEvent("test-sub", testSpaceId, 1),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 3))
	})

	t.Run("empty snapshot emits nothing, not even zero counters", func(t *testing.T) {
		fx := newEngineFixture(t)

		req := givenParticipantRequest()
		req.AsyncInit = true
		resp, err := fx.Search(req)
		require.NoError(t, err)

		assertNoMessages(t, resp.Output)
	})
}

func TestResubscribeSameSubId(t *testing.T) {
	t.Run("replaces the subscription silently", func(t *testing.T) {
		fx := newEngineFixture(t)
		obj := givenParticipant("participant1")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		resp1, err := fx.Search(givenParticipantRequest())
		require.NoError(t, err)
		require.Len(t, resp1.Records, 1)

		// re-subscribe with a filter matching nothing: no Remove events for
		// the old generation, the response supersedes the client state
		req := givenParticipantRequest()
		req.Filters = []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_todo)),
		}}
		resp2, err := fx.Search(req)
		require.NoError(t, err)
		assert.Empty(t, resp2.Records)

		assert.Equal(t, []string{"test-sub"}, fx.SubscriptionIDs())

		// the old generation no longer tracks: a participant change reaches
		// only the new queue (which doesn't match) — no events anywhere
		obj[bundle.RelationKeyName] = domain.String("Alice")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
		assertNoMessages(t, resp2.Output)
	})

	t.Run("caller-provided queue survives replacement", func(t *testing.T) {
		fx := newEngineFixture(t)
		queue := mb.New[*pb.EventMessage](0)

		req := givenParticipantRequest()
		req.InternalQueue = queue
		_, err := fx.Search(req)
		require.NoError(t, err)

		req2 := givenParticipantRequest()
		req2.InternalQueue = queue
		resp2, err := fx.Search(req2)
		require.NoError(t, err)
		assert.Same(t, queue, resp2.Output)

		// the shared queue must not be closed by the replacement teardown
		obj := givenParticipant("participant1")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
		assert.Len(t, waitMessages(t, queue, 3), 3)
	})
}

func TestUnsubscribe(t *testing.T) {
	t.Run("unknown subId is ignored", func(t *testing.T) {
		fx := newEngineFixture(t)
		require.NoError(t, fx.Unsubscribe("unknown"))
	})

	t.Run("stops events and closes the engine-owned queue", func(t *testing.T) {
		fx := newEngineFixture(t)
		resp, err := fx.Search(givenParticipantRequest())
		require.NoError(t, err)

		require.NoError(t, fx.Unsubscribe("test-sub"))
		assert.Empty(t, fx.SubscriptionIDs())

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{givenParticipant("participant1")})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = resp.Output.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, mb.ErrClosed)
	})

	t.Run("caller-provided queue is not closed", func(t *testing.T) {
		fx := newEngineFixture(t)
		queue := mb.New[*pb.EventMessage](0)
		req := givenParticipantRequest()
		req.InternalQueue = queue
		_, err := fx.Search(req)
		require.NoError(t, err)

		require.NoError(t, fx.Unsubscribe("test-sub"))
		require.NoError(t, queue.Add(context.Background(), &pb.EventMessage{}))
	})
}

// TestUnsubscribeAndReturnIdsConsistency pins the ghost-record invariant:
// every membership event for the sub is delivered before
// UnsubscribeAndReturnIds returns, and the returned ids reflect exactly the
// tracked set those events produced — crossspacesub synthesizes Remove
// events from the list, so a stale Add delivered later (or an id missing
// from the list whose Add was delivered) resurrects a ghost record.
func TestUnsubscribeAndReturnIdsConsistency(t *testing.T) {
	fx := newEngineFixture(t)
	queue := mb.New[*pb.EventMessage](0)

	for gen := 0; gen < 30; gen++ {
		subId := fmt.Sprintf("uari-%d", gen)
		req := givenParticipantRequest()
		req.SubId = subId
		req.InternalQueue = queue
		req.AsyncInit = true
		_, err := fx.Search(req)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for w := 0; w < 3; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < 4; i++ {
					id := fmt.Sprintf("obj-%d-%d-%d", gen, w, i)
					fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{givenParticipant(id)})
				}
			}(w)
		}
		time.Sleep(time.Duration(gen%3) * time.Millisecond)

		ids, err := fx.UnsubscribeAndReturnIds(testSpaceId, subId)
		require.NoError(t, err)
		wg.Wait()

		returned := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			returned[id] = struct{}{}
		}
		tracked := make(map[string]struct{})
		for _, msg := range queue.GetAll() {
			if add := msg.GetSubscriptionAdd(); add != nil {
				require.Equal(t, subId, add.SubId)
				tracked[add.Id] = struct{}{}
			}
			if rem := msg.GetSubscriptionRemove(); rem != nil {
				delete(tracked, rem.Id)
			}
		}
		for id := range tracked {
			_, ok := returned[id]
			require.Truef(t, ok, "gen %d: Add(%s) was delivered but the id is not in the returned list — ghost record", gen, id)
		}

		// nothing for this generation may arrive after the call returned
		time.Sleep(10 * time.Millisecond)
		for _, msg := range queue.GetAll() {
			if add := msg.GetSubscriptionAdd(); add != nil {
				require.NotEqualf(t, subId, add.SubId, "gen %d: stale Add delivered after UnsubscribeAndReturnIds returned", gen)
			}
		}
	}
}

func TestUnsubscribeAndReturnIds(t *testing.T) {
	fx := newEngineFixture(t)
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		givenParticipant("participant1"),
		givenParticipant("participant2"),
	})

	_, err := fx.Search(givenParticipantRequest())
	require.NoError(t, err)

	ids, err := fx.UnsubscribeAndReturnIds(testSpaceId, "test-sub")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"participant1", "participant2"}, ids)

	_, err = fx.UnsubscribeAndReturnIds(testSpaceId, "test-sub")
	require.Error(t, err)
}

func TestBroadcastSubscription(t *testing.T) {
	fx := newEngineFixture(t)

	req := givenParticipantRequest()
	req.Internal = false
	resp, err := fx.Search(req)
	require.NoError(t, err)
	assert.Nil(t, resp.Output)

	obj := givenParticipant("participant1")
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events, err := fx.broadcastEvents.NewCond().WithMin(1).Wait(ctx)
	require.NoError(t, err)

	// one logical change arrives as one pb.Event payload
	require.Len(t, events, 1)
	want := []*pb.EventMessage{
		detailsSetEvent("test-sub", testSpaceId, obj),
		addEvent("test-sub", testSpaceId, "participant1"),
		countersEvent("test-sub", testSpaceId, 1),
	}
	assert.Equal(t, want, events[0].Messages)
}
