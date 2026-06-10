package space

import (
	"context"
)

// initMarketplaceSpace sets up the virtual marketplace space. It lives
// outside the controller registry (like tech space): Get special-cases it.
func (s *service) initMarketplaceSpace(ctx context.Context) error {
	ctrl, err := s.factory.CreateMarketplaceSpace(ctx)
	if err != nil {
		return err
	}
	err = ctrl.Start(ctx)
	if err != nil {
		return err
	}
	s.marketplaceCtrl = ctrl
	return nil
}

func (s *service) createTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.factory.CreateAndSetTechSpace(ctx); err != nil {
		return err
	}
	close(s.techSpaceReady)
	return
}

func (s *service) loadTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.factory.LoadAndSetTechSpace(ctx); err != nil {
		return err
	}
	close(s.techSpaceReady)
	return
}
