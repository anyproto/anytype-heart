package identity

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
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

	a := new(app.App)
	a.Register(&spaceIdDeriverStub{})
	a.Register(&detailsModifierStub{})
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, coordinatorClient))
	a.Register(testutil.PrepareMock(ctx, a, accountService))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))

	svc := New(testObserverPeriod)
	err := svc.Init(a)
	require.NoError(t, err)

	return &fixture{
		service:           svc.(*service),
		coordinatorClient: coordinatorClient,
	}
}

func TestObservers(t *testing.T) {
	fx := newFixture(t)
	go fx.observeIdentitiesLoop()

	spaceId := "space1"
	identity := "identity1"
	key, err := symmetric.NewRandom()
	require.NoError(t, err)

	wantProfile := &model.IdentityProfile{
		Identity: identity,
		Name:     "name1",
	}
	wantData, err := proto.Marshal(wantProfile)
	// TODO Encrypt
	require.NoError(t, err)

	var wg sync.WaitGroup
	var callbackCalls []*model.IdentityProfile
	wg.Add(2)
	err = fx.RegisterIdentity(spaceId, identity, key, func(gotIdentity string, gotProfile *model.IdentityProfile) {
		callbackCalls = append(callbackCalls, gotProfile)
		wg.Done()
	})
	require.NoError(t, err)

	fx.coordinatorClient.EXPECT().IdentityRepoGet(gomock.Any(), []string{identity}, []string{identityRepoDataKind}).Return([]*identityrepoproto.DataWithIdentity{}, nil)
	time.Sleep(testObserverPeriod)
	fx.coordinatorClient.EXPECT().IdentityRepoGet(gomock.Any(), []string{identity}, []string{identityRepoDataKind}).Return([]*identityrepoproto.DataWithIdentity{
		{
			Identity: identity,
			Data: []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: wantData,
				},
			},
		},
	}, nil)

	t.Run("change profile's name", func(t *testing.T) {
		wantProfile2 := &model.IdentityProfile{
			Identity: identity,
			Name:     "name1 edited",
		}
		wantData2, err := proto.Marshal(wantProfile2)
		// TODO Encrypt
		require.NoError(t, err)

		time.Sleep(testObserverPeriod)
		fx.coordinatorClient.EXPECT().IdentityRepoGet(gomock.Any(), []string{identity}, []string{identityRepoDataKind}).Return([]*identityrepoproto.DataWithIdentity{
			{
				Identity: identity,
				Data: []*identityrepoproto.Data{
					{
						Kind: identityRepoDataKind,
						Data: wantData2,
					},
				},
			},
		}, nil)
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
}

type spaceIdDeriverStub struct{}

func (s spaceIdDeriverStub) Init(a *app.App) (err error) { return nil }

func (s spaceIdDeriverStub) Name() (name string) { return "spaceIdDeriverStub" }

func (s spaceIdDeriverStub) DeriveID(ctx context.Context, spaceType string) (id string, err error) {
	//TODO implement me
	panic("implement me")
}

type detailsModifierStub struct{}

func (s detailsModifierStub) Init(a *app.App) (err error) { return nil }

func (s detailsModifierStub) Name() (name string) { return "detailsModifierStub" }

func (detailsModifierStub) ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	//TODO implement me
	panic("implement me")
}
