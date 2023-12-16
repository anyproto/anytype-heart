package space

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
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

func (s *service) initTechSpace() (err error) {
	s.techSpace, err = s.factory.CreateAndSetTechSpace(s.ctx)
	return
}

func (s *service) initPersonalSpace() (err error) {
	if s.newAccount {
		return s.createPersonalSpace(s.ctx)
	}
	return s.loadPersonalSpace(s.ctx)
}

func (s *service) createPersonalSpace(ctx context.Context) (err error) {
	ctrl, err := s.factory.CreatePersonalSpace(ctx)
	if err != nil {
		return
	}
	s.personalSpaceID = ctrl.SpaceId()
	_, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spaceControllers[s.personalSpaceID] = ctrl
	return
}

func (s *service) loadPersonalSpace(ctx context.Context) (err error) {
	ctrl, err := s.factory.NewPersonalSpace(ctx)
	if err != nil {
		return
	}
	s.personalSpaceID = ctrl.SpaceId()
	_, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spaceControllers[s.personalSpaceID] = ctrl
	return
}
