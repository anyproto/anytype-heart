package space

import (
	"context"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
)

func (s *service) createPersonalSpace(ctx context.Context) (err error) {
	s.mu.Lock()
	wait := make(chan struct{})
	s.waiting[s.personalSpaceId] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreatePersonalSpace(ctx, s.accountMetadataPayload)
	if err != nil {
		return
	}
	_, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	close(wait)
	if err != nil {
		s.waiting[s.personalSpaceId] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		return
	}
	s.spaceControllers[s.personalSpaceId] = ctrl
	return
}

func (s *service) create(ctx context.Context) (sp clientspace.Space, err error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey, s.AccountMetadataPayload())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	wait := make(chan struct{})
	s.waiting[coreSpace.Id()] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreateShareableSpace(ctx, coreSpace.Id())
	if err != nil {
		s.mu.Lock()
		close(wait)
		s.waiting[coreSpace.Id()] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return nil, err
	}
	sp, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	s.mu.Lock()
	close(wait)
	if err != nil {
		s.waiting[coreSpace.Id()] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return nil, err
	}
	s.spaceControllers[ctrl.SpaceId()] = ctrl
	s.mu.Unlock()
	return
}
