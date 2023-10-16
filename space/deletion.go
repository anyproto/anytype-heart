package space

import (
	"context"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	status := s.getStatus(id)
	status.AccountStatus = spaceinfo.AccountStatusDeleted
	err := s.setStatus(ctx, status)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if status.RemoteStatus != spaceinfo.RemoteStatusDeleted || status.RemoteStatus != spaceinfo.RemoteStatusWaitingDeletion {
		_, err := s.spaceCore.Delete(ctx, id)
		if err != nil {
			log.Warn("network delete error", zap.Error(err), zap.String("spaceId", id))
		}
	}
	if status.LocalStatus == spaceinfo.LocalStatusMissing {
		return nil
	}
	err = s.offload(ctx, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	status = s.getStatus(id)
	status.LocalStatus = spaceinfo.LocalStatusMissing
	err = s.setStatus(ctx, status)
	s.mu.Unlock()
	return err
}

func (s *service) offload(ctx context.Context, id string) (err error) {
	sp, err := s.Get(ctx, id)
	if err != nil && err != ErrSpaceDeleted {
		return err
	}
	if err == nil {
		err = sp.Close(ctx)
		if err != nil {
			return
		}
	}
	err = s.storageService.DeleteSpaceStorage(ctx, id)
	if err != nil {
		return
	}
	return s.indexer.RemoveIndexes(id)
}
