package syncer

import (
	"context"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

type FileRelationSyncer struct {
	service           BlockService
	fileObjectService fileobject.Service
}

func NewFileRelationSyncer(service BlockService, fileObjectService fileobject.Service) *FileRelationSyncer {
	return &FileRelationSyncer{
		service:           service,
		fileObjectService: fileObjectService,
	}
}

func (fs *FileRelationSyncer) Sync(spaceID string, fileId string, newIdsSet map[string]struct{}, origin objectorigin.ObjectOrigin) string {
	// If file is created during import, do nothing
	if _, ok := newIdsSet[fileId]; ok {
		return fileId
	}
	if fileId == addr.MissingObject {
		return fileId
	}

	var (
		fileObjectId string
		err          error
	)
	if strings.HasPrefix(fileId, "http://") || strings.HasPrefix(fileId, "https://") {
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{Url: fileId},
			ObjectOrigin:         origin,
		}
		fileObjectId, _, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
		return fileObjectId
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
		ObjectOrigin:         origin,
	}
	fileObjectId, _, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
	if err != nil {
		log.Errorf("file uploading %s", err)
	}
	return fileObjectId
}
