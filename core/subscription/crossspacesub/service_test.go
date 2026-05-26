package crossspacesub

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	*service

	objectStore  *objectstore.StoreFixture
	spaceService *mock_space.MockService
	eventQueue   *mb.MB[*pb.EventMessage]
}

const techSpaceId = "techSpaceId"

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	a := &app.App{}

	eventQueue := mb.New[*pb.EventMessage](0)

	// Deps for subscription service
	kanbanService := mock_kanban.NewMockService(t)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		for _, msg := range e.Messages {
			eventQueue.Add(context.Background(), msg)
		}
	}).Maybe()
	objectStore := objectstore.NewStoreFixture(t)
	collService := &dummyCollectionService{}
	// Own deps
	subscriptionService := subscriptionservice.New()
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return(techSpaceId).Maybe()

	a.Register(testutil.PrepareMock(ctx, a, kanbanService))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(objectStore)
	a.Register(collService)
	a.Register(subscriptionService)
	a.Register(testutil.PrepareMock(ctx, a, spaceService))

	s := New()
	a.Register(s)
	err := a.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = a.Close(closeCtx)
		require.NoError(t, err)
	})

	return &fixture{
		service:      s.(*service),
		objectStore:  objectStore,
		spaceService: spaceService,
		eventQueue:   eventQueue,
	}
}

