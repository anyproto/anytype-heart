package indexer

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/mock_block"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type IndexerFixture struct {
	*indexer
}

func NewIndexerFixture(t *testing.T) *IndexerFixture {

	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName)
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	objectStore := objectstore.NewStoreFixture(t)
	clientStorage := mock_storage.NewMockClientStorage(t)

	sourceService := mock_source.NewMockService(t)

	fileStr := filestore.New()

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
	indxr.fileStore = fileStr
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
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.SetSpaceId("spaceId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"to index",
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		assert.Equal(t, "spaceId1", doc.SpaceID)
		called = true
		return nil
	})

	assert.True(t, true, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_Empty_NotIndexing(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.SetSpaceId("spaceId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"",
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		assert.Equal(t, "spaceId1", doc.SpaceID)
		called = true
		return nil
	})

	assert.False(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_NoIndexableType(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")

	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"to index",
				blockbuilder.ID("blockId1"),
			),
		)))
	smartTest.SetType(coresb.SmartBlockTypeDate)
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		called = true
		return nil
	})

	assert.False(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_NoTextBlock(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	// Setting no text block
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
	))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		called = true
		return nil
	})

	assert.False(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationShortText_Success(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_shorttext,
	})
	smartTest.Doc.(*state.State).SetDetails(&types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String("Title Text"),
		},
	})
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/r/name", doc.Id)
		assert.Equal(t, "Title Text", doc.Text)
		assert.Equal(t, "Title Text", doc.Title)
		called = true
		return nil
	})

	assert.True(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationLongText_Success(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_longtext,
	})
	smartTest.Doc.(*state.State).SetDetails(&types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String("Title Text"),
		},
	})
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/r/name", doc.Id)
		assert.Equal(t, "Title Text", doc.Text)
		assert.Equal(t, "Title Text", doc.Title)
		called = true
		return nil
	})

	assert.True(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationText_EmptyValue(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_shorttext,
	})
	// Empty value for relation key
	smartTest.Doc.(*state.State).SetDetails(&types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String(""),
		},
	})
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		called = true
		return nil
	})

	assert.False(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationText_WrongFormat(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	// Relation with wrong format
	smartTest.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyName.String(),
		Format: model.RelationFormat_email, // Wrong format
	})
	smartTest.Doc.(*state.State).SetDetails(&types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String("Title Text"),
		},
	})
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		called = true
		return nil
	})

	assert.False(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_BlockText_LessThanMaxSize(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"Text content less than max size",
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		assert.Equal(t, "Text content less than max size", doc.Text)
		called = true
		return nil
	})

	assert.True(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_BlockText_EqualToMaxSize(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	maxSize := ftBlockMaxSize
	textContent := strings.Repeat("a", maxSize) // Text content equal to max size
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				textContent,
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		assert.Equal(t, textContent, doc.Text)
		called = true
		return nil
	})

	assert.True(t, called)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_BlockText_GreaterThanMaxSize(t *testing.T) {
	ixr := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	maxSize := ftBlockMaxSize
	textContent := strings.Repeat("a", maxSize+1) // Text content greater than max size
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				textContent,
				blockbuilder.ID("blockId1"),
			),
		)))
	ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	called := false
	err := ixr.prepareSearchDocument("objectId1", func(doc ftsearch.SearchDoc) error {
		assert.Equal(t, "objectId1/b/blockId1", doc.Id)
		assert.Equal(t, maxSize, len(doc.Text))
		called = true
		return nil
	})

	assert.True(t, called)
	assert.NoError(t, err)
}

func TestRunFullTextIndexer(t *testing.T) {
	ixr := NewIndexerFixture(t)
	for i := range 101 {
		smartTest := smarttest.New("objectId" + strconv.Itoa(i))
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Text content",
					blockbuilder.ID("blockId1"),
				),
			)))
		ixr.store.AddToIndexQueue("objectId" + strconv.Itoa(i))
		ixr.picker.(*mock_block.MockObjectGetter).EXPECT().GetObject(mock.Anything, "objectId"+strconv.Itoa(i)).Return(smartTest, nil)
	}

	ixr.runFullTextIndexer()

	count, _ := ixr.ftsearch.DocCount()
	assert.Equal(t, uint64(101), count)
}
