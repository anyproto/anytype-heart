package indexer

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus/mock_spacesyncstatus"
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
)

type indexerFixture struct {
	*indexer
	pickerFx              *mock_cache.MockCachedObjectGetter
	storageServiceFx      *mock_storage.MockClientStorage
	objectStore           *objectstore.StoreFixture
	sourceFx              *mock_source.MockService
	techSpaceIdProviderFx *mock_spacesyncstatus.MockSpaceIdGetter
}

func NewIndexerFixture(t *testing.T) *indexerFixture {

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

	indexerFx := &indexerFixture{
		indexer:     indxr,
		objectStore: objectStore,
		sourceFx:    sourceService,
	}

	indxr.store = objectStore
	indexerFx.storageServiceFx = clientStorage
	indexerFx.techSpaceIdProviderFx = mock_spacesyncstatus.NewMockSpaceIdGetter(t)
	indxr.source = sourceService

	hasher := mock_indexer.NewMockHasher(t)
	hasher.EXPECT().Hash().Return("5d41402abc4b2a76b9719d911017c592").Maybe()
	indxr.btHash = hasher

	indxr.ftsearch = objectStore.FullText
	indexerFx.ftsearch = indxr.ftsearch
	indexerFx.pickerFx = mock_cache.NewMockCachedObjectGetter(t)
	indxr.picker = indexerFx.pickerFx
	indxr.spaceIndexers = make(map[string]*spaceIndexer)
	indxr.spaces = make(map[string]struct{})
	indxr.techSpaceIdProvider = indexerFx.techSpaceIdProviderFx
	indexerFx.techSpaceIdProviderFx.EXPECT().TechSpaceId().Return("").Maybe()
	indxr.forceFt = make(chan struct{})
	indxr.config = &config.Config{NetworkMode: pb.RpcAccount_LocalOnly}
	indxr.runCtx, indxr.runCtxCancel = context.WithCancel(ctx)

	indexerFx.pickerFx.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Maybe().Return(true, nil)
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

	// Set up the object details to return non-deleted status
	indexerFx.store.SpaceIndex("spaceId1").UpdateObjectDetails(context.Background(), "objectId1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyIsDeleted: domain.Bool(false),
	}))

	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)
	indexerFx.pickerFx.EXPECT().TryRemoveFromCache(mock.Anything, "objectId1").Return(true, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, "spaceId1", docs[0].SpaceId)
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.Len(t, docs, 0)
	assert.NoError(t, err)
}

func TestPrepareSearchDocument_RelationShortText_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("Title Text"),
	}))
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "objectId1/r/name", docs[0].Id)
	assert.Equal(t, "Title Text", docs[0].Text)
	assert.Equal(t, "", docs[0].Title)
}

func TestPrepareSearchDocument_System_Plural_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyPluralName: domain.String("Plural title Text"),
	}))
	smartTest.Doc.(*state.State).SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyResolvedLayout: domain.Int64(0),
	}))
	smartTest.Doc.Layout()
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "objectId1/r/pluralName", docs[0].Id)
	assert.Equal(t, "", docs[0].Text)
	assert.Equal(t, "Plural title Text", docs[0].Title)
}

func TestPrepareSearchDocument_RelationLongText_Success(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("Title Text"),
	}))
	smartTest.SetSpaceId("spaceId1")
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "objectId1/r/name", docs[0].Id)
	assert.Equal(t, "Title Text", docs[0].Text)
	assert.Equal(t, "", docs[0].Title)
}

func TestPrepareSearchDocument_RelationText_EmptyValue(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	// Empty value for relation key
	smartTest.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String(""),
	}))
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	require.NoError(t, err)
	require.Len(t, docs, 0)
}

func TestPrepareSearchDocument_RelationText_WrongFormat(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	smartTest := smarttest.New("objectId1")
	smartTest.SetSpaceId("spaceId1")
	// Relation with wrong format
	key := domain.RelationKey("customTag")
	smartTest.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		key: domain.String("Title Text"),
	}))
	indexerFx.objectStore.AddObjects(t, "spaceId1", []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(key.URL()),
		bundle.RelationKeyUniqueKey:      domain.String(key.URL()),
		bundle.RelationKeyRelationKey:    domain.String(key.String()),
		bundle.RelationKeyRelationFormat: domain.Int64(model.RelationFormat_tag),
	}})
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
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

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	assert.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "objectId1/b/blockId1", docs[0].Id)
	assert.Equal(t, maxSize, len(docs[0].Text))
}

