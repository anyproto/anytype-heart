package identity

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	mock_nameserviceclient "github.com/anyproto/any-sync/nameservice/nameserviceclient/mock"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/mutex"
)

type fixture struct {
	*service
	coordinatorClient *inMemoryIdentityRepo
	spaceService      *mock_space.MockService
	accountService    *mock_account.MockService
	nsClient          *mock_nameserviceclient.MockAnyNsClientService
	objectStore       *objectstore.StoreFixture
	fileAclService    *mock_fileacl.MockService
}

const (
	globalName   = "anytypeuser.any"
	testIdentity = "identity1"
)

func newFixture(t *testing.T, testObserverPeriod time.Duration) *fixture {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	identityRepoClient := newInMemoryIdentityRepo()
	objectStore := objectstore.NewStoreFixture(t)
	accountService := mock_account.NewMockService(t)
	spaceService := mock_space.NewMockService(t)
	fileAclService := mock_fileacl.NewMockService(t)

	wallet := mock_wallet.NewMockWallet(t)
	nsClient := mock_nameserviceclient.NewMockAnyNsClientService(ctrl)
	nsClient.EXPECT().BatchGetNameByAnyId(gomock.Any(), &nameserviceproto.BatchNameByAnyIdRequest{AnyAddresses: []string{testIdentity}}).AnyTimes().
		Return(&nameserviceproto.BatchNameByAddressResponse{Results: []*nameserviceproto.NameByAddressResponse{{
			Found: true,
			Name:  globalName,
		}, {
			Found: false,
			Name:  "",
		},
		}}, nil)

	dbProvider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	a := new(app.App)
	a.Register(dbProvider)
	a.Register(objectStore)
	a.Register(identityRepoClient)
	a.Register(testutil.PrepareMock(ctx, a, accountService))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(testutil.PrepareMock(ctx, a, fileAclService))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(testutil.PrepareMock(ctx, a, nsClient))

	svc := New(testObserverPeriod, 1*time.Microsecond)
	err = svc.Init(a)
	t.Cleanup(func() {
		svc.Close(ctx)
	})
	require.NoError(t, err)

	svcRef := svc.(*service)
	// TODO
	// svcRef.currentProfileDetails = &types.Struct{Fields: make(map[string]*types.Value)}
	fx := &fixture{
		service:           svcRef,
		spaceService:      spaceService,
		accountService:    accountService,
		coordinatorClient: identityRepoClient,
		nsClient:          nsClient,
		objectStore:       objectStore,
		fileAclService:    fileAclService,
	}
	go fx.observeIdentitiesLoop()

	return fx
}

// addParticipant pre-creates a participant record: the identity fan-out is
// update-only, records are created by the ACL processing path
func (fx *fixture) addParticipant(t *testing.T, spaceId, identity string) {
	fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(domain.NewParticipantId(spaceId, identity)),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
		bundle.RelationKeyIdentity:       domain.String(identity),
		bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
	}})
}

func (fx *fixture) participantDetails(t *testing.T, spaceId, identity string) *domain.Details {
	details, err := fx.objectStore.SpaceIndex(spaceId).GetDetails(domain.NewParticipantId(spaceId, identity))
	require.NoError(t, err)
	return details
}

func (fx *fixture) participantHasName(spaceId, identity, name string) bool {
	details, err := fx.objectStore.SpaceIndex(spaceId).GetDetails(domain.NewParticipantId(spaceId, identity))
	if err != nil {
		return false
	}
	return details.GetString(bundle.RelationKeyName) == name
}

func marshalProfile(t *testing.T, profile *model.IdentityProfile, key crypto.SymKey) []byte {
	data, err := proto.Marshal(profile)
	require.NoError(t, err)
	data, err = key.Encrypt(data)
	require.NoError(t, err)
	return data
}

type inMemoryIdentityRepo struct {
	lock           sync.Mutex
	isUnavailable  bool
	getCallback    func(identities []string, kinds []string, res []*identityrepoproto.DataWithIdentity, resErr error)
	identitiesData map[string]*identityrepoproto.DataWithIdentity
}

func newInMemoryIdentityRepo() *inMemoryIdentityRepo {
	return &inMemoryIdentityRepo{
		identitiesData: make(map[string]*identityrepoproto.DataWithIdentity),
	}
}

