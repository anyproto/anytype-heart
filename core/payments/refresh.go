package payments

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func newRefreshController(parent context.Context, fetch func(ctx context.Context, forceFetch bool) (bool, error), interval time.Duration) *refreshController {
	if interval <= 0 {
		interval = time.Minute
	}
	ctx, cancel := context.WithCancel(parent)
	return &refreshController{
		ctx:           ctx,
		cancel:        cancel,
		fetch:         fetch,
		interval:      interval,
		forceInterval: forceRefreshInterval,
		forceCh:       make(chan time.Duration),
		closeCh:       make(chan struct{}),
		now:           time.Now,
	}
}

func (rc *refreshController) Start() {
	if rc == nil {
		return
	}
	go rc.loop()
}

func (rc *refreshController) Stop() {
	if rc == nil {
		return
	}
	rc.cancel()
	<-rc.closeCh
}

func (rc *refreshController) Force(duration time.Duration) {
	if rc == nil {
		return
	}
	if duration <= 0 {
		duration = rc.interval
	}
	select {
	case <-rc.ctx.Done():
		return
	case rc.forceCh <- duration:
	}
}

func (rc *refreshController) loop() {
	defer close(rc.closeCh)

	timer := time.NewTimer(0)
	defer timer.Stop()

	forceActive := false
	forceDeadline := time.Time{}

	for {
		var timerC <-chan time.Time = timer.C
		select {
		case <-rc.ctx.Done():
			return
		case extend := <-rc.forceCh:
			now := rc.now()
			if !forceActive {
				forceActive = true
				forceDeadline = now.Add(extend)
				resetTimer(timer, 0)
			} else {
				newDeadline := now.Add(extend)
				if newDeadline.After(forceDeadline) {
					forceDeadline = newDeadline
				}
				resetTimer(timer, 0)
			}
		case <-timerC:
			if rc.fetch == nil {
				resetTimer(timer, rc.interval)
				continue
			}
			changed, err := rc.fetch(rc.ctx, forceActive)
			if err != nil {
				log.Warn("membership refresh: fetch failed", zap.Error(err), zap.Bool("force", forceActive))
			}
			if forceActive {
				switch {
				case changed:
					forceActive = false
				case rc.now().After(forceDeadline):
					log.Warn("membership refresh: forced refresh timed out before change")
					forceActive = false
				}
			}
			if forceActive {
				resetTimer(timer, rc.forceInterval)
			} else {
				resetTimer(timer, rc.interval)
			}
		}
	}
}

func resetTimer(timer *time.Timer, interval time.Duration) {
	if interval < 0 {
		interval = 0
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
}
