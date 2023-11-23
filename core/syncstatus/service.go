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
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	WatchFile(spaceId string, fileId string, fileHash string) error
	Watch(spaceId string, id string, filesGetter func() []string) (new bool, err error)
	Unwatch(spaceID string, id string)
	OnFileUploaded(fileHash string) error
	OnFileLimited(fileHash string) error
	RegisterSpace(space commonspace.Space)

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	updateReceiver *updateReceiver

	typeProvider              typeprovider.SmartBlockTypeProvider
	fileSyncService           filesync.FileSync
	fileWatcherUpdateInterval time.Duration

	objectWatchersLock sync.Mutex
	objectWatchers     map[string]syncstatus.StatusWatcher

	objectStore  objectstore.ObjectStore
	objectGetter getblock.ObjectGetter
	badger       *badger.DB
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
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	nodeConfService := app.MustComponent[nodeconf.Service](a)
	cfg := app.MustComponent[*config.Config](a)
	eventSender := app.MustComponent[event.Sender](a)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	db, err := dbProvider.SpaceStorage()
	if err != nil {
		return fmt.Errorf("get badger from provider: %w", err)
	}
	s.badger = db

	s.updateReceiver = newUpdateReceiver(nodeConfService, cfg, eventSender, s.objectStore)
	s.objectGetter = app.MustComponent[getblock.ObjectGetter](a)

	s.fileSyncService.OnUploaded(s.OnFileUploaded)
	s.fileSyncService.OnUploadStarted(s.OnFileUploadStarted)
	s.fileSyncService.OnLimited(s.OnFileLimited)
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

func (s *service) WatchFile(spaceId string, fileId string, fileHash string) error {
	// err := s.fileWatcher.Watch(spaceId, fileId, fileHash)
	// return err
	return nil
}

func (s *service) Watch(spaceId string, id string, filesGetter func() []string) (new bool, err error) {
	s.updateReceiver.ClearLastObjectStatus(id)

	//s.linkedFilesWatcher.WatchLinkedFiles(space.Id(), id, filesGetter)
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
	sbt, err := s.typeProvider.Type(spaceID, id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	s.updateReceiver.ClearLastObjectStatus(id)
	switch sbt {
	case smartblock.SmartBlockTypeFile:
		// File watcher unwatches files automatically
	default:
		s.objectWatchersLock.Lock()
		defer s.objectWatchersLock.Unlock()
		objectWatcher := s.objectWatchers[spaceID]
		if objectWatcher != nil {
			objectWatcher.Unwatch(id)
		}
	}
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}
