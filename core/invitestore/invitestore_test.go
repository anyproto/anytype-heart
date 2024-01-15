package invitestore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
}
