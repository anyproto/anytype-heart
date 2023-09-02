package block

import (
	"context"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"go.uber.org/zap"
)

/* TODO Extract to separate component
DEPS:
- s.cache
- s.OnDelete
- s.anytype
- s.syncerLock
- s.syncer
- s.getObject
*/

func (s *Service) StartSync() {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	s.syncStarted = true
	for _, syncer := range s.syncer {
		syncer.Run()
	}
}

// GetTree should only be called by either space services or debug apis, not the client code
func (s *Service) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	if !s.anytype.IsStarted() {
		err = errAppIsNotRunning
		return
	}

	v, err := s.getObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: id,
	})
	if err != nil {
		return
	}

	sb := v.(smartblock.SmartBlock)
	return sb.Tree(), nil
}

func (s *Service) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := s.OnDelete(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	}, nil)
	if err != nil {
		log.Error("failed to execute on delete for tree", zap.Error(err))
	}
	return err
}

// DeleteTree should only be called by space services
func (s *Service) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	if !s.anytype.IsStarted() {
		return errAppIsNotRunning
	}

	obj, err := s.getObject(ctx, domain.FullID{
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

	s.sendOnRemoveEvent(spaceId, treeId)
	_, err = s.cache.Remove(ctx, treeId)
	return
}

func (s *Service) NewTreeSyncer(spaceId string, treeManager treemanager.TreeManager) treemanager.TreeSyncer {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	syncer := newTreeSyncer(spaceId, objectLoadTimeout, concurrentTrees, treeManager)
	s.syncer[spaceId] = syncer
	if s.syncStarted {
		log.With("spaceID", spaceId).Warn("creating tree syncer after run")
		syncer.Run()
	}
	return syncer
}
