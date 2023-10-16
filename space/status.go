package space

import (
	"context"

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

func (s *service) allStatuses() (statuses []spaceinfo.SpaceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	statuses = make([]spaceinfo.SpaceInfo, 0, len(s.statuses))
	for _, status := range s.statuses {
		statuses = append(statuses, status)
	}
	return
}
