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

func (s *service) createTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.factory.CreateAndSetTechSpace(ctx); err != nil {
		return err
	}
	close(s.techSpaceReady)
	s.notifyAccountReady()
	return
}

func (s *service) loadTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.factory.LoadAndSetTechSpace(ctx); err != nil {
		return err
	}
	close(s.techSpaceReady)
	s.notifyAccountReady()
	return
}

// notifyAccountReady is the AccountReady producer: it follows every
// close(techSpaceReady) — load, create, and the create-for-old-accounts
// fallback — so the milestone fires exactly once whichever path ran.
func (s *service) notifyAccountReady() {
	if s.recovery != nil {
		s.recovery.OnAccountReady()
	}
}
