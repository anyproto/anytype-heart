package space

import (
	"context"

	"github.com/anyproto/anytype-heart/space/spacecore"
)

func (s *service) create(ctx context.Context, coreSpace *spacecore.AnySpace) (Space, error) {
	s.mu.Lock()
	s.createdSpaces[coreSpace.Id()] = struct{}{}
	s.mu.Unlock()

	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}

	// load
	if err := s.startLoad(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, coreSpace.Id())
}
