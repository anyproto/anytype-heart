package syncstatus

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	typeProvider              typeprovider.SmartBlockTypeProvider
	fileSyncService           filesync.FileSync
	fileWatcherUpdateInterval time.Duration

	objectWatchersLock sync.Mutex

	objectStore  objectstore.ObjectStore
	objectGetter cache.ObjectGetter
	badger       *badger.DB

	spaceSyncStatus spacesyncstatus.Updater
}

func New(fileWatcherUpdateInterval time.Duration) Service {
	return &service{
		fileWatcherUpdateInterval: fileWatcherUpdateInterval,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.typeProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	// todo: start using sqlite db
	db, err := dbProvider.SpaceStorage()
	if err != nil {
		if errors.Is(err, clientds.ErrSpaceStoreNotAvailable) {
			db, err = dbProvider.LocalStorage()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	s.badger = db

	s.updateReceiver = newUpdateReceiver(nodeConfService, cfg, eventSender, s.objectStore)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)

	s.fileSyncService.OnUploaded(s.OnFileUploaded)
	s.fileSyncService.OnUploadStarted(s.OnFileUploadStarted)
	s.fileSyncService.OnLimited(s.OnFileLimited)

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
