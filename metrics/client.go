package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/metrics/amplitude"
)

type client struct {
	lock             sync.RWMutex
	amplitude        *amplitude.Client
	aggregatableMap  map[string]SamplableEvent
	aggregatableChan chan SamplableEvent
	ctx              context.Context
	cancel           context.CancelFunc
	batcher          *mb.MB[amplitude.Event]
	closeChannel     chan struct{}
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
				err := c.batcher.Close()
				if err != nil {
					clientMetricsLog.Errorf("failed to close batcher")
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
		batcher := c.batcher
		if batcher == nil {
			return
		}
		ctx = mb.CtxWithTimeLimit(ctx, time.Minute)
		msgs, err := batcher.NewCond().WithMin(10).WithMax(100).Wait(ctx)

		if errors.Is(err, mb.ErrClosed) {
			close(c.closeChannel)
			return
		}

		timeout := time.Second * 2
		err = c.sendNextBatch(info, batcher, msgs)
		if err != nil {
			timeout = time.Second * 5 * time.Duration(attempt+1)
			if timeout > maxTimeout {
				timeout = maxTimeout
			}
			attempt++
		} else {
			attempt = 0
		}
		select {
		// this is needed for early continue
		case <-ctx.Done():
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

	<-c.closeChannel

	c.lock.Lock()
	defer c.lock.Unlock()
	c.batcher = nil
}

func (c *client) sendNextBatch(info amplitude.AppInfoProvider, batcher *mb.MB[amplitude.Event], msgs []amplitude.Event) (err error) {
	if len(msgs) == 0 {
		return
	}

	err = c.amplitude.SendEvents(msgs, info)
	if err != nil {
		clientMetricsLog.
			With("unsent messages", len(msgs)+c.batcher.Len()).
			Error("failed to send messages")
		if batcher != nil {
			_ = batcher.TryAdd(msgs...) //nolint:errcheck
		}
	} else {
		clientMetricsLog.
			With("user-id", info.GetUserId()).
			With("messages sent", len(msgs)).
			Debug("events sent to amplitude")
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
	batcher := c.batcher
	if batcher == nil {
		return
	}
	_ = batcher.TryAdd(e) //nolint:errcheck
}
