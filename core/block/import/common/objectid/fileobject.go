package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fileObject struct {
	treeObject *treeObject

	blockService      *block.Service
	fileService       files.Service
	fileObjectService fileobject.Service
}

func (o *fileObject) GetIDAndPayload(ctx context.Context, spaceId string, sn *common.Snapshot, timestamp time.Time, getExisting bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := o.treeObject.GetIDAndPayload(ctx, spaceId, sn, timestamp, getExisting, origin)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}

	filePath := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySource.String())
	if filePath != "" {
		fileObjectId, err := uploadFile(ctx, o.blockService, spaceId, filePath, origin)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("upload file: %w", err)
		}
		return fileObjectId, treestorage.TreeStorageCreatePayload{}, nil
	}
	return id, payload, nil
}
