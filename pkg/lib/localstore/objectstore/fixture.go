package objectstore

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type StoreFixture struct {
	*dsObjectStore
}

// nolint: unused
const spaceName = "space1"

var ctx = context.Background()

func NewStoreFixture(t testing.TB) *StoreFixture {
	ctx, cancel := context.WithCancel(context.Background())

	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName).Maybe()
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}

	dataStore, err := datastore.NewInMemory()
	require.NoError(t, err)

	testApp.Register(dataStore)
	testApp.Register(walletService)
	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	oldStore := oldstore.New()
	err = oldStore.Init(testApp)
	require.NoError(t, err)

	ds := &dsObjectStore{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		fts:                fullText,
		sourceService:      &detailsFromId{},
		arenaPool:          &fastjson.ArenaPool{},
		repoPath:           walletService.RepoPath(),
		oldStore:           oldStore,
		collatorBufferPool: newCollatorBufferPool(),
	}

	t.Cleanup(func() {
		_ = fullText.Close(context.Background())
		_ = ds.Close(context.Background())
	})

	err = ds.Run(ctx)
	require.NoError(t, err)

	return &StoreFixture{
		dsObjectStore: ds,
	}
}

type detailsFromId struct {
}

func (d *detailsFromId) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	return nil, fmt.Errorf("not found")
}

func (fx *StoreFixture) Init(a *app.App) (err error) {
	return nil
}

type TestObject map[domain.RelationKey]*types.Value

func generateObjectWithRandomID() TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return TestObject{
		bundle.RelationKeyId:   pbtypes.String(id),
		bundle.RelationKeyName: pbtypes.String("name" + id),
	}
}

func makeObjectWithName(id string, name string) TestObject {
	return TestObject{
		bundle.RelationKeyId:      pbtypes.String(id),
		bundle.RelationKeyName:    pbtypes.String(name),
		bundle.RelationKeySpaceId: pbtypes.String(spaceName),
	}
}

func makeObjectWithNameAndDescription(id string, name string, description string) TestObject {
	return TestObject{
		bundle.RelationKeyId:          pbtypes.String(id),
		bundle.RelationKeyName:        pbtypes.String(name),
		bundle.RelationKeyDescription: pbtypes.String(description),
		bundle.RelationKeySpaceId:     pbtypes.String(spaceName),
	}
}

func makeDetails(fields TestObject) *types.Struct {
	f := map[string]*types.Value{}
	for k, v := range fields {
		f[string(k)] = v
	}
	return &types.Struct{Fields: f}
}

func (fx *StoreFixture) AddObjects(t testing.TB, objects []TestObject) {
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].GetStringValue()
		require.NotEmpty(t, id)
		err := fx.UpdateObjectDetails(context.Background(), "TODOSPACE", id, makeDetails(obj))
		require.NoError(t, err)
	}
}
