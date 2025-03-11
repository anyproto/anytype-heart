package crossspacesub

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
			givenSpaceViewObject("spaceView1", "space1", model.Account_Active, model.SpaceStatus_Ok),
		})

		// Subscribe
		resp, err := fx.Subscribe(givenRequest())
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
			makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto()),
			makeAddEvent(resp.SubId, obj1.Id()),
			makeCountersEvent(resp.SubId, 1),
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
				makeDetailsAmendEvent(resp.SubId, obj1.Id(), []*pb.EventObjectDetailsAmendKeyValue{
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
		resp, err := fx.Subscribe(givenRequest())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records)
		assert.Empty(t, resp.Dependencies)

		t.Run("add first space", func(t *testing.T) {
			// Add space view
			fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
				givenSpaceViewObject("spaceView1", "space1", model.Account_Active, model.SpaceStatus_Ok),
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
				makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto()),
				makeAddEvent(resp.SubId, obj1.Id()),
				makeCountersEvent(resp.SubId, 1),
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
				makeDetailsSetEvent(resp.SubId, obj2.Details().ToProto()),
				makeAddEvent(resp.SubId, obj2.Id()),
				makeCountersEvent(resp.SubId, 2),
			}
			assert.Equal(t, want, msgs)
		})

		t.Run("add second space", func(t *testing.T) {
			// Add space view
			fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
				givenSpaceViewObject("spaceView2", "space2", model.Account_Active, model.SpaceStatus_Ok),
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
				makeDetailsSetEvent(resp.SubId, obj1.Details().ToProto()),
				makeAddEvent(resp.SubId, obj1.Id()),
				makeCountersEvent(resp.SubId, 3),
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
			givenSpaceViewObject("spaceView1", "space1", model.Account_Active, model.SpaceStatus_Ok),
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
		resp, err := fx.Subscribe(givenRequest())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.SubId)
		assert.Equal(t, []*domain.Details{obj1.Details(), obj2.Details()}, resp.Records)

		// Remove space view by changing its status
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.Account_Deleted, model.SpaceStatus_Unknown),
		})

		// Wait events
		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)

		want := []*pb.EventMessage{
			makeRemoveEvent(resp.SubId, obj1.Id()),
			makeRemoveEvent(resp.SubId, obj2.Id()),
			makeCountersEvent(resp.SubId, 0),
		}
		assert.Equal(t, want, msgs)
	})
}

func TestUnsubscribe(t *testing.T) {
	t.Run("subscription not found", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.Unsubscribe("subId")
		require.Error(t, err)
	})

	t.Run("with existing subscription", func(t *testing.T) {
		fx := newFixture(t)

		// Add space view
		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", "space1", model.Account_Active, model.SpaceStatus_Ok),
		})

		// Subscribe
		resp, err := fx.Subscribe(givenRequest())
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

func makeDetailsSetEvent(subId string, details *types.Struct) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id: pbtypes.GetString(details, bundle.RelationKeyId.String()),
			SubIds: []string{
				subId,
			},
			Details: details,
		},
	})
}

func makeDetailsAmendEvent(subId string, id string, details []*pb.EventObjectDetailsAmendKeyValue) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfObjectDetailsAmend{
		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Id: id,
			SubIds: []string{
				subId,
			},
			Details: details,
		},
	})
}

func makeAddEvent(subId string, id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
			SubId:   subId,
			Id:      id,
			AfterId: "",
		},
	})
}

func makeCountersEvent(subId string, total int) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
			SubId: subId,
			Total: int64(total),
		},
	})
}

func makeRemoveEvent(subId string, id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfSubscriptionRemove{
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

func (d *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) {
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

func givenSpaceViewObject(id string, targetSpaceId string, accountStatus model.AccountStatusType, localStatus model.SpaceStatus) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(accountStatus)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(localStatus)),
	}
}
