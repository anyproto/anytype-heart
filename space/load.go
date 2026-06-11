package space

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/accountspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// registerCtrl inserts a controller into the registry and wakes everyone
// blocked in waitCtrl.
func (s *service) registerCtrl(id string, ctrl spacecontroller.SpaceController) {
	s.mu.Lock()
	s.spaceControllers[id] = ctrl
	s.registryChangedLocked()
	s.mu.Unlock()
}

// registryChangedLocked broadcasts a registry change; must be called with
// s.mu held.
func (s *service) registryChangedLocked() {
	close(s.regChanged)
	s.regChanged = make(chan struct{})
}

// waitCtrl blocks until a controller for the space is registered (the watcher
// registers one for every space view). It returns the last registration error
// for the id, if any; the error is cleared on the next registration attempt,
// so a transient failure does not poison the id for the session.
func (s *service) waitCtrl(ctx context.Context, id string) (spacecontroller.SpaceController, error) {
	for {
		s.mu.Lock()
		if ctrl, ok := s.spaceControllers[id]; ok {
			s.mu.Unlock()
			return ctrl, nil
		}
		if err, ok := s.regErr[id]; ok {
			s.mu.Unlock()
			return nil, err
		}
		ch := s.regChanged
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.ctx.Done():
			return nil, ErrSpaceIsClosing
		case <-ch:
		}
	}
}

// getCtrl returns the controller for the space without blocking on
// registration: a plain map lookup (plus the last registration error). Wait
// is the blocking variant.
func (s *service) getCtrl(spaceId string) (ctrl spacecontroller.SpaceController, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctrl, ok := s.spaceControllers[spaceId]; ok {
		return ctrl, nil
	}
	if err, ok := s.regErr[spaceId]; ok {
		return nil, err
	}
	return nil, ErrSpaceNotExists
}

// startStatus registers a controller for the space (idempotently and
// single-flight per id: a concurrent call for the same id waits for the
// first). Whether the space also starts loading is the lazy-mode demand
// decision: eager mode and the preferred space demand immediately; other
// spaces stay dormant until released or fetched via Get/Wait. Status-driven
// work (offloading, joining) proceeds regardless of demand.
func (s *service) startStatus(ctx context.Context, info spaceinfo.SpacePersistentInfo) (ctrl spacecontroller.SpaceController, err error) {
	for {
		s.mu.Lock()
		if ctrl, ok := s.spaceControllers[info.SpaceID]; ok {
			s.mu.Unlock()
			return ctrl, nil
		}
		building, inFlight := s.constructing[info.SpaceID]
		if !inFlight {
			break // still holding s.mu
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.ctx.Done():
			return nil, ErrSpaceIsClosing
		case <-building:
		}
		// re-check: either the controller is registered now or the attempt
		// failed and we become the next attempt
	}
	building := make(chan struct{})
	s.constructing[info.SpaceID] = building
	// a new attempt supersedes a previous failure
	delete(s.regErr, info.SpaceID)
	demandNow := !s.lazyMode || s.released || info.SpaceID == s.preferredSpaceId
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.constructing, info.SpaceID)
		close(building)
		s.mu.Unlock()
	}()
	if info.SpaceID == s.personalSpaceId {
		ctrl, err = s.factory.NewPersonalSpace(ctx, s.accountMetadataPayload)
	} else if info.EncodedKey == "" {
		ctrl, err = s.factory.NewShareableSpace(ctx, info.SpaceID, info)
	} else {
		ctrl, err = s.factory.NewStreamableSpace(ctx, info.SpaceID, info, s.accountMetadataPayload)
	}
	if err == nil && demandNow {
		if err = ctrl.Start(ctx); err != nil {
			if closeErr := ctrl.Close(ctx); closeErr != nil {
				log.Warn("close controller after failed start", zap.Error(closeErr))
			}
		}
	}
	s.mu.Lock()
	if err == nil && s.isClosing.Load() {
		// Close() may have snapshotted the registry already; do not insert a
		// controller it will never close
		err = ErrSpaceIsClosing
		s.mu.Unlock()
		if closeErr := ctrl.Close(ctx); closeErr != nil {
			log.Warn("close controller registered during shutdown", zap.Error(closeErr))
		}
		s.mu.Lock()
	}
	if err != nil {
		s.regErr[info.SpaceID] = err
		s.registryChangedLocked()
		s.mu.Unlock()
		return nil, err
	}
	s.spaceControllers[info.SpaceID] = ctrl
	// the release may have happened between the demand decision and the
	// insertion above; in that case the snapshot in releaseAll missed this
	// controller, so demand it here (idempotent)
	demandLate := s.released && !demandNow
	s.registryChangedLocked()
	s.mu.Unlock()
	if demandLate {
		ctrl.Demand()
	}
	return ctrl, nil
}

func (s *service) waitLoad(ctx context.Context, ctrl spacecontroller.SpaceController) (sp clientspace.Space, err error) {
	sp, err = ctrl.WaitLoad(ctx)
	if err != nil {
		switch {
		case errors.Is(err, accountspace.ErrModeUnreachable):
			return nil, fmt.Errorf("failed to load space, mode is %d: %w", ctrl.Mode(), ErrFailedToLoad)
		case errors.Is(err, accountspace.ErrCtrlClosed):
			return nil, ErrSpaceIsClosing
		}
		return nil, convertSpaceError(err)
	}
	return sp, nil
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
