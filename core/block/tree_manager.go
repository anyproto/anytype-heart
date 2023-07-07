package block

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/space"
)

var _ treemanager.TreeManager = (*Service)(nil)

// GetTree should only be called by either space services or debug apis, not the client code
func (s *Service) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	if !s.anytype.IsStarted() {
		err = errAppIsNotRunning
		return
	}
	space, err := s.spaceService.GetSpace(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	obj, err := space.GetObject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	sb := obj.Inner()
	return sb.(source.ObjectTreeProvider).Tree(), nil
}

func (s *Service) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	sctx := session.NewContext(ctx, spaceId)
	err := s.OnDelete(sctx, treeId, nil)
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

	space, err := s.spaceService.GetSpace(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	obj, err := space.GetObject(ctx, treeId)
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	s.MarkTreeDeleted(ctx, spaceId, treeId)
	// this should be done not inside lock
	// TODO: looks very complicated, I know
	err = obj.(smartblock.SmartBlock).Inner().(source.ObjectTreeProvider).Tree().Delete()
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(spaceId, treeId)
	err = space.RemoveObjectFromCache(ctx, treeId)
	return
}

func (s *Service) NewTreeSyncer(spaceId string, treeManager treemanager.TreeManager) treemanager.TreeSyncer {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	syncer := newTreeSyncer(spaceId, space.ObjectLoadTimeout, concurrentTrees, treeManager)
	s.syncer[spaceId] = syncer
	if s.syncStarted {
		log.With("spaceID", spaceId).Warn("creating tree syncer after run")
		syncer.Run()
	}
	return syncer
}
