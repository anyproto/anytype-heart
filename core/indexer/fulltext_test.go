package indexer

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
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
	pickerFx         *mock_cache.MockObjectGetter
	storageServiceFx *mock_storage.MockClientStorage
	objectStore      *objectstore.StoreFixture
	sourceFx         *mock_source.MockService
}

func NewIndexerFixture(t *testing.T) *IndexerFixture {

	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName)

	objectStore := objectstore.NewStoreFixture(t)
	clientStorage := mock_storage.NewMockClientStorage(t)

	sourceService := mock_source.NewMockService(t)

	ds, err := datastore.NewInMemory()
	require.NoError(t, err)

	testApp := &app.App{}
	testApp.Register(ds)
	testApp.Register(walletService)

	testApp.Register(objectStore.FullText)

	indxr := &indexer{}

	indexerFx := &IndexerFixture{
		indexer:     indxr,
		objectStore: objectStore,
		sourceFx:    sourceService,
	}

	indxr.store = objectStore
	indexerFx.storageService = clientStorage
	indexerFx.storageServiceFx = clientStorage
	indxr.source = sourceService

	hasher := mock_indexer.NewMockHasher(t)
	hasher.EXPECT().Hash().Return("5d41402abc4b2a76b9719d911017c592").Maybe()
	indxr.btHash = hasher

	indxr.ftsearch = objectStore.FullText
	indexerFx.ftsearch = indxr.ftsearch
	indexerFx.pickerFx = mock_cache.NewMockObjectGetter(t)
	indxr.picker = indexerFx.pickerFx
	indxr.spaceIndexers = make(map[string]*spaceIndexer)
	indxr.forceFt = make(chan struct{})
	indxr.config = &config.Config{NetworkMode: pb.RpcAccount_LocalOnly}
	indxr.runCtx, indxr.runCtxCancel = context.WithCancel(ctx)
	// go indxr.indexBatchLoop()
	return indexerFx
}

func TestPrepareSearchDocument_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, "spaceId1", docs[0].SpaceID)
}

func TestPrepareSearchDocument_Empty_NotIndexing(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 0)
}

func TestPrepareSearchDocument_NoIndexableType(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.Len(t, docs, 0)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_NoTextBlock(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	// Setting no text block
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
	))
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.Len(t, docs, 0)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationShortText_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "objectId1/r/name", docs[0].Id)
	assert.Equal(t, "Title Text", docs[0].Text)
	assert.Equal(t, "Title Text", docs[0].Title)
}

func TestPrepareSearchDocument_RelationLongText_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "objectId1/r/name", docs[0].Id)
	assert.Equal(t, "Title Text", docs[0].Text)
	assert.Equal(t, "Title Text", docs[0].Title)
}

func TestPrepareSearchDocument_RelationText_EmptyValue(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	require.NoError(t, err)
	require.Len(t, docs, 0)
}

func TestPrepareSearchDocument_RelationText_WrongFormat(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 0)
}

func TestPrepareSearchDocument_BlockText_LessThanMaxSize(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"Text content less than max size",
				blockbuilder.ID("blockId1"),
			),
		)))
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, "Text content less than max size", docs[0].Text)
}

func TestPrepareSearchDocument_BlockText_EqualToMaxSize(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, textContent, docs[0].Text)
}

func TestPrepareSearchDocument_BlockText_GreaterThanMaxSize(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
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
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), "objectId1")
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, maxSize, len(docs[0].Text))
}

func TestRunFullTextIndexer(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	for i := range 10 {
		smartTest := smarttest.New("objectId" + strconv.Itoa(i))
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Text content",
					blockbuilder.ID("blockId1"),
				),
			)))
		indexerFx.store.AddToIndexQueue(context.Background(), "objectId"+strconv.Itoa(i))
		indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, "objectId"+strconv.Itoa(i)).Return(smartTest, nil).Once()
	}

	indexerFx.runFullTextIndexer(context.Background())

	count, _ := indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))
	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))

	for i := range 10 {
		content := "Text content"
		if i <= 3 {
			content = "Text content new"
		}
		smartTest := smarttest.New("objectId" + strconv.Itoa(i))
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					content,
					blockbuilder.ID("blockId1"),
				),
			)))
		indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, "objectId"+strconv.Itoa(i)).Return(smartTest, nil).Once()
		indexerFx.store.AddToIndexQueue(context.Background(), "objectId"+strconv.Itoa(i))

	}

	indexerFx.runFullTextIndexer(context.Background())

	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))

}

func TestPrepareSearchDocument_Reindex_Removed(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	indexerFx.ftsearch.Index(ftsearch.SearchDoc{Id: "objectId1/r/blockId1", SpaceID: "spaceId1"})
	indexerFx.ftsearch.Index(ftsearch.SearchDoc{Id: "objectId1/r/blockId2", SpaceID: "spaceId1"})

	count, _ := indexerFx.ftsearch.DocCount()
	assert.Equal(t, uint64(2), count)

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
	indexerFx.store.AddToIndexQueue(context.Background(), "objectId1")
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)
	indexerFx.runFullTextIndexer(context.Background())

	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, uint64(1), count)
}