func (d *inMemoryIdentityRepo) setCallback(callback func(identities []string, kinds []string, res []*identityrepoproto.DataWithIdentity, resErr error)) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.getCallback = callback
}

func (d *inMemoryIdentityRepo) Init(a *app.App) (err error) {
	return nil
}

func (d *inMemoryIdentityRepo) Name() (name string) {
	return "inMemoryIdentityRepo"
}

func (d *inMemoryIdentityRepo) setUnavailable() {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.isUnavailable = true
}

func (d *inMemoryIdentityRepo) IdentityRepoPut(ctx context.Context, identity string, data []*identityrepoproto.Data) (err error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.identitiesData[identity] = &identityrepoproto.DataWithIdentity{
		Identity: identity,
		Data:     data,
	}
	return nil
}

func (d *inMemoryIdentityRepo) IdentityRepoGet(ctx context.Context, identities []string, kinds []string) (res []*identityrepoproto.DataWithIdentity, err error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.isUnavailable {
		err := fmt.Errorf("network problem")
		if d.getCallback != nil {
			d.getCallback(identities, kinds, nil, err)
		}
		return nil, err
	}

	res = make([]*identityrepoproto.DataWithIdentity, 0, len(identities))
	for _, identity := range identities {
		if data, ok := d.identitiesData[identity]; ok {
			res = append(res, data)
		}
	}
	if d.getCallback != nil {
		d.getCallback(identities, kinds, res, err)
	}
	return
}

func TestIdentityProfileCache(t *testing.T) {
	t.Run("with available cache, use it while registering identity", func(t *testing.T) {
		fx := newFixture(t, time.Minute)

		spaceId := "space1"
		identity := "identity1"

		profileSymKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		wantProfile := &model.IdentityProfile{
			Identity: identity,
			Name:     "name1",
		}
		wantData := marshalProfile(t, wantProfile, profileSymKey)
		// Global name is cached separately
		wantProfile.GlobalName = globalName

		err = fx.service.identityProfileCacheStore.Set(context.Background(), identity, wantData)
		require.NoError(t, err)
		err = fx.service.identityGlobalNameCacheStore.Set(context.Background(), identity, globalName)
		require.NoError(t, err)

		fx.addParticipant(t, spaceId, identity)
		err = fx.RegisterIdentity(spaceId, identity, profileSymKey)
		require.NoError(t, err)

		// the cached profile is fanned out into the participant record synchronously
		details := fx.participantDetails(t, spaceId, identity)
		assert.Equal(t, wantProfile.Name, details.GetString(bundle.RelationKeyName))
		assert.Equal(t, globalName, details.GetString(bundle.RelationKeyGlobalName))
	})

	t.Run("with available cache and unavailable identity repo, use cache instead of remote service", func(t *testing.T) {
		testObserverPeriod := 10 * time.Millisecond
		fx := newFixture(t, testObserverPeriod)

		spaceId := "space1"
		identity := "identity1"

		fx.coordinatorClient.setUnavailable()

		profileSymKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		wantProfile := &model.IdentityProfile{
			Identity: identity,
			Name:     "name1",
		}
		wantData := marshalProfile(t, wantProfile, profileSymKey)
		// Global name is cached separately
		wantProfile.GlobalName = globalName

		err = fx.service.identityGlobalNameCacheStore.Set(context.Background(), identity, globalName)
		require.NoError(t, err)

		fx.addParticipant(t, spaceId, identity)
		err = fx.RegisterIdentity(spaceId, identity, profileSymKey)
		require.NoError(t, err)

		err = fx.service.identityProfileCacheStore.Set(context.Background(), identity, wantData)
		require.NoError(t, err)

		// the observe loop falls back to the cache store and fans out into the record
		require.Eventually(t, func() bool {
			return fx.participantHasName(spaceId, identity, wantProfile.Name)
		}, time.Second, testObserverPeriod)
	})
}

