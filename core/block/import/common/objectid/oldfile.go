package objectid

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// oldFile represents file in pre Files-as-Objects format
type oldFile struct {
	blockService      *block.Service
	fileObjectService fileobject.Service
	objectStore       objectstore.ObjectStore
}

func (f *oldFile) GetIDAndPayload(ctx context.Context, spaceId string, sn *common.Snapshot, _ time.Time, _ bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	fileId := sn.Snapshot.Data.Details.GetString(bundle.RelationKeyId)
	filesKeys := map[string]string{}
	for _, fileKeys := range sn.Snapshot.FileKeys {
		if fileKeys.Hash == fileId {
			filesKeys = fileKeys.Keys
			break
		}
	}

	filePath := sn.Snapshot.Data.Details.GetString(bundle.RelationKeySource)
	if filePath != "" {
		fileObjectId, err := uploadFile(ctx, f.blockService, spaceId, filePath, origin, filesKeys, sn.Snapshot.Data.Details)
		if err != nil {
			log.Error("handling old file object: upload file", zap.Error(err))
		}
		if err == nil {
			return fileObjectId, treestorage.TreeStorageCreatePayload{}, nil
		}
	}

	err := f.objectStore.AddFileKeys(domain.FileEncryptionKeys{
		FileId:         domain.FileId(fileId),
		EncryptionKeys: filesKeys,
	})
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("add file keys: %w", err)
	}
	objectId, err := f.fileObjectService.CreateFromImport(domain.FullFileId{SpaceId: spaceId, FileId: domain.FileId(fileId)}, origin)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create file object: %w", err)
	}
	return objectId, treestorage.TreeStorageCreatePayload{}, nil
}

func uploadFile(
	ctx context.Context,
	blockService *block.Service,
	spaceId, filePath string,
	origin objectorigin.ObjectOrigin,
	encryptionKeys map[string]string,
	details *domain.Details,
) (string, error) {
	params := pb.RpcFileUploadRequest{
		SpaceId: spaceId,
		Details: details.CopyOnlyKeys(bundle.RelationKeyName, bundle.RelationKeyIsHiddenDiscovery).ToProto(),
	}

	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		params.Url = filePath
	} else {
		params.LocalPath = filePath
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: params,
		ObjectOrigin:         origin,
		CustomEncryptionKeys: encryptionKeys,
	}

	fileObjectId, _, _, err := blockService.UploadFile(ctx, spaceId, dto)
	if err != nil {
		return "", err
	}
	return fileObjectId, nil
}