func TestSubscribe(t *testing.T) {
	t.Run("with existing space", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Add space view and objects
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})

		// Subscribe
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records)
		assert.Empty(t, resp.Dependencies)

		// Add objects
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			obj1,
		})

		// Wait events
		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)

		want := []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
			makeAddEvent(resp.SubId, obj1.Id(), "space1"),
			makeCountersEvent(resp.SubId, 1, "space1"),
		}
		assert.Equal(t, want, msgs)

		t.Run("update object", func(t *testing.T) {
			obj1 = objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("participant1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
				bundle.RelationKeyName:           domain.String("John Doe"),
			}
			fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
				obj1,
			})

			// Wait events
			msgs, err = fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
			require.NoError(t, err)

			want = []*pb.EventMessage{
				makeDetailsAmendEvent(resp.SubId, obj1.Id(), "space1", []*pb.EventObjectDetailsAmendKeyValue{
					{
						Key:   bundle.RelationKeyName.String(),
						Value: pbtypes.String("John Doe"),
					},
				}),
			}
			assert.Equal(t, want, msgs)
		})
	})

	t.Run("without existing space", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Subscribe
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records)
		assert.Empty(t, resp.Dependencies)

		t.Run("add first space", func(t *testing.T) {
			// Add space view
			fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
				givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			})

			// Add objects
			obj1 := objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("participant1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
			}
			fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
				obj1,
			})

			// Wait events
			msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
			require.NoError(t, err)

			want := []*pb.EventMessage{
				makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
				makeAddEvent(resp.SubId, obj1.Id(), "space1"),
				makeCountersEvent(resp.SubId, 1, "space1"),
			}
			assert.Equal(t, want, msgs)

			// Add another objects
			obj2 := objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("participant2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
			}
			fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
				obj2,
			})

			// Wait events
			msgs, err = fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
			require.NoError(t, err)

			want = []*pb.EventMessage{
				makeDetailsSetEvent(resp.SubId, obj2.Details().ToProto(), "space1"),
				makeAddEvent(resp.SubId, obj2.Id(), "space1"),
				makeCountersEvent(resp.SubId, 2, "space1"),
			}
			assert.Equal(t, want, msgs)
		})

		t.Run("add second space", func(t *testing.T) {
			// Add space view
			fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
				givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			})

			// Add objects
			obj1 := objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("participant3"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
			}
			fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{
				obj1,
			})

			// Wait events
			msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
			require.NoError(t, err)

			want := []*pb.EventMessage{
				makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space2"),
				makeAddEvent(resp.SubId, obj1.Id(), "space2"),
				makeCountersEvent(resp.SubId, 3, "space2"),
			}
			assert.Equal(t, want, msgs)
		})

	})

	t.Run("remove space view", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Add space view and objects
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			obj1,
			obj2,
		})

		// Subscribe
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Equal(t, []*domain.Details{obj1.Details(), obj2.Details()}, resp.Records)

		// Remove space view by changing its status
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Unknown),
		})

		// Wait events
		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)

		want := []*pb.EventMessage{
			makeRemoveEvent(resp.SubId, obj1.Id(), "space1"),
			makeRemoveEvent(resp.SubId, obj2.Id(), "space1"),
			makeCountersEvent(resp.SubId, 0, "space1"),
		}
		assert.Equal(t, want, msgs)
	})

	t.Run("local status of space is changed to loading", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		// Add space view and objects
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			obj1,
			obj2,
		})

		// Subscribe
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Equal(t, []*domain.Details{obj1.Details(), obj2.Details()}, resp.Records)

		// Change status to loading. It reflects how it could work in real application.
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Loading),
		})

		// Nothing happens
		_, err = fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestSubscribeWithPredicate(t *testing.T) {
	t.Run("predicate filters initial spaces", func(t *testing.T) {
		fx := newFixture(t)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView3", "space3", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})

		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj3 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant3"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{obj2})
		fx.objectStore.AddObjects(t, "space3", []objectstore.TestObject{obj3})

		// Wait for space view subscription to process the space views
		time.Sleep(500 * time.Millisecond)

		predicate := func(details *domain.Details) bool {
			targetSpaceId := details.GetString(bundle.RelationKeyTargetSpaceId)
			return targetSpaceId == "space1" || targetSpaceId == "space3"
		}

		resp, err := fx.Subscribe(givenRequest(), predicate)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)

		assert.Len(t, resp.Records, 2)
		recordIds := make([]string, len(resp.Records))
		for i, record := range resp.Records {
			recordIds[i] = record.GetString(bundle.RelationKeyId)
		}
		slices.Sort(recordIds)
		assert.ElementsMatch(t, []string{"participant1", "participant3"}, recordIds)
		assert.Equal(t, int64(2), resp.Counters.Total)
	})

	t.Run("predicate filters when adding spaces dynamically", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		predicate := func(details *domain.Details) bool {
			targetSpaceId := details.GetString(bundle.RelationKeyTargetSpaceId)
			return targetSpaceId == "space1" || targetSpaceId == "space21"
		}

		resp, err := fx.Subscribe(givenRequest(), predicate)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records) // No initial spaces match

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})

		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)
		want := []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
			makeAddEvent(resp.SubId, obj1.Id(), "space1"),
			makeCountersEvent(resp.SubId, 1, "space1"),
		}
		assert.Equal(t, want, msgs)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{obj2})

		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		msgs, err = fx.eventQueue.NewCond().WithMin(1).Wait(ctx2)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Empty(t, msgs)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView21", "space21", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj21 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant21"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space21", []objectstore.TestObject{obj21})

		msgs, err = fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)
		want = []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj21.Details().ToProto(), "space21"),
			makeAddEvent(resp.SubId, obj21.Id(), "space21"),
			makeCountersEvent(resp.SubId, 2, "space21"),
		}
		assert.Equal(t, want, msgs)
	})

	t.Run("predicate that rejects all spaces", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})

		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{obj2})

		predicate := func(details *domain.Details) bool {
			return false
		}

		resp, err := fx.Subscribe(givenRequest(), predicate)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records)
		assert.Equal(t, int64(0), resp.Counters.Total)

		obj3 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant3"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj3})

		msgs, err := fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Empty(t, msgs)
	})

	t.Run("predicate filters based on space view properties", func(t *testing.T) {
		fx := newFixture(t)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObjectWithCreator("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok, "participant1"),
			givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView3", "space3", model.SpaceStatus_SpaceJoining, model.SpaceStatus_Unknown),
		})

		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj3 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant3"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{obj2})
		fx.objectStore.AddObjects(t, "space3", []objectstore.TestObject{obj3})

		time.Sleep(500 * time.Millisecond)

		predicate := func(details *domain.Details) bool {
			accountStatus := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))
			creatorId := details.GetString(bundle.RelationKeyCreator)
			return accountStatus == model.SpaceStatus_SpaceActive && creatorId == "participant1"
		}

		resp, err := fx.Subscribe(givenRequest(), predicate)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)

		assert.Len(t, resp.Records, 1)
		if len(resp.Records) > 0 {
			assert.Equal(t, "participant1", resp.Records[0].GetString(bundle.RelationKeyId))
		}
		assert.Equal(t, int64(1), resp.Counters.Total)
	})

	t.Run("predicate filters space that changes to match criteria", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Initially create space view with creator that doesn't match predicate
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObjectWithCreator("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok, "wrongCreator"),
		})

		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})

		time.Sleep(500 * time.Millisecond)

		// Predicate only matches spaces with creator "targetCreator"
		predicate := func(details *domain.Details) bool {
			accountStatus := model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))
			creatorId := details.GetString(bundle.RelationKeyCreator)
			return accountStatus == model.SpaceStatus_SpaceActive && creatorId == "targetCreator"
		}

		resp, err := fx.Subscribe(givenRequest(), predicate)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)

		// Initially no records should match
		assert.Empty(t, resp.Records)
		assert.Equal(t, int64(0), resp.Counters.Total)

		// Now update the space view to have the matching creator
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObjectWithCreator("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok, "targetCreator"),
		})

		// Wait for events - the participant should now be included
		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)

		want := []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
			makeAddEvent(resp.SubId, obj1.Id(), "space1"),
			makeCountersEvent(resp.SubId, 1, "space1"),
		}
		assert.Equal(t, want, msgs)
	})
}

