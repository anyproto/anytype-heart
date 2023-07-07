package block

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/session"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var errAppIsNotRunning = errors.New("app is not running")

const (
	concurrentTrees = 10
)

func (s *Service) StartSync() {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	s.syncStarted = true
	for _, syncer := range s.syncer {
		syncer.Run()
	}
}

func (s *Service) DeleteSpace(ctx context.Context, spaceID string) error {
	log.Debug("space deleted", zap.String("spaceID", spaceID))
	return nil
}

func (s *Service) DeleteObject(ctx session.Context, id string) (err error) {
	err = Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return
	}

	space, err := s.spaceService.GetSpace(context.Background(), ctx.SpaceID())
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	sbt, _ := s.sbtProvider.Type(ctx.SpaceID(), id)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		err = s.OnDelete(ctx, id, func() error {
			return Do(s, ctx, s.anytype.PredefinedObjects(ctx.SpaceID()).Account, func(w *editor.Workspaces) error {
				return w.DeleteSubObject(id)
			})
		})
	case coresb.SmartBlockTypeFile:
		err = s.OnDelete(ctx, id, func() error {
			if err := s.fileStore.DeleteFile(id); err != nil {
				return err
			}
			if err := s.fileSync.RemoveFile(ctx.SpaceID(), id); err != nil {
				return fmt.Errorf("failed to remove file from sync: %w", err)
			}
			_, err = s.fileService.FileOffload(ctx, id, true)
			if err != nil {
				return err
			}
			return nil
		})
	default:
		// this will call DeleteTree asynchronously in the end
		return space.DeleteTree(context.Background(), id)
	}
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(ctx.SpaceID(), id)
	err = space.RemoveObjectFromCache(context.Background(), id)
	return
}
