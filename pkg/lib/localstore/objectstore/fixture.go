package objectstore

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type StoreFixture struct {
	*dsObjectStore
}

// nolint: unused
const spaceName = "space1"

func NewStoreFixture(t *testing.T) *StoreFixture {
	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName).Maybe()
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}
	testApp.Register(walletService)
	err := fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	badgerDir := filepath.Join(t.TempDir(), "badger")
	db, err := badger.Open(badger.DefaultOptions(badgerDir))
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
		err = fullText.Close(context.Background())
		require.NoError(t, err)
	})

	ds := &dsObjectStore{
		fts:           fullText,
		sourceService: &detailsFromId{},
		db:            db,
		isClosingCh:   make(chan struct{}),
	}
	err = ds.initCache()
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

func (fx *StoreFixture) AddObjects(t *testing.T, objects []TestObject) {
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].GetStringValue()
		require.NotEmpty(t, id)
		err := fx.UpdateObjectDetails(id, makeDetails(obj))
		require.NoError(t, err)
	}
}
