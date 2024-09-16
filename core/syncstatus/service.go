package syncstatus

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "status"

type Service interface {
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	fileSyncService filesync.FileSync
	objectGetter    cache.ObjectGetter
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.fileSyncService.OnUploaded(s.onFileUploaded)
	s.fileSyncService.OnUploadStarted(s.onFileUploadStarted)
	s.fileSyncService.OnLimited(s.onFileLimited)
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}

func (s *service) onFileUploadStarted(objectId string, _ domain.FullFileId) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Syncing)
}

func (s *service) onFileUploaded(objectId string, _ domain.FullFileId) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Synced)
}

func (s *service) onFileLimited(objectId string, _ domain.FullFileId, bytesLeftPercentage float64) error {
	return s.indexFileSyncStatus(objectId, filesyncstatus.Limited)
}

func (s *service) indexFileSyncStatus(fileObjectId string, status filesyncstatus.Status) error {
	err := cache.Do(s.objectGetter, fileObjectId, func(sb smartblock.SmartBlock) (err error) {
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
	return nil
}
