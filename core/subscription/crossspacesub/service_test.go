package crossspacesub

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
		_ = a.Close(context.Background())
	})

	return &fixture{
		service:      s.(*service),
		objectStore:  objectStore,
		spaceService: spaceService,
		eventQueue:   eventQueue,
	}
}

func TestSubscribe(t *testing.T) {
	t.Run("no initial spaces", func(t *testing.T) {
		fx := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := fx.Subscribe(subscriptionservice.SubscribeRequest{
			Keys: []string{bundle.RelationKeyId.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.NotEmpty(t, resp.SubId)
		assert.Empty(t, resp.Records)
		assert.Empty(t, resp.Dependencies)

		fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("spaceView1"),
				bundle.RelationKeyTargetSpaceId: pbtypes.String("space1"),
				bundle.RelationKeyLayout:        pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		})

		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:     pbtypes.String("participant1"),
				bundle.RelationKeyLayout: pbtypes.Int64(int64(model.ObjectType_participant)),
			},
		})
		msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)
		_ = msgs

		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:     pbtypes.String("participant3"),
				bundle.RelationKeyLayout: pbtypes.Int64(int64(model.ObjectType_participant)),
			},
		})

		msgs, err = fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
		require.NoError(t, err)
		_ = msgs
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
