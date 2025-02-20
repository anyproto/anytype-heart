package identity

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	mock_nameserviceclient "github.com/anyproto/any-sync/nameservice/nameserviceclient/mock"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type ownSubscriptionFixture struct {
	*ownProfileSubscription

	accountService     *mock_account.MockService
	objectStoreFixture *objectstore.StoreFixture
	spaceService       *mock_space.MockService
	fileAclService     *mock_fileacl.MockService
	techSpace          *mock_techspace.MockTechSpace
	coordinatorClient  *inMemoryIdentityRepo
	testObserver       *testObserver
}

type testObserver struct {
	lock     sync.Mutex
	profiles []*model.IdentityProfile
}

func (t *testObserver) broadcastMyIdentityProfile(identityProfile *model.IdentityProfile) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.profiles = append(t.profiles, identityProfile)
}

func (t *testObserver) listObservedProfiles() []*model.IdentityProfile {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.profiles
}

const (
	testProfileObjectId = "testProfileObjectId"
	testBatchTimeout    = 100 * time.Millisecond
)

func newOwnSubscriptionFixture(t *testing.T) *ownSubscriptionFixture {
	accountService := mock_account.NewMockService(t)
	spaceService := mock_space.NewMockService(t)
	objectStore := objectstore.NewStoreFixture(t)
	techSpace := mock_techspace.NewMockTechSpace(t)
	coordinatorClient := newInMemoryIdentityRepo()
	fileAclService := mock_fileacl.NewMockService(t)

	testObserver := &testObserver{}
	ctrl := gomock.NewController(t)
	nsClient := mock_nameserviceclient.NewMockAnyNsClientService(ctrl)

	nsClient.EXPECT().GetNameByAnyId(gomock.Any(), &nameserviceproto.NameByAnyIdRequest{AnyAddress: testIdentity}).AnyTimes().
		Return(&nameserviceproto.NameByAddressResponse{
			Found: true,
			Name:  globalName,
		}, nil)

	accountService.EXPECT().AccountID().Return("identity1")

	identityProfileCacheStore, err := keyvaluestore.New(objectStore.GetCommonDb(), "identity_profile", keyvaluestore.BytesMarshal, keyvaluestore.BytesUnmarshal)
	require.NoError(t, err)
	identityGlobalNameCacheStore, err := keyvaluestore.New(objectStore.GetCommonDb(), "global_name", keyvaluestore.StringMarshal, keyvaluestore.StringUnmarshal)
	require.NoError(t, err)

	sub := newOwnProfileSubscription(spaceService, objectStore, accountService, coordinatorClient, fileAclService, testObserver, nsClient, testBatchTimeout, identityGlobalNameCacheStore, identityProfileCacheStore)

	return &ownSubscriptionFixture{
		ownProfileSubscription: sub,
		spaceService:           spaceService,
		coordinatorClient:      coordinatorClient,
		testObserver:           testObserver,
		objectStoreFixture:     objectStore,
		techSpace:              techSpace,
		fileAclService:         fileAclService,
		accountService:         accountService,
	}
}

func (fx *ownSubscriptionFixture) getDataFromTestRepo(t *testing.T, accountSymKey crypto.SymKey) *model.IdentityProfile {
	data, err := fx.identityRepoClient.IdentityRepoGet(context.Background(), []string{"identity1"}, []string{identityRepoDataKind})
	require.NoError(t, err)
	require.Len(t, data, 1)

	profile, _, err := extractProfile(data[0], accountSymKey)
	require.NoError(t, err)
	return profile
}

