package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	fileSyncService filesync.FileSync

	objectWatchersLock sync.Mutex
	objectWatchers     map[string]StatusWatcher

	objectStore  objectstore.ObjectStore
	objectGetter cache.ObjectGetter
	badger       *badger.DB

	spaceSyncStatus spacesyncstatus.Updater
}

func New() Service {
	return &service{
		objectWatchers: map[string]StatusWatcher{},
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

	s.spaceSyncStatus = app.MustComponent[spacesyncstatus.Updater](a)
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