func TestEncryptionKeyPersistence(t *testing.T) {
	t.Run("nil key resolves from the persisted store after restart", func(t *testing.T) {
		// given
		fx := newFixture(t, time.Minute)
		identity := "identity1"
		profileSymKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		require.NoError(t, fx.RegisterIdentity("space1", identity, profileSymKey))

		// simulate a restart: the in-memory key cache is gone, only commonDb persists
		fx.lock.Lock()
		fx.identityEncryptionKeys = make(map[string]crypto.SymKey)
		fx.lock.Unlock()

		// when
		err = fx.RegisterIdentity("space2", identity, nil)

		// then
		require.NoError(t, err)
		gotKey, err := fx.GetMetadataKey(identity)
		require.NoError(t, err)
		assert.True(t, profileSymKey.Equals(gotKey))
	})

	t.Run("nil key for unknown identity fails", func(t *testing.T) {
		// given
		fx := newFixture(t, time.Minute)

		// when
		err := fx.RegisterIdentity("space1", "unknownIdentity", nil)

		// then
		require.Error(t, err)
	})

	t.Run("own identity key is derived and persisted on nil key", func(t *testing.T) {
		// given
		fx := newFixture(t, time.Minute)
		accKeys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.accountService.EXPECT().Keys().Return(accKeys).Maybe()
		fx.myIdentity = accKeys.SignKey.GetPublic().Account()

		// when
		err = fx.RegisterIdentity("space1", fx.myIdentity, nil)

		// then
		require.NoError(t, err)
		persisted, err := fx.identityEncryptionKeyStore.Get(context.Background(), fx.myIdentity)
		require.NoError(t, err)
		_, wantKey, err := domain.DeriveAccountMetadata(accKeys.SignKey)
		require.NoError(t, err)
		assert.True(t, wantKey.Equals(persisted))
	})
}

// newOwnProfileFixture sets up the fixture for the own identity. The icon keys can be read
// once keysReady is set; every change of the own profile is also pushed to the identity repo
// in the background.
func newOwnProfileFixture(t *testing.T, keysReady *atomic.Bool) *fixture {
	fx := newFixture(t, time.Minute)
	accKeys, err := accountdata.NewRandom()
	require.NoError(t, err)
	fx.myIdentity = accKeys.SignKey.GetPublic().Account()
	fx.ownProfileSubscription.myIdentity = fx.myIdentity
	fx.accountService.EXPECT().Keys().Return(accKeys).Maybe()
	fx.accountService.EXPECT().AccountID().Return(fx.myIdentity).Maybe()
	fx.accountService.EXPECT().SignData(mock.Anything).Return(nil, nil).Maybe()
	fx.spaceService.EXPECT().AccountMetadataSymKey().Return(crypto.NewAES()).Maybe()
	fx.fileAclService.EXPECT().GetInfoForFileSharing(mock.Anything).RunAndReturn(func(string) (string, []*model.FileEncryptionKey, error) {
		if !keysReady.Load() {
			return "", nil, fmt.Errorf("no file keys")
		}
		return "fileCid1", []*model.FileEncryptionKey{{Path: "/0/original", Key: "key1"}}, nil
	}).Maybe()
	return fx
}

func (fx *fixture) waitOwnProfile(t *testing.T) *model.IdentityProfile {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	profile, err := fx.WaitProfileWithKey(ctx, fx.myIdentity)
	require.NoError(t, err)
	return profile.IdentityProfile
}

