package fileobject

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
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

	// For trying to read metadata from files
	buf := newNopCloserWrapper(bytes.NewReader(nil))
	fileService.EXPECT().GetContentReader(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(buf, nil).Maybe()

	svc := &service{
		objectStore:    objectStore,
		fileService:    fileService,
		accountService: &dummyAccountService{},
	}
	ind := svc.newIndexer()

	return &indexerFixture{
		objectStoreFixture: objectStore,
		fileService:        fileService,
		indexer:            ind,
	}
}

type nopCloserWrapper struct {
	io.Closer
	io.ReadSeeker
}

func newNopCloserWrapper(r io.ReadSeeker) *nopCloserWrapper {
	return &nopCloserWrapper{
		ReadSeeker: r,
		Closer:     io.NopCloser(r),
	}
}

func TestIndexer_buildDetails(t *testing.T) {
	t.Run("with file", func(t *testing.T) {
		for _, tc := range []struct {
			mimeType string
			wantType domain.TypeKey
		}{
			{mimeType: "application/pdf", wantType: bundle.TypeKeyFile},
			{mimeType: "audio/mpeg", wantType: bundle.TypeKeyAudio},
			{mimeType: "video/mp4", wantType: bundle.TypeKeyVideo},
		} {
			t.Run(fmt.Sprintf("with mime type %s", tc.mimeType), func(t *testing.T) {
				fx := newIndexerFixture(t)

				id := domain.FullFileId{
					SpaceId: "space1",
					FileId:  testFileId,
				}
				ctx := context.Background()

				details, gotTypeKey, err := fx.buildDetails(ctx, id, givenFileInfos(tc.mimeType))
				require.NoError(t, err)
				assert.Equal(t, tc.wantType, gotTypeKey)
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

		details, gotTypeKey, err := fx.buildDetails(ctx, id, givenImageInfos())
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

				details, gotTypeKey, err := fx.buildDetails(ctx, id, givenFileInfos("image/jpeg"))
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
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_file)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("id2"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space2"),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_Indexed)),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_image)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space3", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("id3"),
				bundle.RelationKeyFileId:             domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:            domain.String("space3"),
				bundle.RelationKeyFileIndexingStatus: domain.Int64(int64(model.FileIndexingStatus_NotFound)),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_video)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space4", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id4"),
				bundle.RelationKeyFileId:         domain.String(testFileId.String()),
				bundle.RelationKeySpaceId:        domain.String("space4"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_audio)),
			},
		})
		fx.objectStoreFixture.AddObjects(t, "space5", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id5"),
				bundle.RelationKeySpaceId:        domain.String("space5"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
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
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_audio)),
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

func givenImageInfos() []*storage.FileInfo {
	return []*storage.FileInfo{
		{
			Name:  "name",
			Media: "image/jpeg",
			Mill:  mill.ImageExifId,
		},
		{
			Name:  "name",
			Media: "image/jpeg",
			Mill:  mill.ImageResizeId,
			Meta: &types.Struct{
				Fields: map[string]*types.Value{
					"width":  pbtypes.Int64(640),
					"height": pbtypes.Int64(480),
				},
			},
		},
	}
}

func givenFileInfos(mimeType string) []*storage.FileInfo {
	return []*storage.FileInfo{
		{
			Name:  "name",
			Media: mimeType,
			Mill:  mill.BlobId,
		},
	}
}
