package syncer

import (
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

type FileSyncer struct {
	service           BlockService
	fileObjectService fileobject.Service
}

func NewFileSyncer(
	service BlockService,
	fileObjectService fileobject.Service,
) *FileSyncer {
	return &FileSyncer{
		service:           service,
		fileObjectService: fileObjectService,
	}
}

func (s *FileSyncer) Sync(id domain.FullID, newIdsSet map[string]struct{}, b simple.Block, origin objectorigin.ObjectOrigin) error {
	if targetObjectId := b.Model().GetFile().GetTargetObjectId(); targetObjectId != "" {
		return nil
	}
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
	params.ContextId = id.ObjectID
	dto := block.UploadRequest{
		RpcBlockUploadRequest: params,
		ObjectOrigin:          origin,
	}
	_, err := s.service.UploadBlockFile(nil, dto, "", false)
	if os.IsNotExist(err) {
		return anyerror.CleanupError(err)
	}
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, anyerror.CleanupError(err).Error())
	}
	return nil
}

func (s *FileSyncer) migrateFile(objectId string, fileBlockId string, fileId domain.FullFileId, origin objectorigin.ObjectOrigin) error {
	fileObjectId, err := s.fileObjectService.CreateFromImport(fileId, origin)
	if err != nil {
		return fmt.Errorf("create file object: %w", err)
	}
	err = cache.Do(s.service, objectId, func(sb smartblock.SmartBlock) error {
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
