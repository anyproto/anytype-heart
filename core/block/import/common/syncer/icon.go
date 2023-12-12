package syncer

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

var log = logging.Logger("import")

type IconSyncer struct {
	service  *block.Service
	resolver idresolver.Resolver
}

func NewIconSyncer(service *block.Service, resolver idresolver.Resolver) *IconSyncer {
	return &IconSyncer{service: service, resolver: resolver}
}

func (is *IconSyncer) Sync(id string, b simple.Block, origin model.ObjectOrigin, importType model.ImportType) error {
	icon := b.Model().GetText().GetIconImage()
	_, err := cid.Decode(icon)
	if err == nil {
		return nil
	}
	req := pb.RpcFileUploadRequest{LocalPath: icon}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		req = pb.RpcFileUploadRequest{Url: icon}
	}
	spaceID, err := is.resolver.ResolveSpaceID(id)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, err.Error())
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: req,
		Origin:               origin,
		ImportType:           importType,
	}
	hash, err := is.service.UploadFile(context.Background(), spaceID, dto)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, oserror.TransformError(err).Error())
	}

	err = block.Do(is.service, id, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		upErr := updater.Update(nil, func(simpleBlock simple.Block) error {
			simpleBlock.Model().GetText().IconImage = hash
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
