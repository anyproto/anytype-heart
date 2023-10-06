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
