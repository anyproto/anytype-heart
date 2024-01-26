package syncer

import (
	"context"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type RelationSyncer interface {
	Sync(spaceID string, fileId string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, origin model.ObjectOrigin) string
}

type FileRelationSyncer struct {
	service           *block.Service
	objectStore       objectstore.ObjectStore
	fileStore         filestore.FileStore
	fileObjectService fileobject.Service
}

func NewFileRelationSyncer(service *block.Service, fileStore filestore.FileStore, fileObjectService fileobject.Service, objectStore objectstore.ObjectStore) RelationSyncer {
	return &FileRelationSyncer{
		service:           service,
		fileStore:         fileStore,
		fileObjectService: fileObjectService,
		objectStore:       objectStore,
	}
}

func (fs *FileRelationSyncer) Sync(spaceID string, fileId string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, origin model.ObjectOrigin) string {
	var (
		fileObjectId string
		err          error
	)
	if strings.HasPrefix(fileId, "http://") || strings.HasPrefix(fileId, "https://") {
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{Url: fileId},
			Origin:               origin,
		}
		fileObjectId, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	} else {
		if _, ok := snapshotPayloads[fileId]; ok {
			return fileId
		}
		_, err = cid.Decode(fileId)
		if err == nil {
			fileObjectId, err = fs.fileObjectService.CreateFromImport(domain.FullFileId{SpaceId: spaceID, FileId: domain.FileId(fileId)}, origin)
			if err != nil {
				log.With("fileId", fileId).Errorf("create file object: %v", err)
				return fileId
			}
			return fileObjectId
		}
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{LocalPath: fileId},
			Origin:               origin,
		}
		fileObjectId, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	}
	return fileObjectId
}
