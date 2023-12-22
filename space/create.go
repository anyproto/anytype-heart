package space

import (
	"context"

	"github.com/anyproto/anytype-heart/space/spacecore"
)

func (s *service) create(ctx context.Context, coreSpace *spacecore.AnySpace) (Space, error) {
	err := s.storageService.MarkSpaceCreated(coreSpace.Id())
	if err != nil {
		return nil, err
	}
	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true); err != nil {
		return nil, err
	}

	// load
	if err := s.startLoad(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, coreSpace.Id())
}
