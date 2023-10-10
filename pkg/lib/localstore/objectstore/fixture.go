package objectstore

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type StoreFixture struct {
	*dsObjectStore
}

func NewStoreFixture(t *testing.T) *StoreFixture {
	typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	typeProvider.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil).Maybe()

	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName)
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	fullText := ftsearch.New()
	testApp := &app.App{}
	testApp.Register(walletService)
	err := fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	db, err := badger.Open(badger.DefaultOptions(filepath.Join(t.TempDir(), "badger")))
	require.NoError(t, err)

	ds := &dsObjectStore{
		sbtProvider: typeProvider,
		fts:         fullText,
		db:          db,
	}
	err = ds.initCache()
	require.NoError(t, err)
	return &StoreFixture{
		dsObjectStore: ds,
	}
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
		bundle.RelationKeySpaceId: pbtypes.String("space1"),
	}
}

func makeObjectWithNameAndDescription(id string, name string, description string) TestObject {
	return TestObject{
		bundle.RelationKeyId:          pbtypes.String(id),
		bundle.RelationKeyName:        pbtypes.String(name),
		bundle.RelationKeyDescription: pbtypes.String(description),
		bundle.RelationKeySpaceId:     pbtypes.String("space1"),
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
