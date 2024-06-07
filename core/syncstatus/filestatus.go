package syncstatus

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) OnFileUploadStarted(objectId string) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Syncing)
}

func (s *service) OnFileUploaded(objectId string) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Synced)
}

func (s *service) OnFileLimited(objectId string) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Limited)
}

func (s *service) OnFileDelete(fileId domain.FullFileId) {
	s.sendSpaceStatusUpdate(filesyncstatus.Synced, fileId.SpaceId)
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
		detailsSetter, ok := sb.(basic.DetailsSettable)
		if !ok {
			return fmt.Errorf("setting of details is not supported for %T", sb)
		}
		details := provideFileStatusDetails(status, newStatus)
		return detailsSetter.SetDetails(nil, details, true)
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}

	err = s.updateReceiver.UpdateTree(context.Background(), fileObjectId, status.ToSyncStatus())
	if err != nil {
		return fmt.Errorf("update tree: %w", err)
	}

	s.sendSpaceStatusUpdate(status, spaceId)
	return nil
}

func provideFileStatusDetails(status filesyncstatus.Status, newStatus int64) []*model.Detail {
	syncStatus, syncError := getSyncStatus(status)
	details := make([]*model.Detail, 0, 4)
	details = append(details, &model.Detail{
		Key:   bundle.RelationKeySyncStatus.String(),
		Value: pbtypes.Int64(int64(syncStatus)),
	})
	details = append(details, &model.Detail{
		Key:   bundle.RelationKeySyncError.String(),
		Value: pbtypes.Int64(int64(syncError)),
	})
	details = append(details, &model.Detail{
		Key:   bundle.RelationKeySyncDate.String(),
		Value: pbtypes.Int64(time.Now().Unix()),
	})
	details = append(details, &model.Detail{
		Key:   bundle.RelationKeyFileBackupStatus.String(),
		Value: pbtypes.Int64(newStatus),
	})
	return details
}

func (s *service) sendSpaceStatusUpdate(status filesyncstatus.Status, spaceId string) {
	spaceStatus, spaceError := getSyncStatus(status)
	syncStatus := domain.MakeSyncStatus(spaceId, spaceStatus, 0, spaceError, domain.Files)
	s.spaceSyncStatus.SendUpdate(syncStatus)
}

func getSyncStatus(status filesyncstatus.Status) (domain.SyncStatus, domain.SyncError) {
	var (
		spaceStatus domain.SyncStatus
		spaceError  domain.SyncError
	)
	switch status {
	case filesyncstatus.Synced:
		spaceStatus = domain.Synced
	case filesyncstatus.Syncing:
		spaceStatus = domain.Syncing
	case filesyncstatus.Limited:
		spaceStatus = domain.Error
		spaceError = domain.StorageLimitExceed
	case filesyncstatus.Unknown:
		spaceStatus = domain.Error
		spaceError = domain.NetworkError
	}
	return spaceStatus, spaceError
}
