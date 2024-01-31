package syncer

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

var log = logging.Logger("import")

type IconSyncer struct {
	service           *block.Service
	resolver          idresolver.Resolver
	objectStore       objectstore.ObjectStore
	fileStore         filestore.FileStore
	fileObjectService fileobject.Service
}

func NewIconSyncer(service *block.Service, resolver idresolver.Resolver, fileStore filestore.FileStore, fileObjectService fileobject.Service, objectStore objectstore.ObjectStore) *IconSyncer {
	return &IconSyncer{
		service:           service,
		resolver:          resolver,
		fileStore:         fileStore,
		fileObjectService: fileObjectService,
		objectStore:       objectStore,
	}
}

func (s *IconSyncer) Sync(id domain.FullID, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, b simple.Block, origin domain.ObjectOrigin) error {
	iconImage := b.Model().GetText().GetIconImage()
	newId, err := s.handleIconImage(id.SpaceID, snapshotPayloads, iconImage, origin)
	if err != nil {
		return fmt.Errorf("%w: %w", common.ErrFileLoad, err)
	}
	if newId == iconImage {
		return nil
	}

	err = block.Do(s.service, id.ObjectID, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		upErr := updater.Update(nil, func(simpleBlock simple.Block) error {
			simpleBlock.Model().GetText().IconImage = newId
			return nil
		}, b.Model().Id)
		if upErr != nil {
			return fmt.Errorf("%w: %s", common.ErrFileLoad, err.Error())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, err.Error())
	}
	return nil
}

func (s *IconSyncer) handleIconImage(spaceId string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, iconImage string, origin domain.ObjectOrigin) (string, error) {
	if _, ok := snapshotPayloads[iconImage]; ok {
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

	req := pb.RpcFileUploadRequest{LocalPath: iconImage}
	if strings.HasPrefix(iconImage, "http://") || strings.HasPrefix(iconImage, "https://") {
		req = pb.RpcFileUploadRequest{Url: iconImage}
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: req,
		ObjectOrigin:         origin,
	}
	fileObjectId, _, err := s.service.UploadFile(context.Background(), spaceId, dto)
	if err != nil {
		return "", oserror.TransformError(err)
	}
	return fileObjectId, nil
}