// The one-to-one invite sender waits for the own profile with WaitProfileWithKey, so the
// own identity has to be in the profile cache like any other one.
func TestOwnProfileCache(t *testing.T) {
	t.Run("own profile is cached with the icon keys once its details are loaded", func(t *testing.T) {
		// given
		keysReady := &atomic.Bool{}
		keysReady.Store(true)
		fx := newOwnProfileFixture(t, keysReady)
		want := &model.IdentityProfile{
			Identity:           fx.myIdentity,
			Name:               "name2",
			IconCid:            "fileCid1",
			IconEncryptionKeys: []*model.FileEncryptionKey{{Path: "/0/original", Key: "key1"}},
			GlobalName:         globalName,
		}

		// when the global name arrives from the naming service before the profile details
		fx.ownProfileSubscription.handleGlobalNameUpdate(globalName)

		// then
		assert.Nil(t, fx.getProfileFromCache(fx.myIdentity))

		// when
		fx.ownProfileSubscription.handleOwnProfileDetails(domain.NewDetails().SetString(bundle.RelationKeyIconImage, "fileObjectId"))
		fx.ownProfileSubscription.handleOwnProfileDetails(domain.NewDetails().SetString(bundle.RelationKeyName, "name2"))

		// then the cached profile follows its changes
		assert.Equal(t, want, fx.waitOwnProfile(t))
	})

	t.Run("own profile is cached without the icon keys when they cannot be read", func(t *testing.T) {
		// given
		fx := newOwnProfileFixture(t, &atomic.Bool{})
		want := &model.IdentityProfile{
			Identity: fx.myIdentity,
			Name:     "name1",
			IconCid:  "fileObjectId",
		}

		// when
		fx.ownProfileSubscription.handleOwnProfileDetails(domain.NewDetails().
			SetString(bundle.RelationKeyName, "name1").
			SetString(bundle.RelationKeyIconImage, "fileObjectId"))

		// then
		assert.Equal(t, want, fx.waitOwnProfile(t))
	})

	t.Run("own profile gets the icon keys on refresh once they can be read", func(t *testing.T) {
		// given
		keysReady := &atomic.Bool{}
		fx := newOwnProfileFixture(t, keysReady)
		spaceId := "space1"
		fx.addParticipant(t, spaceId, fx.myIdentity)
		require.NoError(t, fx.RegisterIdentity(spaceId, fx.myIdentity, nil))
		fx.ownProfileSubscription.handleOwnProfileDetails(domain.NewDetails().
			SetString(bundle.RelationKeyName, "name1").
			SetString(bundle.RelationKeyIconImage, "fileObjectId"))
		want := &model.IdentityProfile{
			Identity:           fx.myIdentity,
			Name:               "name1",
			IconCid:            "fileCid1",
			IconEncryptionKeys: []*model.FileEncryptionKey{{Path: "/0/original", Key: "key1"}},
		}

		// when the icon file arrives from another device and the profile is pushed
		keysReady.Store(true)
		fx.refreshMyIdentityProfile()

		// then
		assert.Equal(t, want, fx.waitOwnProfile(t))
		assert.Equal(t, "fileCid1", fx.participantDetails(t, spaceId, fx.myIdentity).GetString(bundle.RelationKeyIconImage))
	})

	t.Run("persisted own profile is not used before the details are loaded", func(t *testing.T) {
		// given a profile without a name pushed by an older version before the details were loaded
		keysReady := &atomic.Bool{}
		keysReady.Store(true)
		fx := newOwnProfileFixture(t, keysReady)
		spaceId := "space1"
		_, myKey, err := domain.DeriveAccountMetadata(fx.accountService.Keys().SignKey)
		require.NoError(t, err)
		persisted := marshalProfile(t, &model.IdentityProfile{Identity: fx.myIdentity, GlobalName: globalName}, myKey)
		require.NoError(t, fx.identityProfileCacheStore.Set(context.Background(), fx.myIdentity, persisted))
		fx.addParticipant(t, spaceId, fx.myIdentity)
		want := &model.IdentityProfile{
			Identity:   fx.myIdentity,
			Name:       "name1",
			GlobalName: globalName,
		}

		// when
		require.NoError(t, fx.RegisterIdentity(spaceId, fx.myIdentity, nil))
		fx.ownProfileSubscription.handleGlobalNameUpdate(globalName)

		// then
		assert.Nil(t, fx.getProfileFromCache(fx.myIdentity))
		assert.Empty(t, fx.participantDetails(t, spaceId, fx.myIdentity).GetString(bundle.RelationKeyGlobalName))

		// when
		fx.ownProfileSubscription.handleOwnProfileDetails(domain.NewDetails().SetString(bundle.RelationKeyName, "name1"))

		// then
		assert.Equal(t, want, fx.waitOwnProfile(t))
		assert.Equal(t, "name1", fx.participantDetails(t, spaceId, fx.myIdentity).GetString(bundle.RelationKeyName))
	})
}

