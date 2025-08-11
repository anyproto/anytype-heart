package space

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) checkControllerExists(spaceId string) bool {
	s.mu.Lock()
	_, ctrlOk := s.spaceControllers[spaceId]
	_, waitingOk := s.waiting[spaceId]
	s.mu.Unlock()
	return ctrlOk || waitingOk
}

type waiterService interface {
	TechSpace() *clientspace.TechSpace
	Get(ctx context.Context, spaceId string) (clientspace.Space, error)
	checkControllerExists(spaceId string) bool
}

type spaceWaiter struct {
	svc        waiterService
	svcCtx     context.Context
	retryDelay time.Duration
}

func newSpaceWaiter(svc waiterService, svcCtx context.Context, retryDelay time.Duration) *spaceWaiter {
	return &spaceWaiter{svc: svc, svcCtx: svcCtx, retryDelay: retryDelay}
}

func (w *spaceWaiter) waitSpace(ctx context.Context, spaceId string) (sp clientspace.Space, err error) {
	techSpace := w.svc.TechSpace()
	if spaceId == techSpace.TechSpaceId() {
		return techSpace, nil
	}
	// wait until we start the space view loading process
	if err := techSpace.WaitViews(); err != nil {
		return nil, fmt.Errorf("wait views: %w", err)
	}
	// if there is no such space view then there is no space
	if spaceId != addr.AnytypeMarketplaceWorkspace {
		exists, err := techSpace.SpaceViewExists(ctx, spaceId)
		if err != nil {
			// func returns error only on derive
			return nil, fmt.Errorf("space view derive error: %w", err)
		}
		if !exists {
			return nil, ErrSpaceNotExists
		}
	}
	// we should wait a bit until the controller is created
	for !w.svc.checkControllerExists(spaceId) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-w.svcCtx.Done():
			return nil, w.svcCtx.Err()
		case <-time.After(w.retryDelay):
			break
		}
	}
	return w.svc.Get(ctx, spaceId)
}
