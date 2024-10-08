package objectid

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type fileObject struct {
	treeObject *treeObject

	blockService *block.Service
}

func (o *fileObject) GetIDAndPayload(ctx context.Context, spaceId string, sn *common.Snapshot, timestamp time.Time, getExisting bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := o.treeObject.GetIDAndPayload(ctx, spaceId, sn, timestamp, getExisting, origin)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}

	filePath := sn.Snapshot.Data.Details.GetString(bundle.RelationKeySource)
	if filePath != "" {
		var encryptionKeys map[string]string
		if sn.Snapshot.Data.FileInfo != nil {
			encryptionKeys = make(map[string]string, len(sn.Snapshot.Data.FileInfo.EncryptionKeys))
			for _, key := range sn.Snapshot.Data.FileInfo.EncryptionKeys {
				encryptionKeys[key.Path] = key.Key
			}
		}
		name := sn.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
		fileObjectId, err := uploadFile(ctx, o.blockService, spaceId, name, filePath, origin, encryptionKeys)
		if err != nil {
			log.Error("handling file object: upload file", zap.Error(err))
			return id, payload, nil
		}
		return fileObjectId, treestorage.TreeStorageCreatePayload{}, nil
	}
	return id, payload, nil
}
