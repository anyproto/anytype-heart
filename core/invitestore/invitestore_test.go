package invitestore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/util/crypto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service
	coordinator *mock_coordinatorclient.MockCoordinatorClient
	fileStore   filestorage.FileStorage
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return("techSpaceId").Maybe()
	rpcStore := rpcstore.NewInMemoryStore(1024)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())
	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	fileStore := filestorage.NewInMemory()

	a := new(app.App)
	a.Register(anystoreprovider.New())
	a.Register(objectstore.NewStoreFixture(t))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(fileStore)
	a.Register(rpcstore.NewInMemoryService(rpcStore))
	a.Register(fileservice.New())
	a.Register(filesync.New())
	a.Register(fileoffloader.New())
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})
	a.Register(testutil.PrepareMock(ctx, a, spaceIdResolver))
	mockCoord := mock_coordinatorclient.NewMockCoordinatorClient(ctrl)
	a.Register(testutil.PrepareMock(ctx, a, mockCoord))

	err := a.Start(ctx)
	require.NoError(t, err)

	s := New()
	s.Init(a)

	return &fixture{
		Service:     s,
		coordinator: mockCoord,
		fileStore:   fileStore,
	}
}

func TestStore(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	priv, pub, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	payload := []byte("payload")
	signature, err := priv.Sign(payload)
	require.NoError(t, err)

	wantInvite := &model.Invite{
		Payload:   payload,
		Signature: signature,
	}

	fx.coordinator.EXPECT().AclUploadInvite(ctx, gomock.Any()).Do(func(ctx context.Context, b blocks.Block) {
		_ = fx.fileStore.Add(ctx, []blocks.Block{b})
	})
	id, key, err := fx.StoreInvite(ctx, wantInvite)
	require.NoError(t, err)

	gotInvite, err := fx.GetInvite(ctx, id, key)
	require.NoError(t, err)
	assert.Equal(t, wantInvite, gotInvite)

	ok, err := pub.Verify(gotInvite.Payload, gotInvite.Signature)
	require.NoError(t, err)
	assert.True(t, ok)

	err = fx.RemoveInvite(ctx, id)
	require.NoError(t, err)
}
