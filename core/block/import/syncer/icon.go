package syncer

import (
	"fmt"
	"os"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type IconSyncer struct {
	service *block.Service
}

func NewIconSyncer(service *block.Service) *IconSyncer {
	return &IconSyncer{service: service}
}

func (is *IconSyncer) Sync(ctx *session.Context, id string, b simple.Block) error {
	fileName := b.Model().GetText().GetIconImage()
	req := pb.RpcFileUploadRequest{LocalPath: fileName}
	if strings.HasPrefix(fileName, "http://") || strings.HasPrefix(fileName, "https://") {
		req = pb.RpcFileUploadRequest{Url: fileName}
	}
	hash, err := is.service.UploadFile(req)
	if err != nil {
		return fmt.Errorf("failed uploading icon image file: %s", err)
	}

	err = is.service.Do(id, func(sb smartblock.SmartBlock) error {
		bs := basic.NewBasic(sb, nil, nil)
		upErr := bs.Update(ctx, func(simpleBlock simple.Block) error {
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
	os.Remove(fileName)
	return nil
}
