package syncer

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/stretchr/testify/assert"

	block2 "github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer/mock_syncer"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestFileRelationSyncer_Sync(t *testing.T) {
	spaceId := "spaceId"
	fileId := "fileId"
	t.Run("relation file is missing", func(t *testing.T) {
		// given
		syncer := NewFileRelationSyncer(nil, nil)

		// when
		newFileId := syncer.Sync(spaceId, addr.MissingObject, nil, objectorigin.Import(model.Import_Pb))

		// then
		assert.Equal(t, addr.MissingObject, newFileId)
	})
	t.Run("relation fil is existing file object", func(t *testing.T) {
		// given
		syncer := NewFileRelationSyncer(nil, nil)
		newFileIds := map[string]struct{}{fileId: {}}

		// when
		newFileId := syncer.Sync(spaceId, fileId, newFileIds, objectorigin.Import(model.Import_Pb))

		// then
		assert.Equal(t, fileId, newFileId)
	})
	t.Run("relation file is not presented in import archive", func(t *testing.T) {
		// given
		rawCid, err := cidutil.NewCidFromBytes([]byte("test"))
		assert.Nil(t, err)

		service := mock_fileobject.NewMockService(t)
		fullFileId := domain.FullFileId{FileId: domain.FileId(rawCid), SpaceId: spaceId}
		service.EXPECT().CreateFromImport(fullFileId, objectorigin.Import(model.Import_Pb)).Return("newFileObjectId", nil)
		syncer := NewFileRelationSyncer(nil, service)
		newFileIds := map[string]struct{}{}

		// when
		newFileId := syncer.Sync(spaceId, rawCid, newFileIds, objectorigin.Import(model.Import_Pb))

		// then
		assert.Equal(t, "newFileObjectId", newFileId)
	})
	t.Run("relation is url", func(t *testing.T) {
		// given
		fileUploader := mock_syncer.NewMockBlockService(t)
		fileUploader.EXPECT().UploadFile(context.Background(), spaceId, block2.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				Url: "http://url.com",
			},
			ObjectOrigin: objectorigin.Import(model.Import_Pb),
		}).Return("newFileObjectId", nil, nil)
		syncer := NewFileRelationSyncer(fileUploader, nil)

		// when
		newFileId := syncer.Sync(spaceId, "http://url.com", nil, objectorigin.Import(model.Import_Pb))

		// then
		assert.Equal(t, "newFileObjectId", newFileId)
	})

	t.Run("relation is local path", func(t *testing.T) {
		// given
		fileUploader := mock_syncer.NewMockBlockService(t)
		fileUploader.EXPECT().UploadFile(context.Background(), spaceId, block2.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "local path",
			},
			ObjectOrigin: objectorigin.Import(model.Import_Pb),
		}).Return("newFileObjectId", nil, nil)
		syncer := NewFileRelationSyncer(fileUploader, nil)

		// when
		newFileId := syncer.Sync(spaceId, "local path", nil, objectorigin.Import(model.Import_Pb))

		// then
		assert.Equal(t, "newFileObjectId", newFileId)
	})
}
