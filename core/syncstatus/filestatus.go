package syncstatus

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) onFileUploadStarted(objectId string, _ domain.FullFileId) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Syncing)
}

func (s *service) onFileUploaded(objectId string, _ domain.FullFileId) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Synced)
}

func (s *service) onFileLimited(objectId string, _ domain.FullFileId, bytesLeftPercentage float64) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Limited)
}

func (s *service) OnFileDelete(fileId domain.FullFileId) {
	s.spaceSyncStatus.Refresh(fileId.SpaceId)
}

func (s *service) indexFileSyncStatus(fileObjectId string, status filesyncstatus.Status) error {
	var spaceId string
	err := cache.Do(s.objectGetter, fileObjectId, func(sb smartblock.SmartBlock) (err error) {
		spaceId = sb.SpaceID()
		prevStatus := pbtypes.GetInt64(sb.Details(), bundle.RelationKeyFileBackupStatus.String())
		newStatus := int64(status)
		if prevStatus == newStatus {
			return nil
		}
		st := sb.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, pbtypes.Int64(newStatus))
		return sb.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	err = s.updateReceiver.UpdateTree(context.Background(), fileObjectId, status.ToSyncStatus())
	if err != nil {
		return fmt.Errorf("update tree: %w", err)
	}
	s.spaceSyncStatus.Refresh(spaceId)
	return nil
}
