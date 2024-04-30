package fileobject

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type indexerFixture struct {
	*indexer
	objectStoreFixture *objectstore.StoreFixture
}

func newIndexerFixture(t *testing.T) *indexerFixture {
	objectStore := objectstore.NewStoreFixture(t)

	svc := &service{
		objectStore: objectStore,
	}
	ind := svc.newIndexer()

	return &indexerFixture{
		objectStoreFixture: objectStore,
		indexer:            ind,
	}
}

func TestIndexer_addFromObjectStore(t *testing.T) {
	t.Run("no records in store", func(t *testing.T) {
		fx := newIndexerFixture(t)
		ctx := context.Background()

		err := fx.addToQueueFromObjectStore(ctx)
		require.NoError(t, err)

		got := fx.indexQueue.GetAll()
		assert.Empty(t, got)
	})

	t.Run("get records only with not indexed status", func(t *testing.T) {
		fx := newIndexerFixture(t)
		ctx := context.Background()

		//  Use same testFileId everywhere to pass domain.IsFileId check. It doesn't matter that files are same here
		fx.objectStoreFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 pbtypes.String("id1"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space1"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_NotIndexed)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_file)),
			},
			{
				bundle.RelationKeyId:                 pbtypes.String("id2"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space2"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_image)),
			},
			{
				bundle.RelationKeyId:                 pbtypes.String("id3"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space3"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_NotFound)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_video)),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("id4"),
				bundle.RelationKeyFileId:  pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId: pbtypes.String("space4"),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_audio)),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("id5"),
				bundle.RelationKeySpaceId: pbtypes.String("space5"),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_basic)),
			},
		})

		err := fx.addToQueueFromObjectStore(ctx)
		require.NoError(t, err)

		got := fx.indexQueue.GetAll()

		want := []indexRequest{
			{id: domain.FullID{SpaceID: "space1", ObjectID: "id1"}, fileId: domain.FullFileId{SpaceId: "space1", FileId: testFileId}},
			{id: domain.FullID{SpaceID: "space3", ObjectID: "id3"}, fileId: domain.FullFileId{SpaceId: "space3", FileId: testFileId}},
			{id: domain.FullID{SpaceID: "space4", ObjectID: "id4"}, fileId: domain.FullFileId{SpaceId: "space4", FileId: testFileId}},
		}

		assert.ElementsMatch(t, want, got)
	})

	t.Run("don't add same records twice", func(t *testing.T) {
		fx := newIndexerFixture(t)
		ctx := context.Background()

		fx.objectStoreFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 pbtypes.String("id1"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space1"),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_audio)),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_NotIndexed)),
			},
		})

		err := fx.addToQueueFromObjectStore(ctx)
		require.NoError(t, err)
		err = fx.addToQueueFromObjectStore(ctx)
		require.NoError(t, err)

		got := fx.indexQueue.GetAll()

		want := []indexRequest{
			{id: domain.FullID{SpaceID: "space1", ObjectID: "id1"}, fileId: domain.FullFileId{SpaceId: "space1", FileId: testFileId}},
		}

		assert.ElementsMatch(t, want, got)
	})
}
