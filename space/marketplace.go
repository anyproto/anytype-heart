package space

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

func (s *service) initMarketplaceSpace(ctx context.Context) error {
	ctrl, err := s.factory.CreateMarketplaceSpace(ctx)
	if err != nil {
		return err
	}
	err = ctrl.Start(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spaceControllers[addr.AnytypeMarketplaceWorkspace] = ctrl
	return nil
}
