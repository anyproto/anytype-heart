package syncstatus

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type FileStatus int

// First constants must repeat syncstatus.SyncStatus constants for
// avoiding inconsistency with data stored in filestore
const (
	FileStatusUnknown FileStatus = iota
	FileStatusSynced
	FileStatusSyncing
	FileStatusLimited
)

func (s FileStatus) ToSyncStatus() syncstatus.SyncStatus {
	switch s {
	case FileStatusUnknown:
		return syncstatus.StatusUnknown
	case FileStatusSynced:
		return syncstatus.StatusSynced
	case FileStatusSyncing, FileStatusLimited:
		return syncstatus.StatusNotSynced
	default:
		return syncstatus.StatusUnknown
	}
}

func (s *service) OnFileUploadStarted(objectId string) error {
	return s.indexFileSyncStatus(objectId, FileStatusSyncing)
}

func (s *service) OnFileUploaded(objectId string) error {
	return s.indexFileSyncStatus(objectId, FileStatusSynced)
}

func (s *service) OnFileLimited(objectId string) error {
	return s.indexFileSyncStatus(objectId, FileStatusLimited)
}

func (s *service) indexFileSyncStatus(fileObjectId string, status FileStatus) error {
	err := getblock.Do(s.objectGetter, fileObjectId, func(sb smartblock.SmartBlock) (err error) {
		prevStatus := pbtypes.GetInt64(sb.Details(), bundle.RelationKeyFileBackupStatus.String())
		newStatus := int64(status)
		if prevStatus == newStatus {
			return nil
		}

		detailsSetter, ok := sb.(basic.DetailsSettable)
		if !ok {
			return fmt.Errorf("setting of details is not supported for %T", sb)
		}
		return detailsSetter.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeyFileBackupStatus.String(),
				Value: pbtypes.Int64(newStatus),
			},
		}, true)
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}

	err = s.updateReceiver.UpdateTree(context.Background(), fileObjectId, status.ToSyncStatus())
	if err != nil {
		return fmt.Errorf("update tree: %w", err)
	}
	return nil
}