// TestLazySubscribe_SearchFirstOpenerDoesNotDeadlock guards against a
// self-deadlock: subscription.getSpaceSubscriptions holds the service lock
// across objectStore.SpaceIndex; when that SpaceIndex is the first open of a
// space pending in a cross-space subscription, its OnSpaceIndexOpened callback
// re-enters subscriptionService.Search on the same goroutine, which tries to
// re-acquire the same (non-reentrant) lock.
func TestLazySubscribe_SearchFirstOpenerDoesNotDeadlock(t *testing.T) {
	fx := newFixture(t)

	// Spaceview for space1 exists; space1's objectstore stays closed.
	fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
		givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
	})
	time.Sleep(500 * time.Millisecond) // let the tech-space sub propagate

	// Cross-space subscription: space1 matches the predicate but is not open,
	// so it is recorded as pending.
	resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
	require.NoError(t, err)
	sub := fx.subscriptions[resp.SubId]
	require.NotNil(t, sub)
	sub.lock.Lock()
	_, pending := sub.pendingSpaceIds["space1"]
	sub.lock.Unlock()
	require.True(t, pending, "space1 must be pending for this repro")

	// Make subscriptionService.Search the FIRST opener of space1.
	done := make(chan error, 1)
	go func() {
		_, e := fx.subscriptionService.Search(subscriptionservice.SubscribeRequest{
			SubId:             "ui-sub-space1",
			SpaceId:           "space1",
			NoDepSubscription: true,
			Keys:              []string{bundle.RelationKeyId.String()},
		})
		done <- e
	}()
	select {
	case e := <-done:
		require.NoError(t, e)
	case <-time.After(5 * time.Second):
		t.Fatal("Search(space1) deadlocked: getSpaceSubscriptions holds the service lock across SpaceIndex, whose OnSpaceIndexOpened callback re-enters the subscription service")
	}
}

