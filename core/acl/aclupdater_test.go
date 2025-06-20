package acl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"

	"github.com/anyproto/anytype-heart/core/acl/mock_acl"
)

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

type aclUpdaterFixture struct {
	*aclUpdater

	objectStore     *objectstore.StoreFixture
	remover         *mock_acl.MockparticipantRemover
	eventQueue      *mb.MB[*pb.EventMessage]
	spaceService    *mock_space.MockService
	crossSpaceSub   crossspacesub.Service
	pubKeys         []crypto.PubKey
	testOwnIdentity string
	techSpaceId     string
	spaceIds        []string
}

func newAclUpdaterFixture(t *testing.T) *aclUpdaterFixture {
	ctx := context.Background()
	a := &app.App{}

	eventQueue := mb.New[*pb.EventMessage](0)

	var pubKeys []crypto.PubKey
	for i := 0; i < 10; i++ {
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		pubKeys = append(pubKeys, pubKey)
	}

	testOwnIdentity := pubKeys[0].Account()

	techSpaceId := "tech." + bson.NewObjectId().Hex()

	var spaceIds []string
	for i := 0; i < 5; i++ {
		spaceIds = append(spaceIds, fmt.Sprintf("space%d.%d", i, i))
	}

	kanbanService := mock_kanban.NewMockService(t)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		for _, msg := range e.Messages {
			eventQueue.Add(context.Background(), msg)
		}
	}).Maybe()
	objectStore := objectstore.NewStoreFixture(t)
	collService := &dummyCollectionService{}
	subscriptionService := subscriptionservice.New()
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return(techSpaceId).Maybe()

	a.Register(testutil.PrepareMock(ctx, a, kanbanService))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(objectStore)
	a.Register(collService)
	a.Register(subscriptionService)
	a.Register(testutil.PrepareMock(ctx, a, spaceService))

	crossSpaceSub := crossspacesub.New()
	a.Register(crossSpaceSub)

	err := a.Start(ctx)
	require.NoError(t, err)

	remover := mock_acl.NewMockparticipantRemover(t)

	updater := newAclUpdater(
		"test-updater",
		testOwnIdentity,
		crossSpaceSub,
		remover,
		100*time.Millisecond,
		1*time.Second,
		1*time.Second,
	)

	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = updater.Close()
		assert.NoError(t, err)
		err = a.Close(closeCtx)
		require.NoError(t, err)
	})

	return &aclUpdaterFixture{
		aclUpdater:      updater,
		objectStore:     objectStore,
		remover:         remover,
		eventQueue:      eventQueue,
		spaceService:    spaceService,
		crossSpaceSub:   crossSpaceSub,
		pubKeys:         pubKeys,
		testOwnIdentity: testOwnIdentity,
		techSpaceId:     techSpaceId,
		spaceIds:        spaceIds,
	}
}

func givenSpaceViewObject(id string, targetSpaceId string, creator string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceActive)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(model.SpaceStatus_Ok)),
		bundle.RelationKeyCreator:            domain.String(creator),
	}
}

func givenParticipantObject(spaceId string, identity string, status model.ParticipantStatus) objectstore.TestObject {
	participantId := domain.NewParticipantId(spaceId, identity)
	return objectstore.TestObject{
		bundle.RelationKeyId:                domain.String(participantId),
		bundle.RelationKeySpaceId:           domain.String(spaceId),
		bundle.RelationKeyIdentity:          domain.String(identity),
		bundle.RelationKeyLayout:            domain.Int64(int64(model.ObjectType_participant)),
		bundle.RelationKeyParticipantStatus: domain.Int64(int64(status)),
	}
}

func TestAclUpdater_Run(t *testing.T) {
	t.Run("processes removing participants from own spaces", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		removingIdentity := fx.pubKeys[1].Account()
		spaceId := fx.spaceIds[0]

		done := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.MatchedBy(func(identities []crypto.PubKey) bool {
			if len(identities) != 1 {
				return false
			}
			return identities[0].Account() == removingIdentity
		})).Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
			close(done)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})

		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, removingIdentity, model.ParticipantStatus_Removing),
		})

		<-done
	})

	t.Run("ignores participants from spaces not owned by us", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", fx.spaceIds[0], fx.pubKeys[2].Account()),
		})

		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, fx.spaceIds[0], []objectstore.TestObject{
			givenParticipantObject(fx.spaceIds[0], fx.pubKeys[3].Account(), model.ParticipantStatus_Removing),
		})

		time.Sleep(200 * time.Millisecond)
	})

	t.Run("handles multiple removing participants", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		identity1 := fx.pubKeys[1].Account()
		identity2 := fx.pubKeys[2].Account()
		spaceId := fx.spaceIds[0]

		done1 := make(chan struct{})
		done2 := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.MatchedBy(func(identities []crypto.PubKey) bool {
			return len(identities) == 1 && identities[0].Account() == identity1
		})).Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
			close(done1)
		}).Return(nil).Once()

		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.MatchedBy(func(identities []crypto.PubKey) bool {
			return len(identities) == 1 && identities[0].Account() == identity2
		})).Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
			close(done2)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})

		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity1, model.ParticipantStatus_Removing),
			givenParticipantObject(spaceId, identity2, model.ParticipantStatus_Removing),
		})

		<-done1
		<-done2
	})

	t.Run("retries on error with backoff", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		identity := fx.pubKeys[1].Account()
		spaceId := fx.spaceIds[0]

		retryDone := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.Anything).
			Return(assert.AnError).Once()
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.Anything).
			Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
				close(retryDone)
			}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})

		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity, model.ParticipantStatus_Removing),
		})

		<-retryDone
	})

	t.Run("stops retrying on ErrRequestNotExists", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		identity := fx.pubKeys[1].Account()
		spaceId := fx.spaceIds[0]

		errorDone := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.Anything).
			Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
				close(errorDone)
			}).Return(ErrRequestNotExists).Once()
		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})
		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity, model.ParticipantStatus_Removing),
		})
		<-errorDone
	})

	t.Run("handles participant status change from removing to active", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		identity := fx.pubKeys[1].Account()
		spaceId := fx.spaceIds[0]

		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})
		err := fx.Run(ctx)
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity, model.ParticipantStatus_Active),
		})
		time.Sleep(100 * time.Millisecond)

		statusDone := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.Anything).
			Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
				close(statusDone)
			}).Return(nil).Once()
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity, model.ParticipantStatus_Removing),
		})
		<-statusDone
	})

	t.Run("handles space addition after updater start", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := fx.Run(ctx)
		require.NoError(t, err)

		spaceId := fx.spaceIds[0]
		fx.objectStore.AddObjects(t, fx.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", spaceId, fx.testOwnIdentity),
		})

		identity := fx.pubKeys[1].Account()
		spaceDone := make(chan struct{})
		fx.remover.EXPECT().ApproveLeave(mock.Anything, spaceId, mock.Anything).
			Run(func(ctx context.Context, spaceId string, identities []crypto.PubKey) {
				close(spaceDone)
			}).Return(nil).Once()

		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			givenParticipantObject(spaceId, identity, model.ParticipantStatus_Removing),
		})

		<-spaceDone
	})
}

func TestAclUpdater_Close(t *testing.T) {
	t.Run("closes cleanly", func(t *testing.T) {
		fx := newAclUpdaterFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := fx.Run(ctx)
		require.NoError(t, err)

		err = fx.Close()
		assert.NoError(t, err)
	})
}
