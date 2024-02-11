package identity

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

type fixture struct {
	*service
	coordinatorClient *mock_coordinatorclient.MockCoordinatorClient
}

const testObserverPeriod = 5 * time.Millisecond

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	coordinatorClient := mock_coordinatorclient.NewMockCoordinatorClient(ctrl)
	objectStore := objectstore.NewStoreFixture(t)
	accountService := mock_account.NewMockService(t)
	spaceService := mock_space.NewMockService(t)
	fileAclService := mock_fileacl.NewMockService(t)
	dataStore := datastore.NewInMemory()
	err := dataStore.Run(ctx)
	require.NoError(t, err)

	a := new(app.App)
	a.Register(&spaceIdDeriverStub{})
	a.Register(dataStore)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, coordinatorClient))
	a.Register(testutil.PrepareMock(ctx, a, accountService))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(testutil.PrepareMock(ctx, a, fileAclService))

	svc := New(testObserverPeriod, 1*time.Microsecond)
	err = svc.Init(a)
	t.Cleanup(func() {
		svc.Close(ctx)
	})
	require.NoError(t, err)

	svcRef := svc.(*service)
	db, err := dataStore.LocalStorage()
	require.NoError(t, err)
	svcRef.db = db
	fx := &fixture{
		service:           svcRef,
		coordinatorClient: coordinatorClient,
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

func TestIdentityProfileCache(t *testing.T) {
	fx := newFixture(t)

	spaceId := "space1"
	identity := "identity1"

	fx.coordinatorClient.EXPECT().IdentityRepoGet(gomock.Any(), []string{identity}, []string{identityRepoDataKind}).Return(nil, fmt.Errorf("network problem")).AnyTimes()

	profileSymKey, err := crypto.NewRandomAES()
	require.NoError(t, err)
	wantProfile := &model.IdentityProfile{
		Identity: identity,
		Name:     "name1",
	}
	wantData := marshalProfile(t, wantProfile, profileSymKey)

	err = badgerhelper.SetValue(fx.db, makeIdentityProfileKey(identity), wantData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	err = fx.RegisterIdentity(spaceId, identity, profileSymKey, func(gotIdentity string, gotProfile *model.IdentityProfile) {
		assert.Equal(t, identity, gotIdentity)
		assert.Equal(t, wantProfile, gotProfile)
		wg.Done()
	})
	require.NoError(t, err)

	wg.Wait()
}

func TestObservers(t *testing.T) {
	fx := newFixture(t)

	spaceId := "space1"
	identity := "identity1"

	profileSymKey, err := crypto.NewRandomAES()
	require.NoError(t, err)
	wantProfile := &model.IdentityProfile{
		Identity: identity,
		Name:     "name1",
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

	var identitiesFromRepo []*identityrepoproto.DataWithIdentity

	fx.coordinatorClient.EXPECT().IdentityRepoGet(gomock.Any(), []string{identity}, []string{identityRepoDataKind}).DoAndReturn(func(_ context.Context, _ []string, _ []string) ([]*identityrepoproto.DataWithIdentity, error) {
		return identitiesFromRepo, nil
	}).AnyTimes()
	time.Sleep(testObserverPeriod)
	identitiesFromRepo = []*identityrepoproto.DataWithIdentity{
		{
			Identity: identity,
			Data: []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: wantData,
				},
			},
		},
	}

	t.Run("change profile's name", func(t *testing.T) {
		wantProfile2 := &model.IdentityProfile{
			Identity: identity,
			Name:     "name1 edited",
		}
		wantData2 := marshalProfile(t, wantProfile2, profileSymKey)

		time.Sleep(testObserverPeriod)
		identitiesFromRepo = []*identityrepoproto.DataWithIdentity{
			{
				Identity: identity,
				Data: []*identityrepoproto.Data{
					{
						Kind: identityRepoDataKind,
						Data: wantData2,
					},
				},
			},
		}
	})

	wg.Wait()

	wantCalls := []*model.IdentityProfile{
		{
			Identity: identity,
			Name:     "name1",
		},
		{
			Identity: identity,
			Name:     "name1 edited",
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

type spaceIdDeriverStub struct{}

func (s spaceIdDeriverStub) Init(a *app.App) (err error) { return nil }

func (s spaceIdDeriverStub) Name() (name string) { return "spaceIdDeriverStub" }

func (s spaceIdDeriverStub) DeriveID(ctx context.Context, spaceType string) (id string, err error) {
	// TODO implement me
	panic("implement me")
}
