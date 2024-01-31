package objectid

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type file struct {
	service   *block.Service
	fileStore filestore.FileStore
}

func newFileObject(service *block.Service, fileStore filestore.FileStore) *file {
	return &file{service: service, fileStore: fileStore}
}

func (f *file) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, _ time.Time, _ bool, origin *domain.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	filePath := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySource.String())
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	fileKeys, err := f.fileStore.GetFileKeys(id)
	if err == nil && len(fileKeys) > 0 {
		return id, treestorage.TreeStorageCreatePayload{}, nil
	}
	params := pb.RpcFileUploadRequest{
		SpaceId:   spaceID,
		LocalPath: filePath,
	}
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		params = pb.RpcFileUploadRequest{
			SpaceId:   spaceID,
			LocalPath: filePath,
		}
	}
	dto := block.FileUploadRequest{
		RpcFileUploadRequest: params,
		ObjectOrigin:         origin,
	}

	filesKeys := make(map[string]string, 0)
	for _, fileKeys := range sn.Snapshot.FileKeys {
		if fileKeys.Hash == id {
			filesKeys = fileKeys.Keys
			break
		}
	}
	hash, err := f.service.UploadFile(ctx, spaceID, dto, filesKeys)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	return hash, treestorage.TreeStorageCreatePayload{}, nil
}