func TestObservers(t *testing.T) {
	testObserverPeriod := 10 * time.Millisecond
	fx := newFixture(t, testObserverPeriod)

	spaceId := "space1"
	identity := "identity1"

	profileSymKey, err := crypto.NewRandomAES()
	require.NoError(t, err)
	wantProfile := &model.IdentityProfile{
		Identity:   identity,
		Name:       "name1",
		GlobalName: globalName,
	}
	wantData := marshalProfile(t, wantProfile, profileSymKey)

	fx.addParticipant(t, spaceId, identity)
	err = fx.RegisterIdentity(spaceId, identity, profileSymKey)
	require.NoError(t, err)

	time.Sleep(testObserverPeriod * 2)

	err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
		{
			Kind: identityRepoDataKind,
			Data: wantData,
		},
	})
	require.NoError(t, err)

	// the profile from the identity repo is fanned out into the participant record
	require.Eventually(t, func() bool {
		return fx.participantHasName(spaceId, identity, "name1")
	}, time.Second, testObserverPeriod)

	t.Run("change profile's name", func(t *testing.T) {
		wantProfile2 := &model.IdentityProfile{
			Identity:    identity,
			Name:        "name1 edited",
			Description: "my description",
			GlobalName:  globalName,
		}
		wantData2 := marshalProfile(t, wantProfile2, profileSymKey)

		err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
			{
				Kind: identityRepoDataKind,
				Data: wantData2,
			},
		})
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return fx.participantHasName(spaceId, identity, "name1 edited")
		}, time.Second, testObserverPeriod)
		details := fx.participantDetails(t, spaceId, identity)
		assert.Equal(t, "my description", details.GetString(bundle.RelationKeyDescription))
		assert.Equal(t, globalName, details.GetString(bundle.RelationKeyGlobalName))
	})

	t.Run("a newly registered space gets the current profile", func(t *testing.T) {
		fx.addParticipant(t, "space2", identity)
		err = fx.RegisterIdentity("space2", identity, profileSymKey)
		require.NoError(t, err)

		// the in-memory cached profile is fanned out synchronously on registration
		details := fx.participantDetails(t, "space2", identity)
		assert.Equal(t, "name1 edited", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "my description", details.GetString(bundle.RelationKeyDescription))
	})
}