func TestLazySubscribe(t *testing.T) {
	t.Run("subscribe returns only loaded spaces initially", func(t *testing.T) {
		fx := newFixture(t)

		// Three matching spaceviews, but only space1 and space2 have data
		// added (which opens their objectstores). space3 stays closed.
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView2", "space2", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
			givenSpaceViewObject("spaceView3", "space3", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj2 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{obj2})

		// Let the tech-space sub propagate all three spaceview details
		time.Sleep(500 * time.Millisecond)

		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Only space1 and space2 are opened, so only their records appear
		recordIds := make([]string, len(resp.Records))
		for i, r := range resp.Records {
			recordIds[i] = r.GetString(bundle.RelationKeyId)
		}
		slices.Sort(recordIds)
		assert.Equal(t, []string{"participant1", "participant2"}, recordIds)

		// space3 must be tracked as pending on the subscription
		sub := fx.subscriptions[resp.SubId]
		require.NotNil(t, sub)
		sub.lock.Lock()
		_, isPending := sub.pendingSpaceIds["space3"]
		_, isSubscribed := sub.perSpaceSubscriptions["space3"]
		sub.lock.Unlock()
		assert.True(t, isPending, "space3 should be pending")
		assert.False(t, isSubscribed, "space3 should not have a per-space subscription yet")
	})

	t.Run("pending space promotes on open", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Spaceview exists for space1, but space1's objectstore stays closed
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		time.Sleep(200 * time.Millisecond)

		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		assert.Empty(t, resp.Records)

		// Confirm space1 is pending and not subscribed before open
		sub := fx.subscriptions[resp.SubId]
		require.NotNil(t, sub)
		sub.lock.Lock()
		_, pendingBefore := sub.pendingSpaceIds["space1"]
		_, subscribedBefore := sub.perSpaceSubscriptions["space1"]
		sub.lock.Unlock()
		assert.True(t, pendingBefore)
		assert.False(t, subscribedBefore)

		// Open space1's objectstore by adding an object — this should
		// fire the callback, promote space1 from pending, and stream
		// the new record as Add+DetailsSet+Counters events.
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})

		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)

		want := []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
			makeAddEvent(resp.SubId, obj1.Id(), "space1"),
			makeCountersEvent(resp.SubId, 1, "space1"),
		}
		assert.Equal(t, want, msgs)

		sub.lock.Lock()
		_, pendingAfter := sub.pendingSpaceIds["space1"]
		_, subscribedAfter := sub.perSpaceSubscriptions["space1"]
		sub.lock.Unlock()
		assert.False(t, pendingAfter, "space1 should no longer be pending")
		assert.True(t, subscribedAfter, "space1 should now have a per-space subscription")
	})

	t.Run("spaceview removed before space opens drops pending entry", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		time.Sleep(200 * time.Millisecond)

		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		assert.Empty(t, resp.Records)

		sub := fx.subscriptions[resp.SubId]
		require.NotNil(t, sub)
		sub.lock.Lock()
		_, pendingBefore := sub.pendingSpaceIds["space1"]
		sub.lock.Unlock()
		require.True(t, pendingBefore)

		// Flip the spaceview status to Deleted: the tech-space filter
		// excludes deleted statuses, so the spaceview is removed and
		// space1 should be dropped from pending without emitting events.
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Unknown),
		})
		time.Sleep(200 * time.Millisecond)

		sub.lock.Lock()
		_, pendingAfter := sub.pendingSpaceIds["space1"]
		_, subscribedAfter := sub.perSpaceSubscriptions["space1"]
		sub.lock.Unlock()
		assert.False(t, pendingAfter, "pending entry should be cleared on spaceview removal")
		assert.False(t, subscribedAfter)

		// No events expected within the window
		_, err = fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("new matching spaceview waits when space is closed", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Subscribe with no matching spaceviews yet
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		assert.Empty(t, resp.Records)

		// Add a spaceview pointing to space1 — but do NOT open space1
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		time.Sleep(200 * time.Millisecond)

		sub := fx.subscriptions[resp.SubId]
		require.NotNil(t, sub)
		sub.lock.Lock()
		_, pending := sub.pendingSpaceIds["space1"]
		_, subscribed := sub.perSpaceSubscriptions["space1"]
		sub.lock.Unlock()
		assert.True(t, pending, "dynamically added spaceview should land in pending")
		assert.False(t, subscribed)

		// Open space1 by adding an object — promotion + events follow
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj1})

		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)
		want := []*pb.EventMessage{
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto(), "space1"),
			makeAddEvent(resp.SubId, obj1.Id(), "space1"),
			makeCountersEvent(resp.SubId, 1, "space1"),
		}
		assert.Equal(t, want, msgs)
	})
}

