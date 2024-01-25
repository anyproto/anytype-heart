package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/metrics/amplitude"
)

var (
	sendingTimeLimit     = time.Minute
	sendingQueueLimitMin = 10
	sendingQueueLimitMax = 100
)

type client struct {
	lock             sync.RWMutex
	telemetry        amplitude.Service
	aggregatableMap  map[string]SamplableEvent
	aggregatableChan chan SamplableEvent
	ctx              context.Context
	cancel           context.CancelFunc
	batcher          *mb.MB[amplitude.Event]
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
				clientBatcher := c.batcher
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

func (c *client) startSendingBatchMessages(info amplitude.AppInfoProvider) {
	ctx := c.ctx
	attempt := 0
	for {
		clientBatcher := c.batcher
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
	c.lock.Lock()
	if c.batcher == nil {
		c.lock.Unlock()
		return
	}
	c.lock.Unlock()
	c.cancel()

	c.lock.Lock()
	defer c.lock.Unlock()
	c.batcher = nil
}

func (c *client) sendNextBatch(info amplitude.AppInfoProvider, batcher *mb.MB[amplitude.Event], msgs []amplitude.Event) (err error) {
	clientBatcher := c.batcher
	if clientBatcher == nil || len(msgs) == 0 {
		return nil
	}

	err = c.telemetry.SendEvents(msgs, info)
	if err != nil {
		clientMetricsLog.
			With("unsent messages", len(msgs)+clientBatcher.Len()).
			Error("failed to send messages")
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
	c.lock.RLock()
	toSend := c.aggregatableMap
	c.aggregatableMap = make(map[string]SamplableEvent)
	c.lock.RUnlock()
	// итерейтим сразу старую мапу и скармливаем ГЦ
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

func (c *client) send(e amplitude.Event) {
	if e == nil {
		return
	}
	e.SetTimestamp()
	clientBatcher := c.batcher
	if clientBatcher == nil {
		return
	}
	_ = clientBatcher.TryAdd(e) //nolint:errcheck
}
