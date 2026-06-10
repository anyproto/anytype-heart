package space

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// Join is unidirectional: it writes the desired state (Joining + aclHeadId)
// into the space view and waits for the watcher-registered controller to pick
// it up. Controllers are never constructed here.
func (s *service) Join(ctx context.Context, id, aclHeadId string) error {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusJoining)
	exists, err := s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return fmt.Errorf("check space view: %w", err)
	}
	if !exists {
		if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, nil); err != nil &&
			!errors.Is(err, techspace.ErrSpaceViewExists) {
			return fmt.Errorf("create space view: %w", err)
		}
	}
	ctrl, err := s.waitCtrl(ctx, id)
	if err != nil {
		return err
	}
	// keep the space loaded after the join completes, also in lazy mode
	ctrl.Demand()
	if exists && ctrl.Mode() != mode.ModeJoining {
		return ctrl.SetPersistentInfo(ctx, info)
	}
	return nil
}

// InviteJoin activates a space joined through a no-approval invite: write the
// Active status into the space view and let the controller converge to
// loading.
func (s *service) InviteJoin(ctx context.Context, id, aclHeadId string) error {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusActive)
	exists, err := s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return fmt.Errorf("check space view: %w", err)
	}
	if !exists {
		if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, nil); err != nil &&
			!errors.Is(err, techspace.ErrSpaceViewExists) {
			return fmt.Errorf("create space view: %w", err)
		}
	}
	ctrl, err := s.waitCtrl(ctx, id)
	if err != nil {
		return err
	}
	ctrl.Demand()
	if exists {
		return ctrl.SetPersistentInfo(ctx, info)
	}
	return nil
}

func (s *service) CancelLeave(ctx context.Context, id string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	return s.techSpace.SetPersistentInfo(ctx, info)
}
