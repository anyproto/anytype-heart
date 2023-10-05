package syncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
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
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	Watch(spaceID string, id string, fileFunc func() []string) (new bool, err error)
	Unwatch(spaceID string, id string)
	OnFileUpload(spaceID string, fileID string) error
	RegisterSpace(space commonspace.Space)

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	typeProvider              typeprovider.SmartBlockTypeProvider
	fileSyncService           filesync.FileSync
	fileWatcherUpdateInterval time.Duration

	fileWatcher        *fileWatcher
	linkedFilesWatcher *linkedFilesWatcher

	objectWatchersLock sync.Mutex
	objectWatchers     map[string]syncstatus.StatusWatcher
}

func New(fileWatcherUpdateInterval time.Duration) Service {
	return &service{
		fileWatcherUpdateInterval: fileWatcherUpdateInterval,
		objectWatchers:            map[string]syncstatus.StatusWatcher{},
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.typeProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	personalIDProvider := app.MustComponent[personalIDProvider](a)
	coreService := app.MustComponent[core.Service](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	fileStore := app.MustComponent[filestore.FileStore](a)
	picker := app.MustComponent[getblock.ObjectGetter](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	fileStatusRegistry := newFileStatusRegistry(s.fileSyncService, fileStore, picker, s.fileWatcherUpdateInterval)
	s.linkedFilesWatcher = newLinkedFilesWatcher(fileStatusRegistry)
	s.updateReceiver = newUpdateReceiver(coreService, s.linkedFilesWatcher, nodeConfService, cfg, eventSender)
	s.fileWatcher = newFileWatcher(personalIDProvider, dbProvider, fileStatusRegistry, s.updateReceiver, s.fileWatcherUpdateInterval)

	s.fileSyncService.OnUpload(s.OnFileUpload)
	return s.fileWatcher.init()
}

func (s *service) Run(ctx context.Context) (err error) {

	err = s.fileWatcher.run()
	if err != nil {
		return fmt.Errorf("failed to run file watcher: %w", err)
	}
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) RegisterSpace(space commonspace.Space) {
	s.objectWatchersLock.Lock()
	defer s.objectWatchersLock.Unlock()

	watcher := space.SyncStatus().(syncstatus.StatusWatcher)
	watcher.SetUpdateReceiver(s.updateReceiver)
	s.objectWatchers[space.Id()] = watcher
}

func (s *service) UnregisterSpace(space commonspace.Space) {
	s.objectWatchersLock.Lock()
	defer s.objectWatchersLock.Unlock()

	// TODO: [MR] now we can't set a nil update receiver, but maybe it doesn't matter that much
	//  and we can just leave as it is, because no events will come through
	delete(s.objectWatchers, space.Id())
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
	default:
		s.linkedFilesWatcher.WatchLinkedFiles(spaceID, id, filesGetter)
		s.objectWatchersLock.Lock()
		defer s.objectWatchersLock.Unlock()
		objectWatcher := s.objectWatchers[spaceID]
		if objectWatcher != nil {
			if err = objectWatcher.Watch(id); err != nil {
				return false, err
			}
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
	default:
		s.linkedFilesWatcher.UnwatchLinkedFiles(id)
		s.objectWatchersLock.Lock()
		defer s.objectWatchersLock.Unlock()
		objectWatcher := s.objectWatchers[spaceID]
		if objectWatcher != nil {
			objectWatcher.Unwatch(id)
		}
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
	s.fileWatcher.close()
	s.linkedFilesWatcher.close()

	return nil
}
