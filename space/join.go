package space

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/space/internal/accountspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// Join is unidirectional: it writes the desired state (Joining + aclHeadId)
// into the space view and waits for the watcher-registered controller to run
// the joiner. Controllers are never constructed here.
func (s *service) Join(ctx context.Context, id, aclHeadId string) error {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusJoining)
	exists, err := s.ensureSpaceView(ctx, id, info)
	if err != nil {
		return err
	}
	ctrl, err := s.waitCtrl(ctx, id)
	if err != nil {
		return err
	}
	// keep the space loaded after the join completes, also in lazy mode
	ctrl.Demand()
	if exists && ctrl.Mode() != mode.ModeJoining {
		if err := ctrl.SetPersistentInfo(ctx, info); err != nil {
			return err
		}
	}
	return s.waitIntentMode(ctx, ctrl, mode.ModeJoining)
}

// InviteJoin activates a space joined through a no-approval invite: write the
// Active status into the space view and wait for the controller to start
// loading.
func (s *service) InviteJoin(ctx context.Context, id, aclHeadId string) error {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusActive)
	exists, err := s.ensureSpaceView(ctx, id, info)
	if err != nil {
		return err
	}
	ctrl, err := s.waitCtrl(ctx, id)
	if err != nil {
		return err
	}
	ctrl.Demand()
	if exists {
		if err := ctrl.SetPersistentInfo(ctx, info); err != nil {
			return err
		}
	}
	return s.waitIntentMode(ctx, ctrl, mode.ModeLoading)
}

// ensureSpaceView creates the space view with the given info if it does not
// exist. Returns whether the view already existed; a creation race
// (ErrSpaceViewExists) counts as existing so the caller still writes its
// intent into the view.
func (s *service) ensureSpaceView(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo) (exists bool, err error) {
	exists, err = s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check space view: %w", err)
	}
	if exists {
		return true, nil
	}
	if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, nil); err != nil {
		if errors.Is(err, techspace.ErrSpaceViewExists) {
			return true, nil
		}
		return false, fmt.Errorf("create space view: %w", err)
	}
	return false, nil
}

// waitIntentMode waits until the controller runs the process the intent asked
// for, surfacing real start errors. ErrModeUnreachable means the status moved
// on (e.g. the join was already accepted), which is success for the intent.
func (s *service) waitIntentMode(ctx context.Context, ctrl spacecontroller.SpaceController, m mode.Mode) error {
	err := ctrl.WaitMode(ctx, m)
	switch {
	case err == nil, errors.Is(err, accountspace.ErrModeUnreachable):
		return nil
	case errors.Is(err, accountspace.ErrCtrlClosed):
		return ErrSpaceIsClosing
	}
	return err
}

func (s *service) CancelLeave(ctx context.Context, id string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	return s.techSpace.SetPersistentInfo(ctx, info)
}
