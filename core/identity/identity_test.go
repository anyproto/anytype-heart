package identity

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	mock_nameserviceclient "github.com/anyproto/any-sync/nameservice/nameserviceclient/mock"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/mutex"
)

type fixture struct {
	*service
	coordinatorClient *inMemoryIdentityRepo
	spaceService      *mock_space.MockService
	accountService    *mock_account.MockService
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
	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)
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
	err = dataStoreProvider.Run(ctx)
	require.NoError(t, err)

	a := new(app.App)
	a.Register(dataStoreProvider)
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
	db, err := dataStoreProvider.LocalStorage()
	require.NoError(t, err)
	svcRef.db = db
	// TODO
	// svcRef.currentProfileDetails = &types.Struct{Fields: make(map[string]*types.Value)}
	fx := &fixture{
		service:           svcRef,
		spaceService:      spaceService,
		accountService:    accountService,
		coordinatorClient: identityRepoClient,
	}
	go fx.observeIdentitiesLoop()

	return fx
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

		err = badgerhelper.SetValue(fx.db, makeIdentityProfileKey(identity), wantData)
		require.NoError(t, err)
		err = badgerhelper.SetValue(fx.db, makeGlobalNameKey(identity), globalName)
		require.NoError(t, err)

		var (
			gotIdentity string
			gotProfile  *model.IdentityProfile
		)
		err = fx.RegisterIdentity(spaceId, identity, profileSymKey, func(callbackIdentity string, profile *model.IdentityProfile) {
			gotIdentity = callbackIdentity
			gotProfile = profile
		})
		require.NoError(t, err)

		assert.Equal(t, identity, gotIdentity)
		assert.Equal(t, wantProfile, gotProfile)
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

		err = badgerhelper.SetValue(fx.db, makeGlobalNameKey(identity), globalName)
		require.NoError(t, err)

		var called uint64
		err = fx.RegisterIdentity(spaceId, identity, profileSymKey, func(callbackIdentity string, profile *model.IdentityProfile) {
			atomic.AddUint64(&called, 1)
			assert.Equal(t, identity, callbackIdentity)
			assert.Equal(t, wantProfile, profile)
		})
		require.NoError(t, err)

		err = badgerhelper.SetValue(fx.db, makeIdentityProfileKey(identity), wantData)
		require.NoError(t, err)

		time.Sleep(testObserverPeriod * 2)
		assert.Equal(t, uint64(1), atomic.LoadUint64(&called))
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

	var wg sync.WaitGroup
	var callbackCalls []*model.IdentityProfile
	wg.Add(2)
	err = fx.RegisterIdentity(spaceId, identity, profileSymKey, func(gotIdentity string, gotProfile *model.IdentityProfile) {
		callbackCalls = append(callbackCalls, gotProfile)
		wg.Done()
	})
	require.NoError(t, err)

	time.Sleep(testObserverPeriod * 2)

	err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
		{
			Kind: identityRepoDataKind,
			Data: wantData,
		},
	})
	require.NoError(t, err)

	t.Run("change profile's name", func(t *testing.T) {
		wantProfile2 := &model.IdentityProfile{
			Identity:    identity,
			Name:        "name1 edited",
			Description: "my description",
			GlobalName:  globalName,
		}
		wantData2 := marshalProfile(t, wantProfile2, profileSymKey)

		time.Sleep(testObserverPeriod * 2)
		err = fx.identityRepoClient.IdentityRepoPut(context.Background(), identity, []*identityrepoproto.Data{
			{
				Kind: identityRepoDataKind,
				Data: wantData2,
			},
		})
		require.NoError(t, err)
	})

	wg.Wait()

	wantCalls := []*model.IdentityProfile{
		{
			Identity:   identity,
			Name:       "name1",
			GlobalName: globalName,
		},
		{
			Identity:    identity,
			Name:        "name1 edited",
			Description: "my description",
			GlobalName:  globalName,
		},
	}
	assert.Equal(t, wantCalls, callbackCalls)

	t.Run("callback should be called at least once for each observer", func(t *testing.T) {
		wg.Add(1)
		err = fx.RegisterIdentity("space2", identity, profileSymKey, func(gotIdentity string, gotProfile *model.IdentityProfile) {
			wg.Done()
		})
		require.NoError(t, err)
		wg.Wait()
	})

}

func TestGetIdentitiesDataFromRepo(t *testing.T) {
	t.Run("empty identities", func(t *testing.T) {
		fx := newFixture(t, time.Millisecond)

		called := mutex.NewValue(false)
		fx.coordinatorClient.setCallback(func(_ []string, _ []string, _ []*identityrepoproto.DataWithIdentity, _ error) {
			called.Set(true)
		})
		list, err := fx.getIdentitiesDataFromRepo(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, list)
		require.False(t, called.Get())
	})
}
