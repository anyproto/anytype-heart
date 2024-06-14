package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

const CName = "status"

type Service interface {
	Watch(spaceId string, id string, filesGetter func() []string) (new bool, err error)
	Unwatch(spaceID string, id string)
	RegisterSpace(space commonspace.Space, sw objectsyncstatus.StatusWatcher)

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	fileSyncService filesync.FileSync

	objectWatchersLock sync.Mutex
	objectWatchers     map[string]objectsyncstatus.StatusWatcher

	objectStore  objectstore.ObjectStore
	objectGetter cache.ObjectGetter

	spaceSyncStatus spacesyncstatus.Updater
}

func New() Service {
	return &service{
		objectWatchers: map[string]objectsyncstatus.StatusWatcher{},
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	nodeStatus := app.MustComponent[nodestatus.NodeStatus](a)

	s.spaceSyncStatus = app.MustComponent[spacesyncstatus.Updater](a)
	s.updateReceiver = newUpdateReceiver(nodeConfService, cfg, eventSender, s.objectStore, nodeStatus)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)

	s.fileSyncService.OnUploaded(s.onFileUploaded)
	s.fileSyncService.OnUploadStarted(s.onFileUploadStarted)
	s.fileSyncService.OnLimited(s.onFileLimited)
	s.fileSyncService.OnDelete(s.OnFileDelete)
	s.fileSyncService.OnQueued(s.OnFileQueued)
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) RegisterSpace(space commonspace.Space, sw objectsyncstatus.StatusWatcher) {
	s.objectWatchersLock.Lock()
	defer s.objectWatchersLock.Unlock()

	sw.SetUpdateReceiver(s.updateReceiver)
	s.objectWatchers[space.Id()] = sw
	s.updateReceiver.spaceId = space.Id()
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
