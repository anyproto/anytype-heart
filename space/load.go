package space

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type controllerWaiter struct {
	wait chan struct{}
	err  error
}

func (s *service) getCtrl(ctx context.Context, spaceId string) (ctrl spacecontroller.SpaceController, err error) {
	s.mu.Lock()
	if ctrl, ok := s.spaceControllers[spaceId]; ok {
		s.mu.Unlock()
		return ctrl, nil
	}
	if w, ok := s.waiting[spaceId]; ok {
		s.mu.Unlock()
		select {
		case <-w.wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		s.mu.Lock()
		err := s.waiting[spaceId].err
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		ctrl := s.spaceControllers[spaceId]
		s.mu.Unlock()
		return ctrl, nil
	}
	s.mu.Unlock()
	return nil, ErrSpaceNotExists
}

func (s *service) startStatus(ctx context.Context, info spaceinfo.SpacePersistentInfo) (ctrl spacecontroller.SpaceController, err error) {
	s.mu.Lock()
	if ctrl, ok := s.spaceControllers[info.SpaceID]; ok {
		s.mu.Unlock()
		return ctrl, nil
	}
	if w, ok := s.waiting[info.SpaceID]; ok {
		s.mu.Unlock()
		select {
		case <-w.wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		s.mu.Lock()
		err := s.waiting[info.SpaceID].err
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		ctrl := s.spaceControllers[info.SpaceID]
		s.mu.Unlock()
		return ctrl, nil
	}
	wait := make(chan struct{})
	s.waiting[info.SpaceID] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	if info.SpaceID == s.personalSpaceId {
		ctrl, err = s.factory.NewPersonalSpace(ctx, s.accountMetadataPayload)
	} else if info.EncodedKey == "" {
		ctrl, err = s.factory.NewShareableSpace(ctx, info.SpaceID, info)
	} else {
		// probably need to add factory.NewOneToOne here?
		// start streamable
		ctrl, err = s.factory.NewStreamableSpace(ctx, info.SpaceID, info, s.accountMetadataPayload)
	}
	s.mu.Lock()
	close(wait)
	if err != nil {
		s.waiting[info.SpaceID] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return nil, err
	}
	s.spaceControllers[info.SpaceID] = ctrl
	s.mu.Unlock()
	return ctrl, nil
}

func (s *service) waitLoad(ctx context.Context, ctrl spacecontroller.SpaceController) (sp clientspace.Space, err error) {
	if ld, ok := ctrl.Current().(loader.LoadWaiter); ok {
		sp, err = ld.WaitLoad(ctx)
		if err != nil {
			err = convertSpaceError(err)
		}
		return
	}
	return nil, fmt.Errorf("failed to load space, mode is %d: %w", ctrl.Mode(), ErrFailedToLoad)
}

func convertSpaceError(err error) error {
	switch {
	case errors.Is(err, spacesyncproto.ErrSpaceIsDeleted):
		return ErrSpaceDeleted
	case errors.Is(err, spacestorage.ErrSpaceStorageMissing):
		return ErrSpaceStorageMissig
	default:
		return err
	}
}
