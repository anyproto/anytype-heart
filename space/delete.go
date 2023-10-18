package space

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const deleteStorageLockTimeout = time.Second * 10

func (s *service) Delete(ctx context.Context, id string) error {
	err := s.startDelete(ctx, id)
	if err != nil {
		return err
	}
	return s.waitOffload(ctx, id)
}

func (s *service) startOffload(ctx context.Context, id string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.loading[id]; ok {
		return nil
	}
	if _, ok := s.offloading[id]; ok {
		return nil
	}
	if _, ok := s.offloaded[id]; ok {
		return nil
	}
	status := s.getStatus(id)
	if status.LocalStatus == spaceinfo.LocalStatusMissing {
		s.offloaded[id] = struct{}{}
		return
	}
	status.AccountStatus = spaceinfo.AccountStatusDeleted
	err = s.setStatus(ctx, status)
	if err != nil {
		return err
	}
	s.offloading[id] = newOffloadingSpace(ctx, id, s)
	return nil
}

func (s *service) onOffload(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.offloading, id)
	delete(s.loaded, id)
	if err != nil {
		log.Warn("offload error", zap.Error(err), zap.String("spaceId", id))
		return
	}
	status := s.getStatus(id)
	status.LocalStatus = spaceinfo.LocalStatusMissing
	_ = s.setStatus(s.ctx, status)
	s.offloaded[id] = struct{}{}
}

func (s *service) waitOffload(ctx context.Context, id string) (err error) {
	s.mu.Lock()
	if _, ok := s.offloaded[id]; ok {
		s.mu.Unlock()
		return nil
	}
	offloading, ok := s.offloading[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("space %s is not offloading", id)
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-offloading.loadCh:
		return offloading.loadErr
	}
}

func (s *service) startDelete(ctx context.Context, id string) error {
	s.mu.Lock()
	status := s.getStatus(id)
	if status.AccountStatus != spaceinfo.AccountStatusDeleted {
		status.AccountStatus = spaceinfo.AccountStatusDeleted
		err := s.setStatus(ctx, status)
		if err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.mu.Unlock()
	if !status.RemoteStatus.IsDeleted() {
		_, err := s.spaceCore.Delete(ctx, id)
		if err != nil {
			log.Warn("network delete error", zap.Error(err), zap.String("spaceId", id))
		}
	}
	return s.startOffload(ctx, id)
}

func (s *service) offload(ctx context.Context, id string) (err error) {
	s.mu.Lock()
	if sp, ok := s.loaded[id]; ok {
		s.mu.Unlock()
		err = sp.Close(ctx)
		if err != nil {
			return
		}
		s.mu.Lock()
	}
	delete(s.loaded, id)
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, deleteStorageLockTimeout)
	err = s.storageService.DeleteSpaceStorage(ctx, id)
	cancel()
	if err != nil {
		return
	}
	err = s.indexer.RemoveIndexes(id)
	if err != nil {
		return err
	}
	return s.offloader.FilesSpaceOffload(ctx, id)
}
