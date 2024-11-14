package fileobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type indexerFixture struct {
	*indexer
	fileService        *mock_files.MockService
	objectStoreFixture *objectstore.StoreFixture
}

func newIndexerFixture(t *testing.T) *indexerFixture {
	objectStore := objectstore.NewStoreFixture(t)
	fileService := mock_files.NewMockService(t)

	svc := &service{
		objectStore: objectStore,
		fileService: fileService,
	}
	ind := svc.newIndexer()

	return &indexerFixture{
		objectStoreFixture: objectStore,
		fileService:        fileService,
		indexer:            ind,
	}
}

func TestIndexer_buildDetails(t *testing.T) {
	t.Run("with file", func(t *testing.T) {
		for _, typeKey := range []domain.TypeKey{
			bundle.TypeKeyFile,
			bundle.TypeKeyAudio,
			bundle.TypeKeyVideo,
		} {
			t.Run(fmt.Sprintf("with type %s", typeKey), func(t *testing.T) {
				fx := newIndexerFixture(t)
				id := domain.FullFileId{
					SpaceId: "space1",
					FileId:  testFileId,
				}
				ctx := context.Background()

				// TODO Infos
				details, gotTypeKey, err := fx.buildDetails(ctx, id, nil)
				require.NoError(t, err)
				assert.Equal(t, typeKey, gotTypeKey)
				assert.Equal(t, "name", pbtypes.GetString(details, bundle.RelationKeyName.String()))
				assert.Equal(t, pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)), details.Fields[bundle.RelationKeyFileIndexingStatus.String()])
			})
		}
	})
	t.Run("with image", func(t *testing.T) {
		fx := newIndexerFixture(t)
		id := domain.FullFileId{
			SpaceId: "space1",
			FileId:  testFileId,
		}
		ctx := context.Background()

		// TODO Infos
		details, gotTypeKey, err := fx.buildDetails(ctx, id, nil)
		require.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyImage, gotTypeKey)
		assert.Equal(t, "name", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)), details.Fields[bundle.RelationKeyFileIndexingStatus.String()])
	})
	t.Run("with image fell back to file", func(t *testing.T) {
		for _, typeKey := range []domain.TypeKey{
			bundle.TypeKeyFile,
			bundle.TypeKeyAudio,
			bundle.TypeKeyVideo,
			bundle.TypeKeyImage,
		} {
			t.Run(fmt.Sprintf("with type %s", typeKey), func(t *testing.T) {
				fx := newIndexerFixture(t)
				id := domain.FullFileId{
					SpaceId: "space1",
					FileId:  testFileId,
				}
				ctx := context.Background()

				// TODO Infos
				details, gotTypeKey, err := fx.buildDetails(ctx, id, nil)
				require.NoError(t, err)
				assert.Equal(t, bundle.TypeKeyImage, gotTypeKey)
				assert.Equal(t, "name", pbtypes.GetString(details, bundle.RelationKeyName.String()))
				assert.Equal(t, pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)), details.Fields[bundle.RelationKeyFileIndexingStatus.String()])
			})
		}
	})
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
		fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 pbtypes.String("id1"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space1"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_NotIndexed)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_file)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 pbtypes.String("id2"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space2"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_image)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space3", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 pbtypes.String("id3"),
				bundle.RelationKeyFileId:             pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId:            pbtypes.String("space3"),
				bundle.RelationKeyFileIndexingStatus: pbtypes.Int64(int64(model.FileIndexingStatus_NotFound)),
				bundle.RelationKeyLayout:             pbtypes.Int64(int64(model.ObjectType_video)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space4", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id4"),
				bundle.RelationKeyFileId:  pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId: pbtypes.String("space4"),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_audio)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space5", []objectstore.TestObject{
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

		fx.objectStoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
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