// TestLazySubscribe_NoDataLossUnderConcurrentOpen reproduces the exact race
// the goroutine-id machinery was meant to guard: for a space that is pending
// in a cross-space subscription, its objectstore is opened concurrently from
// multiple goroutines while another goroutine is hammering writes into it.
//
// The data-loss hypothesis: a write that lands after the store is open but
// before the per-space subscription's SubscribeForAll is wired is dropped.
// If the subscription service's "persist, then re-query after wiring"
// design holds, every written object must still be delivered to the
// cross-space subscription regardless of interleaving.
//
// Run with -race and -count to exercise many interleavings.
func TestLazySubscribe_NoDataLossUnderConcurrentOpen(t *testing.T) {
	const (
		numSpaces      = 24
		objectsPerSpc  = 40
		drainTimeout   = 20 * time.Second
		settleSpaceViw = 500 * time.Millisecond
	)

	fx := newFixture(t)

	// Register spaceviews for every space, but do NOT open the spaces yet.
	spaceViews := make([]objectstore.TestObject, 0, numSpaces)
	for k := 0; k < numSpaces; k++ {
		spaceViews = append(spaceViews, givenSpaceViewObject(
			fmt.Sprintf("sv-%d", k), fmt.Sprintf("space-%d", k),
			model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok))
	}
	fx.objectStore.AddObjects(t, techSpaceId, spaceViews)

	// Let the tech-space monitor process all spaceviews into the sub's
	// pending set before we subscribe.
	time.Sleep(settleSpaceViw)

	resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Records, "all spaces start pending (none opened yet)")

	// Build the expected set of object ids.
	expected := make(map[string]struct{}, numSpaces*objectsPerSpc)
	for k := 0; k < numSpaces; k++ {
		for i := 0; i < objectsPerSpc; i++ {
			expected[fmt.Sprintf("p-%d-%d", k, i)] = struct{}{}
		}
	}

	var writeErrs sync.Map // id -> error
	var wg sync.WaitGroup

	for k := 0; k < numSpaces; k++ {
		spaceId := fmt.Sprintf("space-%d", k)

		// Opener: contends on the first open so the wiring callback may run
		// on this goroutine while the writer races ahead on its own.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 64; j++ {
				_ = fx.objectStore.SpaceIndex(spaceId)
			}
		}()

		// Writer: writes objectsPerSpc participant objects as fast as
		// possible, re-fetching the store each iteration (the first such
		// call may itself trigger the open + callback).
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			for i := 0; i < objectsPerSpc; i++ {
				id := fmt.Sprintf("p-%d-%d", k, i)
				obj := objectstore.TestObject{
					bundle.RelationKeyId:             domain.String(id),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
				}
				store := fx.objectStore.SpaceIndex(spaceId)
				if werr := store.UpdateObjectDetails(context.Background(), id, obj.Details()); werr != nil {
					writeErrs.Store(id, werr)
				}
			}
		}(k)
	}
	wg.Wait()

	writeErrs.Range(func(key, value any) bool {
		t.Errorf("write failed for %v: %v", key, value)
		return true
	})

	// Drain delivered SubscriptionAdd events for our cross-space subId and
	// collect the set of object ids that actually reached the subscription.
	delivered := make(map[string]struct{}, len(expected))
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	for len(delivered) < len(expected) {
		msgs, werr := fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
		if werr != nil {
			break // timeout: stop draining, report below
		}
		for _, msg := range msgs {
			if add := msg.GetSubscriptionAdd(); add != nil && add.SubId == resp.SubId {
				delivered[add.Id] = struct{}{}
			}
		}
	}

	var missing []string
	for id := range expected {
		if _, ok := delivered[id]; !ok {
			missing = append(missing, id)
		}
	}
	slices.Sort(missing)
	assert.Emptyf(t, missing,
		"DATA LOSS: %d/%d objects never reached the cross-space subscription: %v",
		len(missing), len(expected), missing)
}

func TestUnsubscribe(t *testing.T) {
	t.Run("subscription not found", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.Unsubscribe("subId")
		require.Error(t, err)
	})

	t.Run("with existing subscription", func(t *testing.T) {
		fx := newFixture(t)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
		})
		resp, err := fx.Subscribe(givenRequest(), NoOpPredicate())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)

		// Unsubscribe
		err = fx.Unsubscribe(resp.SubId)
		require.NoError(t, err)

		// Add objects
		obj1 := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			obj1,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		// Wait events
		msgs, err := fx.eventQueue.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Empty(t, msgs)
	})
}

func makeDetailsSetEvent(subId string, details *types.Struct, spaceId string) *pb.EventMessage {
	id := pbtypes.GetString(details, bundle.RelationKeyId.String())
	return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id: id,
			SubIds: []string{
				subId,
			},
			Details: details,
		},
	})
}

func makeDetailsAmendEvent(subId string, id string, spaceId string, details []*pb.EventObjectDetailsAmendKeyValue) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsAmend{
		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Id: id,
			SubIds: []string{
				subId,
			},
			Details: details,
		},
	})
}

func makeAddEvent(subId string, id string, spaceId string) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
			SubId:   subId,
			Id:      id,
			AfterId: "",
		},
	})
}

func makeCountersEvent(subId string, total int, spaceId string) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
			SubId: subId,
			Total: int64(total),
		},
	})
}

func makeRemoveEvent(subId string, id string, spaceId string) *pb.EventMessage {
	return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionRemove{
		SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
			SubId: subId,
			Id:    id,
		},
	})
}

type dummyCollectionService struct{}

func (d *dummyCollectionService) Init(a *app.App) (err error) {
	return nil
}

func (d *dummyCollectionService) Name() (name string) {
	return "dummyCollectionService"
}

func (d *dummyCollectionService) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, nil, nil
}

func (d *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) error {
	return nil
}

func givenRequest() subscriptionservice.SubscribeRequest {
	return subscriptionservice.SubscribeRequest{
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

func givenSpaceViewObject(id string, targetSpaceId string, spaceStatus model.SpaceStatus, localStatus model.SpaceStatus) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(spaceStatus)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(localStatus)),
	}
}

func givenSpaceViewObjectWithCreator(id string, targetSpaceId string, spaceStatus model.SpaceStatus, localStatus model.SpaceStatus, creator string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(spaceStatus)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(localStatus)),
		bundle.RelationKeyCreator:            domain.String(creator),
	}
}
