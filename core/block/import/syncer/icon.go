package syncer

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

var log = logging.Logger("import")

type IconSyncer struct {
	service *block.Service
	picker  getblock.Picker
}

func NewIconSyncer(service *block.Service, picker getblock.Picker) *IconSyncer {
	return &IconSyncer{service: service, picker: picker}
}

func (is *IconSyncer) Sync(id string, b simple.Block) error {
	icon := b.Model().GetText().GetIconImage()
	_, err := cid.Decode(icon)
	if err == nil {
		return nil
	}
	req := pb.RpcFileUploadRequest{LocalPath: icon}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		req = pb.RpcFileUploadRequest{Url: icon}
	}
	spaceID, err := is.service.ResolveSpaceID(id)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	hash, err := is.service.UploadFile(context.Background(), spaceID, req)
	if err != nil {
		log.Errorf("failed uploading icon image file: %s", oserror.TransformError(err))
	}

	err = getblock.Do(is.picker, id, func(sb smartblock.SmartBlock) error {
		updater := sb.(basic.Updatable)
		upErr := updater.Update(nil, func(simpleBlock simple.Block) error {
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
