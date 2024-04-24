package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	Watch(spaceId string, id string, filesGetter func() []string) (new bool, err error)
	Unwatch(spaceID string, id string)
	RegisterSpace(space commonspace.Space)

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	fileSyncService filesync.FileSync

	objectWatchersLock sync.Mutex
	objectWatchers     map[string]syncstatus.StatusWatcher

	objectStore  objectstore.ObjectStore
	objectGetter cache.ObjectGetter
	badger       *badger.DB

	spaceSyncStatus syncstatus.SpaceSyncStatusUpdater
}

func New() Service {
	return &service{
		objectWatchers: map[string]syncstatus.StatusWatcher{},
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	s.updateReceiver = newUpdateReceiver(nodeConfService, cfg, eventSender, s.objectStore)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)

	s.fileSyncService.OnUploaded(s.onFileUploaded)
	s.fileSyncService.OnUploadStarted(s.onFileUploadStarted)
	s.fileSyncService.OnLimited(s.onFileLimited)

	s.spaceSyncStatus = app.MustComponent[syncstatus.SpaceSyncStatusUpdater](a)
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
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

func (s *service) Unwatch(spaceID string, id string) {
	s.unwatch(spaceID, id)
}

func (s *service) Watch(spaceId string, id string, filesGetter func() []string) (new bool, err error) {
	s.updateReceiver.ClearLastObjectStatus(id)

	s.objectWatchersLock.Lock()
	defer s.objectWatchersLock.Unlock()
	objectWatcher := s.objectWatchers[spaceId]
	if objectWatcher != nil {
		if err = objectWatcher.Watch(id); err != nil {
			return false, err
		}
	}
	return true, nil

}

func (s *service) unwatch(spaceID string, id string) {
	s.updateReceiver.ClearLastObjectStatus(id)

	s.objectWatchersLock.Lock()
	defer s.objectWatchersLock.Unlock()
	objectWatcher := s.objectWatchers[spaceID]
	if objectWatcher != nil {
		objectWatcher.Unwatch(id)
	}
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}
