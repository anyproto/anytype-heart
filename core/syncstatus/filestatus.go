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
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const fileStatusPrefix = "/file_backup_status/"

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

func (s *service) updateFileStatus(fileHash string, status FileStatus) error {
	fileId, err := s.getFileIdByFileHash(fileHash)
	if err != nil {
		return fmt.Errorf("get file id by file hash: %w", err)
	}
	err = s.indexFileSyncStatus(fileId, status)
	if err != nil {
		return fmt.Errorf("index file sync status: %w", err)
	}
	err = badgerhelper.SetValue(s.badger, []byte(fileStatusPrefix+fileId), int(status))
	if err != nil {
		return fmt.Errorf("set file status: %w", err)
	}
	return nil
}

func (s *service) OnFileUploadStarted(fileHash string) error {
	return s.updateFileStatus(fileHash, FileStatusSyncing)
}

func (s *service) OnFileUploaded(fileHash string) error {
	return s.updateFileStatus(fileHash, FileStatusSynced)
}

func (s *service) OnFileLimited(fileHash string) error {
	return s.updateFileStatus(fileHash, FileStatusLimited)
}

func (s *service) getFileIdByFileHash(fileHash string) (string, error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileHash.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileHash),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("query objects by file hash: %w", err)
	}
	if len(records) == 0 {
		return "", fmt.Errorf("file object not found")
	}
	return pbtypes.GetString(records[0].Details, bundle.RelationKeyId.String()), nil
}

func (s *service) indexFileSyncStatus(fileId string, status FileStatus) error {
	err := s.updateReceiver.UpdateTree(context.Background(), fileId, status.ToSyncStatus())
	if err != nil {
		return fmt.Errorf("update tree: %w", err)
	}

	err = getblock.Do(s.objectGetter, fileId, func(sb smartblock.SmartBlock) (err error) {
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
	return err
}
