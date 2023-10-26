package space

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) getStatus(spaceId string) spaceinfo.SpaceInfo {
	return s.statuses[spaceId]
}

func (s *service) setStatus(ctx context.Context, info spaceinfo.SpaceInfo) (err error) {
	if s.getStatus(info.SpaceID) == info {
		return nil
	}
	if err = s.techSpace.SetInfo(ctx, info); err != nil {
		return
	}
	s.statuses[info.SpaceID] = info
	return nil
}

func (s *service) updateRemoteStatusLocked(ctx context.Context, spaceID string, remoteStatus spaceinfo.RemoteStatus) (status spaceinfo.SpaceInfo, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status = s.getStatus(spaceID)
	status.RemoteStatus = remoteStatus
	err = s.setStatus(ctx, status)
	if err != nil {
		return status, err
	}
	return status, nil
}

func (s *service) updateSpaceViewInfo(status spaceinfo.SpaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.getStatus(status.SpaceID)
	if info.SpaceID == "" {
		s.statuses[status.SpaceID] = spaceinfo.SpaceInfo{
			SpaceID:       status.SpaceID,
			AccountStatus: status.AccountStatus,
		}
		return
	}
	info.AccountStatus = status.AccountStatus
	s.statuses[status.SpaceID] = info
}

func (s *service) allStatuses() (statuses []spaceinfo.SpaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	statuses = make([]spaceinfo.SpaceInfo, 0, len(s.statuses))
	for _, status := range s.statuses {
		// TODO: check why we have them in statuses
		if status.SpaceID != addr.AnytypeMarketplaceWorkspace && status.SpaceID != s.techSpace.TechSpaceId() {
			statuses = append(statuses, status)
		}
	}
	return
}
