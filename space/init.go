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
	wait := make(chan struct{})
	close(wait)
	s.waiting[addr.AnytypeMarketplaceWorkspace] = controllerWaiter{
		wait: wait,
	}
	s.spaceControllers[addr.AnytypeMarketplaceWorkspace] = ctrl
	return nil
}

func (s *service) initTechSpace(ctx context.Context) (err error) {
	s.techSpace, err = s.factory.CreateAndSetTechSpace(ctx)
	return
}

func (s *service) initPersonalSpace(ctx context.Context) (err error) {
	if s.newAccount {
		return s.createPersonalSpace(ctx)
	}
	return s.loadPersonalSpace(ctx)
}
