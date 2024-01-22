package syncer

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type FileSyncer struct {
	service           *block.Service
	objectStore       objectstore.ObjectStore
	fileStore         filestore.FileStore
	fileObjectService fileobject.Service
}

func NewFileSyncer(
	service *block.Service,
	fileStore filestore.FileStore,
	fileObjectService fileobject.Service,
	objectStore objectstore.ObjectStore,
) *FileSyncer {
	return &FileSyncer{
		service:           service,
		fileStore:         fileStore,
		fileObjectService: fileObjectService,
		objectStore:       objectStore,
	}
}

func (s *FileSyncer) Sync(id domain.FullID, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, b simple.Block, origin model.ObjectOrigin) error {
	if hash := b.Model().GetFile().GetHash(); hash != "" {
		err := s.migrateFile(id.ObjectID, b.Model().Id, domain.FullFileId{
			FileId:  domain.FileId(hash),
			SpaceId: id.SpaceID,
		}, origin)
		if err != nil {
			return fmt.Errorf("%w: %w", common.ErrFileLoad, err)
		}
		return nil
	}
	if hash := b.Model().GetFile().GetTargetObjectId(); hash != "" {
		return nil
	}
	if b.Model().GetFile().Name == "" {
		return nil
	}
	if b.Model().GetFile().State == model.BlockContentFile_Error {
		// we store error in the name field in case of error
		return nil
	}
	// todo: name unknown format. handle state?
	params := pb.RpcBlockUploadRequest{
		FilePath: b.Model().GetFile().Name,
		BlockId:  b.Model().Id,
	}
	if strings.HasPrefix(b.Model().GetFile().Name, "http://") || strings.HasPrefix(b.Model().GetFile().Name, "https://") {
		params = pb.RpcBlockUploadRequest{
			Url:     b.Model().GetFile().Name,
			BlockId: b.Model().Id,
		}
	}
	dto := block.UploadRequest{
		RpcBlockUploadRequest: params,
		Origin:                origin,
	}
	_, err := s.service.UploadFileBlock(id.ObjectID, dto)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, oserror.TransformError(err).Error())
	}
	return nil
}

func (s *FileSyncer) migrateFile(objectId string, fileBlockId string, fileId domain.FullFileId, origin model.ObjectOrigin) error {
	fileObjectId, err := createFileObject(s.objectStore, s.fileStore, s.fileObjectService, fileId, origin)
	if err != nil {
		return fmt.Errorf("create file object: %w", err)
	}
	err = block.Do(s.service, objectId, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		return updater.Update(nil, func(simpleBlock simple.Block) error {
			simpleBlock.Model().GetFile().TargetObjectId = fileObjectId
			return nil
		}, fileBlockId)
	})
	if err != nil {
		return fmt.Errorf("update file block: %w", err)
	}
	return nil
}

func createFileObject(objectStore objectstore.ObjectStore, fileStore filestore.FileStore, fileObjectService fileobject.Service, fileId domain.FullFileId, origin model.ObjectOrigin) (string, error) {
	// Check that fileId is not a file object id
	recs, _, err := objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.SpaceId),
			},
		},
	})
	if err == nil && len(recs) > 0 {
		return recs[0], nil
	}

	fileObjectId, _, err := fileObjectService.GetObjectDetailsByFileId(fileId)
	if err == nil {
		return fileObjectId, nil
	}
	keys, err := fileStore.GetFileKeys(fileId.FileId)
	if err != nil {
		return "", fmt.Errorf("get file keys: %w", err)
	}
	fileObjectId, _, err = fileObjectService.Create(context.Background(), fileId.SpaceId, fileobject.CreateRequest{
		FileId:         fileId.FileId,
		EncryptionKeys: keys,
		IsImported:     true,
		Origin:         origin,
	})
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}
	return fileObjectId, nil
}
