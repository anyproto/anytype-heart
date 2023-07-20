package syncer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
)

type IconSyncer struct {
	service *block.Service
	picker  getblock.Picker
}

func NewIconSyncer(service *block.Service, picker getblock.Picker) *IconSyncer {
	return &IconSyncer{service: service, picker: picker}
}

func (is *IconSyncer) Sync(id string, b simple.Block) error {
	fileName := b.Model().GetText().GetIconImage()
	req := pb.RpcFileUploadRequest{LocalPath: fileName}
	if strings.HasPrefix(fileName, "http://") || strings.HasPrefix(fileName, "https://") {
		req = pb.RpcFileUploadRequest{Url: fileName}
	}
	spaceID, err := is.service.ResolveSpaceID(id)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	hash, err := is.service.UploadFile(context.Background(), spaceID, req)
	if err != nil {
		return fmt.Errorf("failed uploading icon image file: %s", err)
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
	os.Remove(fileName)
	return nil
}
