package invitestore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
)

type fixture struct {
	Service
}

func newFixture(t *testing.T) *fixture {
	blockStorage := filestorage.NewInMemory()
	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()

	ctx := context.Background()
	a := new(app.App)
	a.Register(blockStorage)
	a.Register(rpcStoreService)
	a.Register(commonFileService)
	err := a.Start(ctx)
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

	wantInvite := "hi!"
	id, key, err := fx.StoreInvite(ctx, wantInvite)
	require.NoError(t, err)

	invite, err := fx.GetInvite(ctx, id, key)
	require.NoError(t, err)
	assert.Equal(t, wantInvite, invite)
}
