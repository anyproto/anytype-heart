package objectid

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type file struct {
	service *block.Service
}

func newFileObject(service *block.Service) *file {
	return &file{service: service}
}

func (f *file) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, _ time.Time, _ bool) (string, treestorage.TreeStorageCreatePayload, error) {
	filePath := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySource.String())
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
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
		Origin:               model.ObjectOrigin_import,
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
