package deletioncontroller

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type updateLoop struct {
	updateCtx    context.Context
	updateCancel context.CancelFunc
	updateChan   chan struct{}
	updateFunc   func(ctx context.Context) error
	timeout      time.Duration
	loopInterval time.Duration
	loopDone     chan struct{}
}

func newUpdateLoop(updateFunc func(ctx context.Context) error, loopInterval, timeout time.Duration) *updateLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &updateLoop{
		updateCtx:    ctx,
		updateCancel: cancel,
		updateChan:   make(chan struct{}, 1),
		timeout:      timeout,
		loopInterval: loopInterval,
		updateFunc:   updateFunc,
		loopDone:     make(chan struct{}),
	}
}

func (ul *updateLoop) Run() {
	go ul.loop()
}

func (ul *updateLoop) loop() {
	defer close(ul.loopDone)
	update := func() {
		ctx, cancel := context.WithTimeout(ul.updateCtx, ul.timeout)
		defer cancel()
		err := ul.updateFunc(ctx)
		if err != nil {
			log.Warn("update loop error", zap.Error(err))
		}
	}
	update()
	ticker := time.NewTicker(ul.loopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ul.updateCtx.Done():
			return
		case <-ul.updateChan:
			update()
			ticker.Reset(ul.loopInterval)
		case <-ticker.C:
			update()
		}
	}
}

func (ul *updateLoop) notify() {
	select {
	case ul.updateChan <- struct{}{}:
	default:
	}
}

func (ul *updateLoop) Close() {
	ul.updateCancel()
	<-ul.loopDone
}
