package space

import (
	"context"

	"github.com/anyproto/any-sync/util/crypto"
)

func (s *service) AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) (err error) {
	s.mu.Lock()
	waiter, exists := s.waiting[id]
	if exists {
		s.mu.Unlock()
		<-waiter.wait
		return waiter.err
	}
	wait := make(chan struct{})
	s.waiting[id] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreateStreamableSpace(ctx, guestKey, id)
	if err != nil {
		s.mu.Lock()
		close(wait)
		s.waiting[id] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	close(wait)
	s.spaceControllers[ctrl.SpaceId()] = ctrl
	s.mu.Unlock()
	return nil
}
