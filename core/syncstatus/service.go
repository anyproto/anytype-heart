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
	Watch(id string, fileFunc func() []string) (new bool, err error)
	Unwatch(id string)
	OnFileUpload(spaceID string, fileID string) error
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	typeProvider typeprovider.SmartBlockTypeProvider
	spaceService space.Service

	coreService core.Service

	fileWatcher        *fileWatcher
	objectWatcher      *objectWatcher
	subObjectsWatcher  *subObjectsWatcher
	linkedFilesWatcher *linkedFilesWatcher
}

func New(
	typeProvider typeprovider.SmartBlockTypeProvider,
	dbProvider datastore.Datastore,
	spaceService space.Service,
	coreService core.Service,
	fileSyncService filesync.FileSync,
	nodeConfService nodeconf.Service,
	fileStore filestore.FileStore,
	picker getblock.Picker,
	cfg *config.Config,
	eventSender event.Sender,
	fileWatcherUpdateInterval time.Duration,
) Service {
	fileStatusRegistry := newFileStatusRegistry(fileSyncService, fileStore, picker, fileWatcherUpdateInterval)
	linkedFilesWatcher := newLinkedFilesWatcher(spaceService, fileStatusRegistry)
	subObjectsWatcher := newSubObjectsWatcher()
	updateReceiver := newUpdateReceiver(coreService, linkedFilesWatcher, subObjectsWatcher, nodeConfService, cfg, eventSender)
	fileWatcher := newFileWatcher(spaceService, dbProvider, fileStatusRegistry, updateReceiver, fileWatcherUpdateInterval)
	objectWatcher := newObjectWatcher(spaceService, updateReceiver)
	return &service{
		spaceService:       spaceService,
		typeProvider:       typeProvider,
		coreService:        coreService,
		fileWatcher:        fileWatcher,
		objectWatcher:      objectWatcher,
		subObjectsWatcher:  subObjectsWatcher,
		linkedFilesWatcher: linkedFilesWatcher,
	}
}

func (s *service) Init(a *app.App) (err error) {
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

	_, err = s.watch(s.coreService.PredefinedBlocks().Account, nil)
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) Watch(id string, filesGetter func() []string) (new bool, err error) {
	return s.watch(id, filesGetter)
}

func (s *service) Unwatch(id string) {

	s.unwatch(id)
}

func (s *service) watch(id string, filesGetter func() []string) (new bool, err error) {
	sbt, err := s.typeProvider.Type(id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	switch sbt {
	case smartblock.SmartBlockTypeFile:
		err := s.fileWatcher.Watch(s.spaceService.AccountId(), id)
		return false, err
	case smartblock.SmartBlockTypeSubObject:
		s.subObjectsWatcher.Watch(id)
		return true, nil
	default:
		s.linkedFilesWatcher.WatchLinkedFiles(id, filesGetter)
		if err = s.objectWatcher.Watch(id); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (s *service) unwatch(id string) {
	sbt, err := s.typeProvider.Type(id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
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
	s.unwatch(s.coreService.PredefinedBlocks().Account)
	s.fileWatcher.close()
	s.linkedFilesWatcher.close()

	return nil
}
