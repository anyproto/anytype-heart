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
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
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

				file := mock_files.NewMockFile(t)
				file.EXPECT().Info().Return(&storage.FileInfo{
					Mill:  mill.BlobId,
					Media: "text",
				})
				file.EXPECT().Details(ctx).Return(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("name"),
				},
				), typeKey, nil)
				fx.fileService.EXPECT().FileByHash(ctx, id).Return(file, nil)

				details, gotTypeKey, err := fx.buildDetails(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, typeKey, gotTypeKey)
				assert.Equal(t, "name", details.GetString(bundle.RelationKeyName))
				assert.Equal(t, int64(model.FileIndexingStatus_Indexed), details.GetInt64(bundle.RelationKeyFileIndexingStatus))
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

		file := mock_files.NewMockFile(t)
		file.EXPECT().Info().Return(&storage.FileInfo{
			Mill:  mill.ImageResizeId,
			Media: "image/jpeg",
		})

		image := mock_files.NewMockImage(t)
		image.EXPECT().Details(ctx).Return(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("name"),
		},
		), nil)
		fx.fileService.EXPECT().FileByHash(ctx, id).Return(file, nil)
		fx.fileService.EXPECT().ImageByHash(ctx, id).Return(image, nil)

		details, gotTypeKey, err := fx.buildDetails(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyImage, gotTypeKey)
		assert.Equal(t, "name", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.FileIndexingStatus_Indexed), details.GetInt64(bundle.RelationKeyFileIndexingStatus))
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

				file := mock_files.NewMockFile(t)
				file.EXPECT().Info().Return(&storage.FileInfo{
					Mill:  mill.BlobId,
					Media: "image/jpeg",
				})
				file.EXPECT().Details(ctx).Return(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("name"),
				},
				), typeKey, nil)
				fx.fileService.EXPECT().FileByHash(ctx, id).Return(file, nil)

				details, gotTypeKey, err := fx.buildDetails(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, bundle.TypeKeyImage, gotTypeKey)
				assert.Equal(t, "name", details.GetString(bundle.RelationKeyName))
				assert.Equal(t, int64(model.FileIndexingStatus_Indexed), details.GetInt64(bundle.RelationKeyFileIndexingStatus))
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
				bundle.RelationKeyId:                 domain.String("id1"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space1"),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_NotIndexed)),
				bundle.RelationKeyLayout:             domain.Int64(int64(model.ObjectType_file)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("id2"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space2"),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_Indexed)),
				bundle.RelationKeyLayout:             domain.Int64(int64(model.ObjectType_image)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space3", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("id3"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space3"),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_NotFound)),
				bundle.RelationKeyLayout:             domain.Int64(int64(model.ObjectType_video)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space4", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id4"),
				bundle.RelationKeyFileId:  domain.String(testFileId.String()),
				bundle.RelationKeySpaceId: domain.String("space4"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_audio)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space5", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id5"),
				bundle.RelationKeySpaceId: domain.String("space5"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_basic)),
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
				bundle.RelationKeyId:                 domain.String("id1"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space1"),
				bundle.RelationKeyLayout:             domain.Int64(int64(model.ObjectType_audio)),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_NotIndexed)),
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
