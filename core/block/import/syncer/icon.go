package syncer

import (
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

var log = logging.Logger("import")

type IconSyncer struct {
	service *block.Service
}

func NewIconSyncer(service *block.Service) *IconSyncer {
	return &IconSyncer{service: service}
}

func (is *IconSyncer) Sync(ctx *session.Context, id string, b simple.Block) error {
	icon := b.Model().GetText().GetIconImage()
	_, err := cid.Decode(icon)
	if err == nil {
		return nil
	}
	req := pb.RpcFileUploadRequest{LocalPath: icon}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		req = pb.RpcFileUploadRequest{Url: icon}
	}
	hash, err := is.service.UploadFile(req)
	if err != nil {
		log.Errorf("failed uploading icon image file: %s", oserror.TransformError(err))
	}

	err = is.service.Do(id, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		upErr := updater.Update(ctx, func(simpleBlock simple.Block) error {
			simpleBlock.Model().GetText().IconImage = hash
			return nil
		}, b.Model().Id)
		if upErr != nil {
			return fmt.Errorf("failed to update block: %s", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update block: %s", err)
	}
	return nil
}
