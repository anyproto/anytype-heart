package space

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) Delete(ctx context.Context, id string, deletionPeriod time.Duration) error {
	// TODO: do for non-0 deletion period
	status := s.getStatus(id)
	if err := s.checkDeletionPossible(status); err != nil {
		return err
	}
	status.RemoteStatus = spaceinfo.RemoteStatusDeleted
	err := s.setStatus(ctx, status)
	if err != nil {
		return err
	}
	err = s.delController.NetworkDelete(ctx, id)
	if err != nil {
		log.Warn("network delete error", zap.Error(err), zap.String("spaceId", id))
	}
	if status.LocalStatus == spaceinfo.LocalStatusMissing {
		return nil
	}
	err = s.offload(ctx, id)
	if err != nil {
		return err
	}
	status.LocalStatus = spaceinfo.LocalStatusMissing
	return nil
}

func (s *service) RevertDeletion(ctx context.Context, id string) (err error) {
	return nil
}

func (s *service) checkDeletionPossible(spaceInfo spaceinfo.SpaceInfo) error {
	// TODO: check if other conditions are needed
	switch spaceInfo.RemoteStatus {
	case spaceinfo.RemoteStatusWaitingDeletion:
		return ErrSpaceWaitingForDeletion
	default:
		break
	}
	return nil
}

func (s *service) offload(ctx context.Context, id string) (err error) {
	sp, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	err = sp.Close(ctx)
	if err != nil {
		return
	}
	err = s.storageService.DeleteSpaceStorage(ctx, id)
	if err != nil {
		return
	}
	return s.indexer.RemoveIndexes(id)
}
