package space

import (
	"context"

	spaceservice "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) startLoad(ctx context.Context, spaceID string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.techSpace.GetInfo(spaceID).LocalStatus != spaceinfo.LocalStatusUnknown {
		// loading already started
		return nil
	}

	viewID, err := s.techSpace.DeriveSpaceViewID(ctx, spaceID)
	if err != nil {
		return
	}

	err = s.techSpace.SetInfo(ctx, spaceinfo.SpaceInfo{
		SpaceID:     spaceID,
		ViewID:      viewID,
		LocalStatus: spaceinfo.LocalStatusLoading,
	})
	if err != nil {
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
		return s.techSpace.SetStatuses(s.ctx, spaceID, spaceinfo.LocalStatusMissing, spaceinfo.RemoteStatusWaitingDeletion)
	case spaceservice.ErrSpaceIsDeleted:
		return s.techSpace.SetStatuses(s.ctx, spaceID, spaceinfo.LocalStatusMissing, spaceinfo.RemoteStatusDeleted)
	default:
		return s.techSpace.SetStatuses(s.ctx, spaceID, spaceinfo.LocalStatusMissing, spaceinfo.RemoteStatusError)
	}
	s.loaded[spaceID] = sp

	// TODO: check remote status
	return s.techSpace.SetStatuses(s.ctx, spaceID, spaceinfo.LocalStatusOk, spaceinfo.RemoteStatusUnknown)
}

func (s *service) waitLoad(ctx context.Context, spaceID string) (sp Space, err error) {
	s.mu.Lock()
	localStatus := s.techSpace.GetInfo(spaceID).LocalStatus

	if localStatus == spaceinfo.LocalStatusLoading {
		// loading in progress, wait channel and retry
		waitCh := s.loading[spaceID].loadCh
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
		return s.waitLoad(ctx, spaceID)
	}

	if localStatus == spaceinfo.LocalStatusOk {
		// space is loaded just return it
		sp = s.loaded[spaceID]
		s.mu.Unlock()
		return
	}

	// return loading error
	err = s.loading[spaceID].loadErr
	s.mu.Unlock()
	return
}
