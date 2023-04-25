package syncer

import (
	"fmt"
	"os"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/space"
)

type FileSyncer struct {
	service      *block.Service
	fileSync     filesync.FileSync
	spaceService space.Service
}

func NewFileSyncer(
	service *block.Service,
	fileSync filesync.FileSync,
	spaceService space.Service,
) *FileSyncer {
	return &FileSyncer{
		service:      service,
		fileSync:     fileSync,
		spaceService: spaceService,
	}
}

func (fs *FileSyncer) SyncExistingFile(fileID string) error {
	return fs.fileSync.AddFile(fs.spaceService.AccountId(), fileID)
}

func (fs *FileSyncer) Sync(ctx *session.Context, id string, b simple.Block) error {
	if hash := b.Model().GetFile().GetHash(); hash != "" {
		return nil
	}

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
	hash, err := fs.service.UploadFileBlockWithHash(ctx, id, params)

	if err != nil {
		return fmt.Errorf("failed syncing file: %s", err)
	}
	b.Model().GetFile().Hash = hash
	os.Remove(b.Model().GetFile().Name)
	return nil
}
