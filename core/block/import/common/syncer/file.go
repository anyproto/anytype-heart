package syncer

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
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

func (fs *FileSyncer) Sync(id string, b simple.Block, origin model.ObjectOrigin) error {
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
	}
	_, err := fs.service.UploadFileBlock(id, dto)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrFileLoad, oserror.TransformError(err).Error())
	}
	return nil
}

func createFileObject(fileStore filestore.FileStore, fileObjectService fileobject.Service, st *state.State, fileId domain.FullFileId, origin model.ObjectOrigin) (string, error) {
	keys, err := fileStore.GetFileKeys(fileId.FileId)
	if err != nil {
		filesKeys := st.GetAndUnsetFileKeys()
		keys = map[string]string{}
		for _, fileKeys := range filesKeys {
			if fileKeys.Hash == fileId.FileId.String() {
				keys = fileKeys.Keys
				err = fileStore.AddFileKeys(domain.FileEncryptionKeys{
					FileId:         fileId.FileId,
					EncryptionKeys: keys,
				})
				if err != nil {
					return "", fmt.Errorf("add file keys: %w", err)
				}
				break
			}
		}
	}
	if len(keys) == 0 {
		log.With("fileId", fileId.FileId.String()).Warnf("encryption keys not found")
	}
	fileObjectId, _, err := fileObjectService.Create(context.Background(), fileId.SpaceId, fileobject.CreateRequest{
		FileId:         fileId.FileId,
		EncryptionKeys: keys,
		IsImported:     true,
		Origin:         origin,
	})
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}
	fmt.Println("CREATED", fileId.FileId.String(), fileObjectId)
	return fileObjectId, nil
}
