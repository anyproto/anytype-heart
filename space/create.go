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
	ctrl, err := s.factory.CreatePersonalSpace(ctx)
	if err != nil {
		return
	}
	s.personalSpaceId = ctrl.SpaceId()
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

func (s *service) create(ctx context.Context) (clientspace.Space, error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey, s.metadataPayload)
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
	return ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
}
