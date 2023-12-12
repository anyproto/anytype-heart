package syncer

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

type FileSyncer struct {
	service *block.Service
}

func NewFileSyncer(
	service *block.Service,
) *FileSyncer {
	return &FileSyncer{
		service: service,
	}
}

func (fs *FileSyncer) Sync(id string, b simple.Block, origin model.ObjectOrigin, importType model.ImportType) error {
	if hash := b.Model().GetFile().GetHash(); hash != "" {
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
		ImportType:            importType,
	}
	_, err := fs.service.UploadFileBlockWithHash(id, dto)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, oserror.TransformError(err).Error())
	}
	return nil
}
