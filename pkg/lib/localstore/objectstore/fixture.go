package objectstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceobjects"
)

type StoreFixture struct {
	*dsObjectStore
	FullText ftsearch.FTSearch
}

type detailsFromId struct {
}

func (d *detailsFromId) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	return nil, fmt.Errorf("not found")
}

type stubTechSpaceIdProvider struct{}

func (s *stubTechSpaceIdProvider) TechSpaceId() string {
	return "test-tech-space"
}

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
		componentCtx:        ctx,
		componentCtxCancel:  cancel,
		fts:                 fullText,
		sourceService:       &detailsFromId{},
		arenaPool:           &fastjson.ArenaPool{},
		repoPath:            walletService.RepoPath(),
		oldStore:            oldStore,
		stores:              map[string]spaceobjects.Store{},
		techSpaceIdProvider: &stubTechSpaceIdProvider{},
		subManager:          &spaceobjects.SubscriptionManager{},
	}

	t.Cleanup(func() {
		_ = fullText.Close(context.Background())
		_ = ds.Close(context.Background())
	})

	err = ds.Run(ctx)
	require.NoError(t, err)

	return &StoreFixture{
		dsObjectStore: ds,
		FullText:      fullText,
	}
}

func (fx *StoreFixture) Init(a *app.App) (err error) {
	return nil
}

type TestObject = spaceobjects.TestObject

func (fx *StoreFixture) AddObjects(t testing.TB, spaceId string, objects []spaceobjects.TestObject) {
	store := fx.SpaceId(spaceId)
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].GetStringValue()
		require.NotEmpty(t, id)
		err := store.UpdateObjectDetails(context.Background(), id, makeDetails(obj))
		require.NoError(t, err)
	}
}

func makeDetails(fields spaceobjects.TestObject) *types.Struct {
	f := map[string]*types.Value{}
	for k, v := range fields {
		f[string(k)] = v
	}
	return &types.Struct{Fields: f}
}