func TestGetIdentitiesDataFromRepo(t *testing.T) {
	t.Run("empty identities", func(t *testing.T) {
		fx := newFixture(t, time.Millisecond)

		called := mutex.NewValue(false)
		fx.coordinatorClient.setCallback(func(_ []string, _ []string, _ []*identityrepoproto.DataWithIdentity, _ error) {
			called.Set(true)
		})
		err := fx.observeIdentities(context.Background())
		require.NoError(t, err)
		require.False(t, called.Get())
	})
	t.Run("receive 100 identities", func(t *testing.T) {
		// given
		testObserverPeriod := time.Minute
		fx := newFixture(t, testObserverPeriod)
		nsServiceResult := make([]*nameserviceproto.NameByAddressResponse, 0, 100)
		var allIdentities []string
		for i := 0; i < 100; i++ {
			identity := fmt.Sprintf("identity%d", i)
			allIdentities = append(allIdentities, identity)
			profileSymKey, err := crypto.NewRandomAES()
			require.NoError(t, err)
			wantProfile := &model.IdentityProfile{
				Identity:   identity,
				Name:       "name1",
				GlobalName: globalName,
			}
			wantData := marshalProfile(t, wantProfile, profileSymKey)

			err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: wantData,
				},
			})
			require.NoError(t, err)
			fx.identityObservers[identity] = map[string]struct{}{"test": {}}
			fx.identityEncryptionKeys[identity] = profileSymKey
			fx.addParticipant(t, "test", identity)
			nsServiceResult = append(nsServiceResult, &nameserviceproto.NameByAddressResponse{
				Found: false,
				Name:  "",
			})
		}
		fx.nsClient.EXPECT().BatchGetNameByAnyId(gomock.Any(), gomock.Any()).Return(&nameserviceproto.BatchNameByAddressResponse{Results: nsServiceResult}, nil)

		// when
		fx.identityForceUpdate <- struct{}{}

		// then
		require.Eventually(t, func() bool {
			return allParticipantsHaveName(fx, "test", allIdentities, "name1")
		}, 5*time.Second, 10*time.Millisecond)
	})
	t.Run("receive more than 100 identities", func(t *testing.T) {
		// given
		testObserverPeriod := time.Duration(math.MaxInt) // make sure observing won't run by ticker
		fx := newFixture(t, testObserverPeriod)
		nsServiceResult := make([]*nameserviceproto.NameByAddressResponse, 0, 100)
		var allIdentities []string
		for i := 0; i < 500; i++ {
			identity := fmt.Sprintf("identity%d", i)
			allIdentities = append(allIdentities, identity)
			profileSymKey, err := crypto.NewRandomAES()
			require.NoError(t, err)
			wantProfile := &model.IdentityProfile{
				Identity:   identity,
				Name:       "name1",
				GlobalName: globalName,
			}
			wantData := marshalProfile(t, wantProfile, profileSymKey)

			err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: wantData,
				},
			})
			require.NoError(t, err)
			fx.identityObservers[identity] = map[string]struct{}{"test": {}}
			fx.identityEncryptionKeys[identity] = profileSymKey
			fx.addParticipant(t, "test", identity)
			nsServiceResult = append(nsServiceResult, &nameserviceproto.NameByAddressResponse{
				Found: false,
				Name:  "",
			})
		}
		fx.nsClient.EXPECT().BatchGetNameByAnyId(gomock.Any(), gomock.Any()).Return(&nameserviceproto.BatchNameByAddressResponse{Results: nsServiceResult}, nil)

		// when
		fx.identityForceUpdate <- struct{}{}

		// then
		require.Eventually(t, func() bool {
			return allParticipantsHaveName(fx, "test", allIdentities, "name1")
		}, 5*time.Second, 10*time.Millisecond)
	})
	t.Run("partly receive identity from coordinator, but it failed at some point - use cache for such identities", func(t *testing.T) {
		// given
		testObserverPeriod := time.Duration(math.MaxInt) // make sure observing won't run by ticker
		fx := newFixture(t, testObserverPeriod)
		nsServiceResult := make([]*nameserviceproto.NameByAddressResponse, 0, 100)
		var allIdentities []string
		for i := 0; i < 500; i++ {
			identity := fmt.Sprintf("identity%d", i)
			allIdentities = append(allIdentities, identity)
			profileSymKey, err := crypto.NewRandomAES()
			require.NoError(t, err)
			wantProfile := &model.IdentityProfile{
				Identity:   identity,
				Name:       "name1",
				GlobalName: globalName,
			}
			wantData := marshalProfile(t, wantProfile, profileSymKey)

			err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: wantData,
				},
			})
			require.NoError(t, err)
			fx.identityObservers[identity] = map[string]struct{}{"test": {}}
			fx.identityEncryptionKeys[identity] = profileSymKey
			fx.addParticipant(t, "test", identity)
			nsServiceResult = append(nsServiceResult, &nameserviceproto.NameByAddressResponse{
				Found: false,
				Name:  "",
			})
			err = fx.service.identityProfileCacheStore.Set(context.Background(), identity, wantData)
			require.NoError(t, err)
			err = fx.service.identityGlobalNameCacheStore.Set(context.Background(), identity, globalName)
			require.NoError(t, err)
		}
		fx.nsClient.EXPECT().BatchGetNameByAnyId(gomock.Any(), gomock.Any()).Return(&nameserviceproto.BatchNameByAddressResponse{Results: nsServiceResult}, nil)

		// when
		var called bool // call identity repo once and then fail it to simulate failure between identities batching call
		fx.coordinatorClient.setCallback(func(identities []string, kinds []string, res []*identityrepoproto.DataWithIdentity, resErr error) {
			if called {
				fx.coordinatorClient.isUnavailable = true
			} else {
				called = true
			}
		})
		fx.identityForceUpdate <- struct{}{}

		// then
		require.Eventually(t, func() bool {
			return allParticipantsHaveName(fx, "test", allIdentities, "name1")
		}, 5*time.Second, 10*time.Millisecond)
	})
}

func allParticipantsHaveName(fx *fixture, spaceId string, identities []string, name string) bool {
	for _, identity := range identities {
		if !fx.participantHasName(spaceId, identity, name) {
			return false
		}
	}
	return true
}
