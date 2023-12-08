package space

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) getLocalStatus(spaceId string) spaceinfo.SpaceLocalInfo {
	return s.localStatuses[spaceId]
}

func (s *service) getPersistentStatus(spaceId string) spaceinfo.SpacePersistentInfo {
	return s.persistentStatuses[spaceId]
}

func (s *service) setLocalStatus(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	if s.getLocalStatus(info.SpaceID) == info {
		return nil
	}
	if err = s.techSpace.SetLocalInfo(ctx, info); err != nil {
		return
	}
	s.localStatuses[info.SpaceID] = info
	return nil
}

func (s *service) setPersistentStatus(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error) {
	if s.getPersistentStatus(info.SpaceID) == info {
		return nil
	}
	if err = s.techSpace.SetPersistentInfo(ctx, info); err != nil {
		return
	}
	s.persistentStatuses[info.SpaceID] = info
	return nil
}

func (s *service) updateRemoteStatusLocked(ctx context.Context, spaceID string, remoteStatus spaceinfo.RemoteStatus) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.getLocalStatus(spaceID)
	status.RemoteStatus = remoteStatus
	return s.setLocalStatus(ctx, status)
}

func (s *service) updatePersistentStatusLocked(status spaceinfo.SpacePersistentInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.getPersistentStatus(status.SpaceID)
	if info.SpaceID == "" {
		s.persistentStatuses[status.SpaceID] = spaceinfo.SpacePersistentInfo{
			SpaceID:       status.SpaceID,
			AccountStatus: status.AccountStatus,
		}
		return
	}
	info.AccountStatus = status.AccountStatus
	s.persistentStatuses[status.SpaceID] = info
}

func (s *service) allIDs() (ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, status := range s.localStatuses {
		// TODO: check why we have them in localStatuses
		if status.SpaceID != addr.AnytypeMarketplaceWorkspace && status.SpaceID != s.techSpace.TechSpaceId() {
			ids = append(ids, status.SpaceID)
		}
	}
	return
}
