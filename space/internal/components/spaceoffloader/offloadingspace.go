package spaceoffloader

import (
	"context"
	"time"

	"go.uber.org/zap"
)

var loadingRetryTimeout = time.Second * 20

type offloader interface {
	offload(ctx context.Context, id string) (err error)
	onOffload(id string, err error)
}

type offloadingSpace struct {
	loadErr error
	loadCh  chan struct{}
	id      string

	ol offloader

	retryTimeout time.Duration
}

func newOffloadingSpace(ctx context.Context, id string, ol offloader) *offloadingSpace {
	os := &offloadingSpace{
		ol:           ol,
		id:           id,
		loadCh:       make(chan struct{}),
		retryTimeout: loadingRetryTimeout,
	}
	go os.offloadRetry(ctx)
	return os
}

func (s *offloadingSpace) offloadRetry(ctx context.Context) {
	defer func() {
		s.ol.onOffload(s.id, s.loadErr)
		close(s.loadCh)
	}()
	if s.offload(ctx) {
		return
	}
	ticker := time.NewTicker(s.retryTimeout)
	for {
		select {
		case <-ctx.Done():
			s.loadErr = ctx.Err()
			return
		case <-ticker.C:
			if s.offload(ctx) {
				return
			}
		}
	}
}

func (s *offloadingSpace) offload(ctx context.Context) (ok bool) {
	err := s.ol.offload(ctx, s.id)
	if err != nil {
		log.WarnCtx(ctx, "space offload error", zap.Error(err), zap.String("spaceId", s.id))
		return false
	}
	return true
}