func TestRunFullTextIndexer(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	for i := range 10 {
		objectId := "objectId" + strconv.Itoa(i)

		// Set up object details to mark as not deleted
		indexerFx.store.SpaceIndex("spaceId1").UpdateObjectDetails(context.Background(), objectId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(objectId),
			bundle.RelationKeyIsDeleted: domain.Bool(false),
		}))

		smartTest := smarttest.New(objectId)
		smartTest.SetSpaceId("spaceId1")
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Text content",
					blockbuilder.ID("blockId1"),
				),
			)))
		indexerFx.store.AddToIndexQueue(context.Background(), domain.FullID{ObjectID: objectId, SpaceID: "spaceId1"})
		indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, objectId).Return(smartTest, nil).Once()
	}

	indexerFx.OnSpaceLoad("spaceId1")
	// Verify that the space was loaded
	spaces := indexerFx.activeSpaces()
	assert.Contains(t, spaces, "spaceId1", "Space should be in active spaces")

	// Check queue before processing
	queuedIds, err := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
	require.NoError(t, err)
	assert.Len(t, queuedIds, 10, "Should have 10 items in queue")

	indexerFx.runFullTextIndexer(context.Background())

	// Check queue after processing
	queuedIds, err = indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
	require.NoError(t, err)
	assert.Len(t, queuedIds, 0, "Queue should be empty after processing")

	count, _ := indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))
	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))

	for i := range 10 {
		content := "Text content"
		if i <= 3 {
			content = "Text content new"
		}
		objectId := "objectId" + strconv.Itoa(i)

		// Object details are already set from the first run, no need to update

		smartTest := smarttest.New(objectId)
		smartTest.SetSpaceId("spaceId1")
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					content,
					blockbuilder.ID("blockId1"),
				),
			)))
		indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, objectId).Return(smartTest, nil).Once()
		indexerFx.store.AddToIndexQueue(context.Background(), domain.FullID{ObjectID: objectId, SpaceID: "spaceId1"})

	}

	indexerFx.runFullTextIndexer(context.Background())

	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, 10, int(count))

}

func TestRunFullTextIndexer_Minimal(t *testing.T) {
	indexerFx := NewIndexerFixture(t)

	// Set up a single object
	objectId := "testObject1"
	indexerFx.store.SpaceIndex("spaceId1").UpdateObjectDetails(context.Background(), objectId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String(objectId),
		bundle.RelationKeyIsDeleted: domain.Bool(false),
	}))

	smartTest := smarttest.New(objectId)
	smartTest.SetSpaceId("spaceId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"Hello World",
				blockbuilder.ID("blockId1"),
			),
		)))

	// Add to queue
	err := indexerFx.store.AddToIndexQueue(context.Background(), domain.FullID{ObjectID: objectId, SpaceID: "spaceId1"})
	require.NoError(t, err)

	// Set up mock expectations
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, objectId).Return(smartTest, nil).Once()
	// TryRemoveFromCache is already mocked with a wildcard in the fixture

	// Load space
	indexerFx.OnSpaceLoad("spaceId1")

	// Check queue before indexing
	queuedBefore, err := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
	require.NoError(t, err)
	assert.Len(t, queuedBefore, 1, "Should have 1 item in queue before indexing")

	// Run indexer
	err = indexerFx.runFullTextIndexer(context.Background())
	require.NoError(t, err)

	// Check queue after indexing
	queuedAfter, err := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
	require.NoError(t, err)
	assert.Len(t, queuedAfter, 0, "Queue should be empty after indexing")

	// Check if document was indexed
	count, err := indexerFx.ftsearch.DocCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count, "Should have indexed 1 document")

	// Also verify that we can iterate over the indexed document
	var foundDoc bool
	err = indexerFx.ftsearch.Iterate(objectId, []string{"Title", "Text"}, func(doc *ftsearch.SearchDoc) bool {
		foundDoc = true
		t.Logf("Found document: %+v", doc)
		return true
	})
	require.NoError(t, err)
	assert.True(t, foundDoc, "Should find the indexed document")
}

func TestFTSearchDirect(t *testing.T) {
	indexerFx := NewIndexerFixture(t)

	// Test direct indexing
	err := indexerFx.ftsearch.Index(ftsearch.SearchDoc{
		Id:      "testId",
		SpaceId: "spaceId1",
		Text:    "Hello World",
	})
	require.NoError(t, err)

	count, err := indexerFx.ftsearch.DocCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count, "Should have 1 document after direct index")
}

func TestPrepareSearchDocumentWithDetails(t *testing.T) {
	indexerFx := NewIndexerFixture(t)

	objectId := "testObject1"
	// Set up object details to mark as not deleted
	err := indexerFx.store.SpaceIndex("spaceId1").UpdateObjectDetails(context.Background(), objectId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String(objectId),
		bundle.RelationKeyIsDeleted: domain.Bool(false),
	}))
	require.NoError(t, err)

	smartTest := smarttest.New(objectId)
	smartTest.SetSpaceId("spaceId1")
	smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"Hello World",
				blockbuilder.ID("blockId1"),
			),
		)))

	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)
	indexerFx.pickerFx.EXPECT().TryRemoveFromCache(mock.Anything, objectId).Return(true, nil)

	docs, err := indexerFx.prepareSearchDocument(context.Background(), domain.FullID{ObjectID: objectId, SpaceID: "spaceId1"})
	require.NoError(t, err)
	require.Len(t, docs, 1, "Should prepare 1 document")
	assert.Equal(t, "testObject1/b/blockId1", docs[0].Id)
	assert.Equal(t, "spaceId1", docs[0].SpaceId)
	assert.Equal(t, "Hello World", docs[0].Text)
}

