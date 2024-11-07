package syncer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ipfs/go-cid"

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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

var log = logging.Logger("import")

type IconSyncer struct {
	service           BlockService
	fileObjectService fileobject.Service
}

func NewIconSyncer(service BlockService, fileObjectService fileobject.Service) *IconSyncer {
	return &IconSyncer{
		service:           service,
		fileObjectService: fileObjectService,
	}
}

func (s *IconSyncer) Sync(id domain.FullID, newIdsSet map[string]struct{}, b simple.Block, origin objectorigin.ObjectOrigin) error {
	iconImage := b.Model().GetText().GetIconImage()
	if iconImage == addr.MissingObject {
		return nil
	}
	newId, err := s.handleIconImage(id.SpaceID, newIdsSet, iconImage, origin)
	if err != nil {
		uplErr := s.updateTextBlock(id, "", b)
		if uplErr != nil {
			return fmt.Errorf("%w: %s", common.ErrFileLoad, uplErr.Error())
		}
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("%w: %s", common.ErrFileLoad, err.Error())
	}
	if newId == iconImage {
		return nil
	}

	err = s.updateTextBlock(id, newId, b)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, err.Error())
	}
	return nil
}

func (s *IconSyncer) updateTextBlock(id domain.FullID, newId string, b simple.Block) error {
	return cache.Do(s.service, id.ObjectID, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		upErr := updater.Update(nil, func(simpleBlock simple.Block) error {
			simpleBlock.Model().GetText().IconImage = newId
			return nil
		}, b.Model().Id)
		if upErr != nil {
			return fmt.Errorf("%w: %s", common.ErrFileLoad, upErr.Error())
		}
		return nil
	})
}

func (s *IconSyncer) handleIconImage(spaceId string, newIdsSet map[string]struct{}, iconImage string, origin objectorigin.ObjectOrigin) (string, error) {
	if _, ok := newIdsSet[iconImage]; ok {
		return iconImage, nil
	}
	_, err := cid.Decode(iconImage)
	if err == nil {
		fileObjectId, err := s.fileObjectService.CreateFromImport(domain.FullFileId{SpaceId: spaceId, FileId: domain.FileId(iconImage)}, origin)
		if err != nil {
			log.With("fileId", iconImage).Errorf("create file object: %v", err)
			return iconImage, nil
		}
		return fileObjectId, nil
	}

	req := pb.RpcFileUploadRequest{LocalPath: iconImage, ImageKind: model.ImageKind_Icon}
	if strings.HasPrefix(iconImage, "http://") || strings.HasPrefix(iconImage, "https://") {
		req = pb.RpcFileUploadRequest{Url: iconImage}
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: req,
		ObjectOrigin:         origin,
	}
	fileObjectId, _, err := s.service.UploadFile(context.Background(), spaceId, dto)
	if err != nil {
		return "", anyerror.CleanupError(err)
	}
	return fileObjectId, nil
}