func TestOwnProfileSubscription(t *testing.T) {
	newName := "foobar"
	t.Run("do not take global name from profile details", func(t *testing.T) {
		fx := newOwnSubscriptionFixture(t)
		fx.accountService.EXPECT().AccountID().Return("identity1")
		fx.spaceService.EXPECT().GetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
		fx.techSpace.EXPECT().AccountObjectId().Return(testProfileObjectId, nil)
		fx.spaceService.EXPECT().TechSpaceId().Return("space1")
		accountSymKey := crypto.NewAES()
		fx.spaceService.EXPECT().AccountMetadataSymKey().Return(accountSymKey)
		fx.accountService.EXPECT().SignData(mock.Anything).RunAndReturn(func(data []byte) ([]byte, error) {
			privKey, _, err := crypto.GenerateRandomEd25519KeyPair()
			if err != nil {
				return nil, err
			}
			return privKey.Sign(data)
		})

		fx.fileAclService.EXPECT().GetInfoForFileSharing("fileObjectId").Return("fileCid1", []*model.FileEncryptionKey{
			{
				Path: "/0/original",
				Key:  "key1",
			},
		}, nil)

		err := fx.run(context.Background())
		require.NoError(t, err)

		time.Sleep(testBatchTimeout / 4)

		fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String(testProfileObjectId),
				bundle.RelationKeySpaceId:     domain.String("space1"),
				bundle.RelationKeyGlobalName:  domain.String("foobar"),
				bundle.RelationKeyIconImage:   domain.String("fileObjectId"),
				bundle.RelationKeyName:        domain.String("John Doe"),
				bundle.RelationKeyDescription: domain.String("Description"),
			},
		})

		time.Sleep(2 * testBatchTimeout)

		got := fx.testObserver.listObservedProfiles()

		// first we update profile details from store, then globalName from NS
		want := []*model.IdentityProfile{
			{
				Identity:   "identity1",
				GlobalName: globalName,
			},
			{
				Identity:    "identity1",
				Name:        "John Doe",
				Description: "Description",
				IconCid:     "fileObjectId",
				GlobalName:  globalName,
			},
		}
		assert.Equal(t, want, got)

		gotProfile := fx.getDataFromTestRepo(t, accountSymKey)
		wantProfile := &model.IdentityProfile{
			Identity:    "identity1",
			Name:        "John Doe",
			Description: "Description",
			IconCid:     "fileCid1",
			IconEncryptionKeys: []*model.FileEncryptionKey{
				{
					Path: "/0/original",
					Key:  "key1",
				},
			},
			GlobalName: globalName,
		}
		assert.Equal(t, wantProfile, gotProfile)
	})

	t.Run("rewrite global name from channel signal", func(t *testing.T) {
		fx := newOwnSubscriptionFixture(t)
		fx.accountService.EXPECT().AccountID().Return("identity1")
		fx.spaceService.EXPECT().GetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
		fx.techSpace.EXPECT().AccountObjectId().Return(testProfileObjectId, nil)
		fx.spaceService.EXPECT().TechSpaceId().Return("space1")
		accountSymKey := crypto.NewAES()
		fx.spaceService.EXPECT().AccountMetadataSymKey().Return(accountSymKey)
		fx.accountService.EXPECT().SignData(mock.Anything).RunAndReturn(func(data []byte) ([]byte, error) {
			privKey, _, err := crypto.GenerateRandomEd25519KeyPair()
			if err != nil {
				return nil, err
			}
			return privKey.Sign(data)
		})

		err := fx.run(context.Background())
		require.NoError(t, err)

		time.Sleep(testBatchTimeout / 4)

		fx.updateGlobalName(newName)

		time.Sleep(2 * testBatchTimeout)

		got := fx.testObserver.listObservedProfiles()

		// first we initialize globalName with the one from NS
		want := []*model.IdentityProfile{
			{
				Identity:   testIdentity,
				GlobalName: globalName,
			},
			{
				Identity:   testIdentity,
				GlobalName: newName,
			},
		}
		assert.Equal(t, want, got)

		gotProfile := fx.getDataFromTestRepo(t, accountSymKey)
		wantProfile := &model.IdentityProfile{
			Identity:   testIdentity,
			GlobalName: newName,
		}

		assert.Equal(t, wantProfile, gotProfile)
	})

	t.Run("push profile to identity repo in batches", func(t *testing.T) {
		fx := newOwnSubscriptionFixture(t)
		fx.accountService.EXPECT().AccountID().Return("identity1")
		fx.spaceService.EXPECT().GetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
		fx.techSpace.EXPECT().AccountObjectId().Return(testProfileObjectId, nil)
		fx.spaceService.EXPECT().TechSpaceId().Return("space1")
		accountSymKey := crypto.NewAES()
		fx.spaceService.EXPECT().AccountMetadataSymKey().Return(accountSymKey)
		fx.accountService.EXPECT().SignData(mock.Anything).RunAndReturn(func(data []byte) ([]byte, error) {
			privKey, _, err := crypto.GenerateRandomEd25519KeyPair()
			if err != nil {
				return nil, err
			}
			return privKey.Sign(data)
		})

		fx.fileAclService.EXPECT().GetInfoForFileSharing("fileObjectId2").Return("fileCid2", []*model.FileEncryptionKey{
			{
				Path: "/0/original",
				Key:  "key2",
			},
		}, nil)

		err := fx.run(context.Background())
		require.NoError(t, err)

		time.Sleep(testBatchTimeout / 4)

		fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String(testProfileObjectId),
				bundle.RelationKeySpaceId:     domain.String("space1"),
				bundle.RelationKeyGlobalName:  domain.String("foobar"),
				bundle.RelationKeyName:        domain.String("John Doe"),
				bundle.RelationKeyDescription: domain.String("Description"),
			},
		})
		time.Sleep(testBatchTimeout / 4)

		fx.updateGlobalName(newName)
		time.Sleep(testBatchTimeout / 4)

		fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String(testProfileObjectId),
				bundle.RelationKeySpaceId:     domain.String("space1"),
				bundle.RelationKeyGlobalName:  domain.String("foobar2"),
				bundle.RelationKeyIconImage:   domain.String("fileObjectId2"),
				bundle.RelationKeyName:        domain.String("John Doe2"),
				bundle.RelationKeyDescription: domain.String("Description2"),
			},
		})
		time.Sleep(testBatchTimeout / 4)

		time.Sleep(testBatchTimeout)

		got := fx.testObserver.listObservedProfiles()

		want := []*model.IdentityProfile{
			{
				Identity:   "identity1",
				GlobalName: globalName,
			},
			{
				Identity:    "identity1",
				Name:        "John Doe",
				Description: "Description",
				GlobalName:  globalName,
			},
			{
				Identity:    "identity1",
				Name:        "John Doe",
				Description: "Description",
				GlobalName:  newName,
			},
			{
				Identity:    "identity1",
				Name:        "John Doe2",
				Description: "Description2",
				IconCid:     "fileObjectId2",
				GlobalName:  newName,
			},
		}
		assert.Equal(t, want, got)

		gotProfile := fx.getDataFromTestRepo(t, accountSymKey)
		wantProfile := &model.IdentityProfile{
			Identity:    "identity1",
			Name:        "John Doe2",
			Description: "Description2",
			IconCid:     "fileCid2",
			GlobalName:  newName,
			IconEncryptionKeys: []*model.FileEncryptionKey{
				{
					Path: "/0/original",
					Key:  "key2",
				},
			},
		}
		assert.Equal(t, wantProfile, gotProfile)
	})
}