func TestAutoBatcherSimple(t *testing.T) {
	indexerFx := NewIndexerFixture(t)

	// First, verify that the index is working with direct indexing
	err := indexerFx.ftsearch.Index(ftsearch.SearchDoc{
		Id:      "direct1",
		SpaceId: "space1",
		Text:    "Direct Index",
	})
	require.NoError(t, err)

	count, err := indexerFx.ftsearch.DocCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count, "Direct indexing should work")

	// Now test the batcher
	batcher := indexerFx.ftsearch.NewAutoBatcher()

	// Add a document
	err = batcher.UpsertDoc(ftsearch.SearchDoc{
		Id:      "test1",
		SpaceId: "space1",
		Text:    "Hello World",
	})
	require.NoError(t, err)

	// Finish batch
	ftIndexSeq, err := batcher.Finish()
	require.NoError(t, err)
	t.Logf("Batcher returned ftIndexSeq: %d", ftIndexSeq)
	assert.NotEqual(t, uint64(0), ftIndexSeq, "ftIndexSeq should not be 0 after indexing a document")

	var foundDoc bool
	err = indexerFx.ftsearch.Iterate("test1", []string{"Text"}, func(doc *ftsearch.SearchDoc) bool {
		t.Logf("Found document in batch: %+v", doc)
		foundDoc = true
		assert.Equal(t, "Hello World", doc.Text, "Document text should match")
		return true
	})
	require.NoError(t, err)
	assert.True(t, foundDoc, "Should find the indexed document in the batch")
	// Check if document was indexed
	count, err = indexerFx.ftsearch.DocCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count, "Should have 2 documents after batch (1 direct + 1 batched)")
}

func TestAutoBatcherUpdate(t *testing.T) {
	indexerFx := NewIndexerFixture(t)

	// First, index a document with initial content
	err := indexerFx.ftsearch.Index(ftsearch.SearchDoc{
		Id:      "updateTest1",
		SpaceId: "space1",
		Text:    "Initial Content",
		Title:   "Initial Title",
	})
	require.NoError(t, err)

	// Verify initial content
	var foundInitial bool
	err = indexerFx.ftsearch.Iterate("updateTest1", []string{"Text", "Title"}, func(doc *ftsearch.SearchDoc) bool {
		foundInitial = true
		assert.Equal(t, "Initial Content", doc.Text)
		assert.Equal(t, "Initial Title", doc.Title)
		return true
	})
	require.NoError(t, err)
	assert.True(t, foundInitial, "Should find the initial document")

	// Now update the document using the batcher
	batcher := indexerFx.ftsearch.NewAutoBatcher()

	err = batcher.UpsertDoc(ftsearch.SearchDoc{
		Id:      "updateTest1",
		SpaceId: "space1",
		Text:    "Updated Content",
		Title:   "Updated Title",
	})
	require.NoError(t, err)

	// Finish batch
	ftIndexSeq, err := batcher.Finish()
	require.NoError(t, err)
	assert.NotEqual(t, uint64(0), ftIndexSeq, "ftIndexSeq should not be 0 after updating a document")

	// Verify updated content
	var foundUpdated bool
	err = indexerFx.ftsearch.Iterate("updateTest1", []string{"Text", "Title"}, func(doc *ftsearch.SearchDoc) bool {
		foundUpdated = true
		assert.Equal(t, "Updated Content", doc.Text, "Text should be updated")
		assert.Equal(t, "Updated Title", doc.Title, "Title should be updated")
		return true
	})
	require.NoError(t, err)
	assert.True(t, foundUpdated, "Should find the updated document")

	// Verify still only one document (not duplicated)
	count, err := indexerFx.ftsearch.DocCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count, "Should still have only 1 document after update")
}

func TestPrepareSearchDocument_Reindex_Removed(t *testing.T) {
	indexerFx := NewIndexerFixture(t)
	indexerFx.ftsearch.Index(ftsearch.SearchDoc{Id: "objectId1/r/blockId1", SpaceId: "spaceId1"})
	indexerFx.ftsearch.Index(ftsearch.SearchDoc{Id: "objectId1/r/blockId2", SpaceId: "spaceId1"})

	count, _ := indexerFx.ftsearch.DocCount()
	assert.Equal(t, uint64(2), count)

	// Set up object details to mark as not deleted
	indexerFx.store.SpaceIndex("spaceId1").UpdateObjectDetails(context.Background(), "objectId1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String("objectId1"),
		bundle.RelationKeyIsDeleted: domain.Bool(false),
	}))

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
	indexerFx.store.AddToIndexQueue(context.Background(), domain.FullID{ObjectID: "objectId1", SpaceID: "spaceId1"})
	indexerFx.pickerFx.EXPECT().GetObject(mock.Anything, mock.Anything).Return(smartTest, nil)
	indexerFx.OnSpaceLoad("spaceId1")
	indexerFx.runFullTextIndexer(context.Background())

	count, _ = indexerFx.ftsearch.DocCount()
	assert.Equal(t, uint64(1), count)
}
