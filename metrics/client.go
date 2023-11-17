package metrics

import (
	"context"
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

func (c *client) startAggregating(info appInfoProvider) {
	c.lock.RLock()
	ctx := c.ctx
	c.lock.RUnlock()
	go func() {
		ticker := time.NewTicker(sendInterval)
		for {
			select {
			case <-ticker.C:
				c.recordAggregatedData(info)
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
				c.recordAggregatedData(info)
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

func (c *client) startSendingBatchMessages(info appInfoProvider) {
	c.lock.RLock()
	ctx := c.ctx
	c.lock.RUnlock()
	attempt := 0
	for {
		c.lock.Lock()
		b := c.batcher
		c.lock.Unlock()
		if b == nil {
			return
		}
		ctx = mb.CtxWithTimeLimit(ctx, time.Minute)
		msgs, err := b.NewCond().WithMin(10).WithMax(100).Wait(ctx)

		if err == mb.ErrClosed {
			close(c.closeChannel)
			return
		}

		timeout := time.Second * 2
		err = c.sendNextBatch(info, b, msgs)
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

func (c *client) sendNextBatch(info appInfoProvider, b *mb.MB[amplitude.Event], msgs []amplitude.Event) (err error) {
	if len(msgs) == 0 {
		return
	}

	var events []amplitude.Event
	for _, ev := range msgs {
		events = append(events, ev)
	}
	err = c.amplitude.Events(events)
	if err != nil {
		clientMetricsLog.
			With("unsent messages", len(msgs)+c.batcher.Len()).
			Error("failed to send messages")
		if b != nil {
			b.Add(c.ctx, msgs...)
		}
	} else {
		clientMetricsLog.
			With("user-id", info.getUserId()).
			With("messages sent", len(msgs)).
			Debug("events sent to amplitude")
	}
	return
}

func (c *client) recordAggregatedData(info appInfoProvider) {
	var events []SamplableEvent
	c.lock.RLock()
	for k, ev := range c.aggregatableMap {
		events = append(events, ev)
		delete(c.aggregatableMap, k)
	}
	c.lock.RUnlock()
	for _, ev := range events {
		c.send(info, ev)
	}
}

func (c *client) sendSampled(ev SamplableEvent) {
	select {
	case c.aggregatableChan <- ev:
	default:
	}
}

func (c *client) send(info appInfoProvider, e Event) {
	ev := e.get()
	if ev == nil {
		return
	}
	c.lock.RLock()
	ampEvent := amplitude.Event{
		UserID:          info.getUserId(),
		Platform:        info.getPlatform(),
		DeviceID:        info.getDeviceId(),
		EventType:       ev.eventType,
		EventProperties: ev.eventData,
		AppVersion:      info.getAppVersion(),
		StartVersion:    info.getStartVersion(),
		Time:            time.Now().Unix() * 1000,
	}

	b := c.batcher
	c.lock.RUnlock()
	if b == nil {
		return
	}
	b.Add(c.ctx, ampEvent)
	clientMetricsLog.
		With("event-type", ev.eventType).
		With("event-data", ev.eventData).
		With("user-id", info.getUserId()).
		With("platform", info.getPlatform()).
		With("device-id", info.getDeviceId()).
		Debug("event added to batcher")
}
