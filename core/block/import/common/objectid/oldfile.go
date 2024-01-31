package objectid

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// oldFile represents file in pre Files-as-Objects format
type oldFile struct {
	blockService      *block.Service
	fileStore         filestore.FileStore
	fileObjectService fileobject.Service
}

func (f *oldFile) GetIDAndPayload(ctx context.Context, spaceId string, sn *common.Snapshot, _ time.Time, _ bool, origin domain.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	filePath := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySource.String())
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())

	filesKeys := map[string]string{}
	for _, fileKeys := range sn.Snapshot.FileKeys {
		if fileKeys.Hash == id {
			filesKeys = fileKeys.Keys
			break
		}
	}

	// If we got keys we can just create file object with existing file CID
	if len(filesKeys) > 0 {
		err := f.fileStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(id),
			EncryptionKeys: filesKeys,
		})
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("add file keys: %w", err)
		}
		objectId, err := f.fileObjectService.CreateFromImport(domain.FullFileId{SpaceId: spaceId, FileId: domain.FileId(id)}, origin)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create file object: %w", err)
		}
		return objectId, treestorage.TreeStorageCreatePayload{}, nil
	}

	params := pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: filePath,
	}
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		params = pb.RpcFileUploadRequest{
			SpaceId:   spaceId,
			LocalPath: filePath,
		}
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: params,
		ObjectOrigin:         origin,
	}

	hash, _, err := f.blockService.UploadFile(ctx, spaceId, dto)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	return hash, treestorage.TreeStorageCreatePayload{}, nil
}
