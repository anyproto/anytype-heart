package indexer

import (
	"context"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/mock_block"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore/mock_filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type IndexerFixture struct {
	*indexer
}

func NewStoreFixture(t *testing.T) *IndexerFixture {

	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName)
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	objectStore := mock_objectstore.NewMockObjectStore(t)

	clientStorage := mock_storage.NewMockClientStorage(t)

	sourceService := mock_source.NewMockService(t)

	fileStore := mock_filestore.NewMockFileStore(t)

	testApp := &app.App{}
	testApp.Register(walletService)

	fullText := ftsearch.New()
	testApp.Register(fullText)

	err := fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	indxr := &indexer{
		indexedFiles: &sync.Map{},
	}

	indxr.newAccount = config.New().NewAccount
	indxr.store = objectStore
	indxr.storageService = clientStorage
	indxr.source = sourceService
	indxr.btHash = mock_indexer.NewMockHasher(t)
	indxr.fileStore = fileStore
	indxr.ftsearch = fullText
	indxr.picker = mock_block.NewMockObjectGetter(t)
	indxr.fileService = mock_files.NewMockService(t)
	indxr.quit = make(chan struct{})
	indxr.forceFt = make(chan struct{})

	require.NoError(t, err)
	return &IndexerFixture{
		indexer: indxr,
	}
}

func TestPrepareSearchDocument_Success(t *testing.T) {
	ixr := NewStoreFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"to index",
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)
	ixr.store.(*mock_objectstore.MockObjectStore).EXPECT().UpdateObjectSnippet(mock.Anything, mock.Anything).Return(nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		called = true
		return nil
	})

	assert.True(t, true, called)
	assert.NoError(t, err)
}
