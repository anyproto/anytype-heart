package space

import (
	"context"
	"fmt"

	spaceservice "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) startLoad(ctx context.Context, spaceID string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := s.getStatus(spaceID)

	if status.LocalStatus != spaceinfo.LocalStatusUnknown {
		return nil
	}

	exists, err := s.techSpace.SpaceViewExists(ctx, spaceID)
	if err != nil {
		return
	}
	if !exists {
		return ErrSpaceNotExists
	}

	info := spaceinfo.SpaceInfo{
		SpaceID:     spaceID,
		LocalStatus: spaceinfo.LocalStatusLoading,
	}
	if err = s.setStatus(ctx, info); err != nil {
		return
	}
	s.loading[spaceID] = newLoadingSpace(s.ctx, s.open, spaceID, s.onLoad)
	return
}

func (s *service) onLoad(spaceID string, sp Space, loadErr error) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch loadErr {
	case nil:
	case spaceservice.ErrSpaceDeletionPending:
		return s.setStatus(s.ctx, spaceinfo.SpaceInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusWaitingDeletion,
		})
	case spaceservice.ErrSpaceIsDeleted:
		return s.setStatus(s.ctx, spaceinfo.SpaceInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusDeleted,
		})
	default:
		return s.setStatus(s.ctx, spaceinfo.SpaceInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusError,
		})
	}

	s.loaded[spaceID] = sp

	// TODO: check remote status
	return s.setStatus(s.ctx, spaceinfo.SpaceInfo{
		SpaceID:      spaceID,
		LocalStatus:  spaceinfo.LocalStatusOk,
		RemoteStatus: spaceinfo.RemoteStatusUnknown,
	})
}

func (s *service) waitLoad(ctx context.Context, spaceID string) (sp Space, err error) {
	s.mu.Lock()
	status := s.getStatus(spaceID)

	switch status.LocalStatus {
	case spaceinfo.LocalStatusUnknown:
		return nil, fmt.Errorf("waitLoad for an unknown space")
	case spaceinfo.LocalStatusLoading:
		// loading in progress, wait channel and retry
		waitCh := s.loading[spaceID].loadCh
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
		return s.waitLoad(ctx, spaceID)
	case spaceinfo.LocalStatusMissing:
		// local missing status means the loader ended with an error
		err = s.loading[spaceID].loadErr
	case spaceinfo.LocalStatusOk:
		sp = s.loaded[spaceID]
	default:
		err = fmt.Errorf("undefined space status: %v", status.LocalStatus)
	}
	s.mu.Unlock()
	return
}
