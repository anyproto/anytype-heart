package invitestore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return("techSpaceId").Maybe()
	rpcStore := rpcstore.NewInMemoryStore(1024)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return("repo/path")

	a := new(app.App)
	a.Register(objectstore.NewStoreFixture(t))
	a.Register(dataStoreProvider)
	a.Register(filestore.New())
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(filestorage.NewInMemory())
	a.Register(rpcstore.NewInMemoryService(rpcStore))
	a.Register(fileservice.New())
	a.Register(filesync.New())
	a.Register(fileoffloader.New())
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

	err = a.Start(ctx)
	require.NoError(t, err)

	s := New()
	s.Init(a)

	return &fixture{
		Service: s,
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

	_, err = fx.GetInvite(ctx, id, key)
	require.Error(t, err)
}