func TestWaitForDetails(t *testing.T) {
	fx := newOwnSubscriptionFixture(t)
	fx.accountService.EXPECT().AccountID().Return("identity1")
	fx.spaceService.EXPECT().GetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
	fx.techSpace.EXPECT().AccountObjectId().Return(testProfileObjectId, nil)
	fx.spaceService.EXPECT().TechSpaceId().Return("space1")
	accountSymKey := crypto.NewAES()
	fx.spaceService.EXPECT().AccountMetadataSymKey().Return(accountSymKey)
	fx.accountService.EXPECT().SignData(mock.Anything).RunAndReturn(func(data []byte) ([]byte, error) {
		privKey, _, err := crypto.GenerateRandomEd25519KeyPair()
		if err != nil {
			return nil, err
		}
		return privKey.Sign(data)
	})

	err := fx.run(context.Background())
	require.NoError(t, err)

	fx.updateGlobalName(globalName)
	time.Sleep(2 * testBatchTimeout)

	t.Run("ignore when only global name is updated", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		identity, metadataKey, details := fx.getDetails(ctx)
		assert.Empty(t, identity)
		assert.Nil(t, metadataKey)
		assert.Nil(t, details)
	})

	fx.fileAclService.EXPECT().GetInfoForFileSharing("fileObjectId").Return("fileCid1", []*model.FileEncryptionKey{
		{
			Path: "/0/original",
			Key:  "key1",
		},
	}, nil)
	fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:          domain.String(testProfileObjectId),
			bundle.RelationKeySpaceId:     domain.String("space1"),
			bundle.RelationKeyGlobalName:  domain.String("foobar"),
			bundle.RelationKeyIconImage:   domain.String("fileObjectId"),
			bundle.RelationKeyName:        domain.String("John Doe"),
			bundle.RelationKeyDescription: domain.String("Description"),
		},
	})
	time.Sleep(2 * testBatchTimeout)

	t.Run("expect ok when profile is updated", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		identity, metadataKey, details := fx.getDetails(ctx)
		assert.Equal(t, testIdentity, identity)
		assert.Equal(t, accountSymKey, metadataKey)

		wantDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String(testProfileObjectId),
			bundle.RelationKeyName:        domain.String("John Doe"),
			bundle.RelationKeyDescription: domain.String("Description"),
			bundle.RelationKeyGlobalName:  domain.String(globalName),
			bundle.RelationKeyIconImage:   domain.String("fileObjectId"),
		})
		assert.Equal(t, wantDetails, details)
	})
}

func TestStartWithError(t *testing.T) {
	fx := newOwnSubscriptionFixture(t)
	fx.spaceService.EXPECT().GetTechSpace(mock.Anything).Return(nil, fmt.Errorf("space error"))

	t.Run("GetMyProfileDetails before run with cancelled input context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		identity, key, details := fx.getDetails(ctx)
		assert.Empty(t, identity)
		assert.Nil(t, key)
		assert.Nil(t, details)
	})

	err := fx.run(context.Background())
	require.Error(t, err)

	fx.close()

	done := make(chan struct{})

	go func() {
		_, _, _ = fx.getDetails(context.Background())
		close(done)
	}()

	select {
	case <-time.After(time.Second):
		t.Fatal("GetMyProfileDetails should not block")
	case <-done:
	}
}
