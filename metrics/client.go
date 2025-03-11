package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/metrics/anymetry"
)

var (
	sendingTimeLimit     = time.Minute
	sendingQueueLimitMin = 10
	sendingQueueLimitMax = 100
)

type client struct {
	lock             sync.RWMutex
	batcherLock      sync.RWMutex
	telemetry        anymetry.Service
	aggregatableMap  map[string]SamplableEvent
	aggregatableChan chan SamplableEvent
	ctx              context.Context
	cancel           context.CancelFunc
	batcher          *mb.MB[anymetry.Event]
}

func (c *client) startAggregating() {
	ctx := c.ctx
	go func() {
		ticker := time.NewTicker(sendInterval)
		for {
			select {
			case <-ticker.C:
				c.recordAggregatedData()
			case ev := <-c.aggregatableChan:
				c.lock.Lock()

				other, ok := c.aggregatableMap[ev.Key()]
				var newEv SamplableEvent
				if !ok {
					newEv = ev
				} else {
					newEv = ev.Aggregate(other)
				}
				c.aggregatableMap[ev.Key()] = newEv

				c.lock.Unlock()
			case <-ctx.Done():
				c.recordAggregatedData()
				// we close here so that we are sure that we don't lose the aggregated data
				clientBatcher := c.getBatcher()
				if clientBatcher != nil {
					err := clientBatcher.Close()
					if err != nil {
						clientMetricsLog.Errorf("failed to close batcher")
					}
				}
				return
			}
		}
	}()
}

func (c *client) startSendingBatchMessages(info anymetry.AppInfoProvider) {
	ctx := c.ctx
	attempt := 0
	for {
		clientBatcher := c.getBatcher()
		if clientBatcher == nil {
			return
		}
		ctx = mb.CtxWithTimeLimit(ctx, sendingTimeLimit)
		msgs, err := clientBatcher.NewCond().
			WithMin(sendingQueueLimitMin).
			WithMax(sendingQueueLimitMax).
			Wait(ctx)

		if errors.Is(err, mb.ErrClosed) {
			return
		}

		err = c.sendNextBatch(info, clientBatcher, msgs)
		timeout := time.Second * 2
		if err == nil {
			attempt = 0
		} else {
			timeout = time.Second * 5 * time.Duration(attempt+1)
			if timeout > maxTimeout {
				timeout = maxTimeout
			}
			attempt++
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(timeout):
		}
	}
}

func (c *client) Close() {
	if c.getBatcher() == nil {
		return
	}

	c.cancel()

	c.setBatcher(nil)
}

func (c *client) getBatcher() *mb.MB[anymetry.Event] {
	defer c.batcherLock.RUnlock()
	c.batcherLock.RLock()
	return c.batcher
}

func (c *client) setBatcher(batcher *mb.MB[anymetry.Event]) {
	defer c.batcherLock.Unlock()
	c.batcherLock.Lock()
	c.batcher = batcher
}

func (c *client) sendNextBatch(info anymetry.AppInfoProvider, batcher *mb.MB[anymetry.Event], msgs []anymetry.Event) (err error) {
	clientBatcher := c.getBatcher()
	if clientBatcher == nil || len(msgs) == 0 {
		return nil
	}

	err = c.telemetry.SendEvents(msgs, info)
	if err != nil {
		if batcher != nil {
			_ = batcher.TryAdd(msgs...) //nolint:errcheck
		}
	} else {
		clientMetricsLog.
			With("user-id", info.GetUserId()).
			With("messages sent", len(msgs)).
			Debug("events sent to telemetry")
	}
	return
}

func (c *client) recordAggregatedData() {
	c.lock.Lock()
	toSend := c.aggregatableMap
	c.aggregatableMap = make(map[string]SamplableEvent)
	c.lock.Unlock()
	for _, ev := range toSend {
		c.send(ev)
	}
}

func (c *client) sendSampled(ev SamplableEvent) {
	select {
	case c.aggregatableChan <- ev:
	default:
	}
}

func (c *client) send(e anymetry.Event) {
	if e == nil {
		return
	}
	e.SetTimestamp()
	clientBatcher := c.getBatcher()
	if clientBatcher == nil {
		return
	}
	_ = clientBatcher.TryAdd(e) //nolint:errcheck
}
