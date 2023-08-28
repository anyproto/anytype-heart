package syncstatus

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	Watch(spaceID string, id string, fileFunc func() []string) (new bool, err error)
	Unwatch(spaceID string, id string)
	OnFileUpload(spaceID string, fileID string) error
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	typeProvider              typeprovider.SmartBlockTypeProvider
	spaceService              space.Service
	fileSyncService           filesync.FileSync
	fileWatcherUpdateInterval time.Duration

	coreService core.Service

	fileWatcher        *fileWatcher
	objectWatcher      *objectWatcher
	subObjectsWatcher  *subObjectsWatcher
	linkedFilesWatcher *linkedFilesWatcher
	updateReceiver     *updateReceiver
}

func New(fileWatcherUpdateInterval time.Duration) Service {
	return &service{
		fileWatcherUpdateInterval: fileWatcherUpdateInterval,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	s.typeProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.coreService = app.MustComponent[core.Service](a)
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	spaceService := app.MustComponent[space.Service](a)
	coreService := app.MustComponent[core.Service](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	fileStore := app.MustComponent[filestore.FileStore](a)
	picker := app.MustComponent[getblock.Picker](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	fileStatusRegistry := newFileStatusRegistry(s.fileSyncService, fileStore, picker, s.fileWatcherUpdateInterval)
	s.linkedFilesWatcher = newLinkedFilesWatcher(spaceService, fileStatusRegistry)
	s.subObjectsWatcher = newSubObjectsWatcher()
	s.updateReceiver = newUpdateReceiver(coreService, s.linkedFilesWatcher, s.subObjectsWatcher, nodeConfService, cfg, eventSender)
	s.fileWatcher = newFileWatcher(spaceService, dbProvider, fileStatusRegistry, s.updateReceiver, s.fileWatcherUpdateInterval)
	s.objectWatcher = newObjectWatcher(spaceService, s.updateReceiver)

	s.fileSyncService.OnUpload(s.OnFileUpload)
	return s.fileWatcher.init()
}

func (s *service) Run(ctx context.Context) (err error) {

	err = s.fileWatcher.run()
	if err != nil {
		return fmt.Errorf("failed to run file watcher: %w", err)
	}

	if err = s.objectWatcher.run(ctx); err != nil {
		return err
	}

	// TODO Iterate all spaces?
	_, err = s.watch(s.spaceService.AccountId(), s.coreService.AccountObjects().Workspace, nil)
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) Watch(spaceID string, id string, filesGetter func() []string) (new bool, err error) {
	return s.watch(spaceID, id, filesGetter)
}

func (s *service) Unwatch(spaceID string, id string) {
	s.unwatch(spaceID, id)
}

func (s *service) watch(spaceID string, id string, filesGetter func() []string) (new bool, err error) {
	sbt, err := s.typeProvider.Type(spaceID, id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	s.updateReceiver.ClearLastObjectStatus(id)
	switch sbt {
	case smartblock.SmartBlockTypeFile:
		err := s.fileWatcher.Watch(spaceID, id)
		return false, err
	case smartblock.SmartBlockTypeSubObject:
		s.subObjectsWatcher.Watch(id)
		return true, nil
	default:
		s.linkedFilesWatcher.WatchLinkedFiles(spaceID, id, filesGetter)
		if err = s.objectWatcher.Watch(id); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (s *service) unwatch(spaceID string, id string) {
	sbt, err := s.typeProvider.Type(spaceID, id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	s.updateReceiver.ClearLastObjectStatus(id)
	switch sbt {
	case smartblock.SmartBlockTypeFile:
		// File watcher unwatches files automatically
	case smartblock.SmartBlockTypeSubObject:
		s.subObjectsWatcher.Unwatch(id)
	default:
		s.linkedFilesWatcher.UnwatchLinkedFiles(id)
		s.objectWatcher.Unwatch(id)
	}
}

func (s *service) OnFileUpload(spaceID string, fileID string) error {
	_, err := s.fileWatcher.registry.setFileStatus(fileWithSpace{spaceID: spaceID, fileID: fileID}, fileStatus{
		status:    FileStatusSynced,
		updatedAt: time.Now(),
	})
	return err
}

func (s *service) Close(ctx context.Context) (err error) {
	// TODO Iterate all spaces?
	s.unwatch(s.spaceService.AccountId(), s.coreService.AccountObjects().Workspace)
	s.fileWatcher.close()
	s.linkedFilesWatcher.close()

	return nil
}
