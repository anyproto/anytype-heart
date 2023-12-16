package space

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) startStatus(ctx context.Context, spaceID string, status spaceinfo.AccountStatus) (ctrl spacecontroller.SpaceController, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctrl, ok := s.spaceControllers[spaceID]; ok {
		return ctrl, nil
	}
	ctrl, err = s.factory.NewShareableSpace(ctx, spaceID, status)
	if err != nil {
		return nil, err
	}
	s.spaceControllers[spaceID] = ctrl
	return
}

func (s *service) waitLoad(ctx context.Context, ctrl spacecontroller.SpaceController) (sp clientspace.Space, err error) {
	if ld, ok := ctrl.Current().(loader.LoadWaiter); ok {
		return ld.WaitLoad(ctx)
	}
	return nil, fmt.Errorf("failed to load space")
}
