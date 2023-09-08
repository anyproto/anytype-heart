package block

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

const (
	concurrentTrees = 10
)

var errAppIsNotRunning = errors.New("app is not running")

type treeManager struct {
	coreService core.Service
	objectCache objectcache.Cache
	eventSender event.Sender

	onDelete func(id domain.FullID) error

	syncer      map[string]*treeSyncer
	syncStarted bool
	syncerLock  sync.Mutex
}

func newTreeManager(onDelete func(id domain.FullID) error) *treeManager {
	return &treeManager{
		onDelete: onDelete,
		syncer:   make(map[string]*treeSyncer),
	}
}

func (s *treeManager) init(a *app.App) {
	s.coreService = app.MustComponent[core.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.objectCache = app.MustComponent[objectcache.Cache](a)
}

func (s *treeManager) StartSync() {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	s.syncStarted = true
	for _, syncer := range s.syncer {
		syncer.Run()
	}
}

// GetTree should only be called by either space services or debug apis, not the client code
func (s *treeManager) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	if !s.coreService.IsStarted() {
		err = errAppIsNotRunning
		return
	}

	v, err := s.objectCache.GetObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: id,
	})
	if err != nil {
		return
	}

	sb := v.(smartblock.SmartBlock)
	return sb.Tree(), nil
}

func (s *treeManager) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := s.onDelete(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	})
	if err != nil {
		log.Error("failed to execute on delete for tree", zap.Error(err))
	}
	return err
}

// DeleteTree should only be called by space services
func (s *treeManager) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	if !s.coreService.IsStarted() {
		return errAppIsNotRunning
	}

	obj, err := s.objectCache.GetObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	})
	if err != nil {
		return
	}
	s.MarkTreeDeleted(ctx, spaceId, treeId)
	// this should be done not inside lock
	sb := obj.(smartblock.SmartBlock)
	err = sb.(source.ObjectTreeProvider).Tree().Delete()
	if err != nil {
		return
	}

	sendOnRemoveEvent(s.eventSender, treeId)
	err = s.objectCache.Remove(ctx, treeId)
	return
}

func (s *treeManager) NewTreeSyncer(spaceId string, treeManager treemanager.TreeManager) treemanager.TreeSyncer {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	syncer := newTreeSyncer(spaceId, objectcache.ObjectLoadTimeout, concurrentTrees, treeManager)
	s.syncer[spaceId] = syncer
	if s.syncStarted {
		log.With("spaceID", spaceId).Warn("creating tree syncer after run")
		syncer.Run()
	}
	return syncer
}
