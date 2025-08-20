package space

import (
	"context"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) Join(ctx context.Context, id, aclHeadId string) error {
	// TODO: refactor using unidirectional model where we change/create space view and it asynchronously starts controller
	s.mu.Lock()
	waiter, exists := s.waiting[id]
	if exists {
		s.mu.Unlock()
		<-waiter.wait
		if waiter.err != nil {
			return waiter.err
		}
		s.mu.Lock()
		ctrl := s.spaceControllers[id]
		s.mu.Unlock()
		if ctrl.Mode() != mode.ModeJoining {
			info := spaceinfo.NewSpacePersistentInfo(id)
			info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusJoining)
			return ctrl.SetPersistentInfo(ctx, info)
		}
		return nil
	}
	wait := make(chan struct{})
	s.waiting[id] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreateInvitingSpace(ctx, id, aclHeadId)
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

func (s *service) InviteJoin(ctx context.Context, id, aclHeadId string) error {
	// TODO: refactor using unidirectional model where we change/create space view and it asynchronously starts controller
	s.mu.Lock()
	waiter, exists := s.waiting[id]
	if exists {
		s.mu.Unlock()
		<-waiter.wait
		if waiter.err != nil {
			return waiter.err
		}
		s.mu.Lock()
		ctrl := s.spaceControllers[id]
		s.mu.Unlock()
		if ctrl.Mode() != mode.ModeLoading {
			info := spaceinfo.NewSpacePersistentInfo(id)
			info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusActive)
			return ctrl.SetPersistentInfo(ctx, info)
		}
		return nil
	}
	wait := make(chan struct{})
	s.waiting[id] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreateActiveSpace(ctx, id, aclHeadId)
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

func (s *service) CancelLeave(ctx context.Context, id string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	return s.techSpace.SetPersistentInfo(ctx, info)
}
