package metrics

import (
	"context"
	"github.com/cheggaaa/mb"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/msingleton/amplitude-go"
)

var (
	SharedClient     = NewClient()
	clientMetricsLog = logging.Logger("client-metrics")
	sendInterval     = 10.0 * time.Second
	maxTimeout       = 30 * time.Second
	bufferSize       = 500
)

type EventRepresentable interface {
	// ToEvent returns nil in case event should be ignored
	ToEvent() *Event
}

type EventAggregatable interface {
	EventRepresentable

	Key() string
	Aggregate(other EventAggregatable) EventAggregatable
}

type Event struct {
	EventType string
	EventData map[string]interface{}
}

type Client interface {
	InitWithKey(k string)

	SetAppVersion(v string)
	SetDeviceId(t string)
	SetPlatform(p string)
	SetUserId(id string)

	RecordEvent(ev EventRepresentable)
	AggregateEvent(ev EventAggregatable)

	Run()
	Close()
}

type client struct {
	lock             sync.RWMutex
	appVersion       string
	userId           string
	deviceId         string
	platform         string
	amplitude        *amplitude.Client
	aggregatableMap  map[string]EventAggregatable
	aggregatableChan chan EventAggregatable
	ctx              context.Context
	cancel           context.CancelFunc
	batcher          *mb.MB
	closeChannel     chan struct{}
}

func NewClient() Client {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	return &client{
		aggregatableMap:  make(map[string]EventAggregatable),
		aggregatableChan: make(chan EventAggregatable, bufferSize),
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (c *client) InitWithKey(k string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.amplitude = amplitude.New(k)
}

func (c *client) SetDeviceId(t string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deviceId = t
}

func (c *client) SetPlatform(p string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.platform = p
}

func (c *client) SetUserId(id string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.userId = id
}

func (c *client) SetAppVersion(version string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.appVersion = version
}

func (c *client) recordAggregatedData() {
	var events []EventAggregatable
	c.lock.RLock()
	for k, ev := range c.aggregatableMap {
		events = append(events, ev)
		delete(c.aggregatableMap, k)
	}
	c.lock.RUnlock()
	for _, ev := range events {
		c.RecordEvent(ev)
	}
}

func (c *client) Run() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.batcher = mb.New(0)
	c.closeChannel = make(chan struct{})
	go c.startAggregating()
	go c.startSendingBatchMessages()
}

func (c *client) startAggregating() {
	c.lock.RLock()
	ctx := c.ctx
	c.lock.RUnlock()
	go func() {
		ticker := time.NewTicker(sendInterval)
		for {
			select {
			case <-ticker.C:
				c.recordAggregatedData()
			case ev := <-c.aggregatableChan:
				c.lock.Lock()

				other, ok := c.aggregatableMap[ev.Key()]
				var newEv EventAggregatable
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

func (c *client) startSendingBatchMessages() {
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
		msgs := b.WaitMinMax(10, 100)
		// if batcher is closed
		if len(msgs) == 0 {
			c.sendNextBatch(nil, b.GetAll())
			close(c.closeChannel)
			return
		}
		timeout := time.Second * 2
		err := c.sendNextBatch(b, msgs)
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

func (c *client) sendNextBatch(b *mb.MB, msgs []interface{}) (err error) {
	if len(msgs) == 0 {
		return
	}

	var events []amplitude.Event
	for _, ev := range msgs {
		events = append(events, ev.(amplitude.Event))
	}
	err = c.amplitude.Events(events)
	if err != nil {
		clientMetricsLog.
			With("unsent messages", len(msgs)+c.batcher.Len()).
			Error("failed to send messages")
		if b != nil {
			b.Add(msgs...)
		}
	} else {
		clientMetricsLog.
			With("user-id", c.userId).
			With("messages sent", len(msgs)).
			Debug("events sent to amplitude")
	}
	return
}

func (c *client) RecordEvent(ev EventRepresentable) {
	if c.amplitude == nil || ev == nil {
		return
	}
	e := ev.ToEvent()
	if e == nil {
		return
	}
	c.lock.RLock()
	ampEvent := amplitude.Event{
		UserId:          c.userId,
		Platform:        c.platform,
		DeviceId:        c.deviceId,
		EventType:       e.EventType,
		EventProperties: e.EventData,
		AppVersion:      c.appVersion,
		Time:            time.Now().Unix() * 1000,
	}
	b := c.batcher
	c.lock.RUnlock()
	if b == nil {
		return
	}
	b.Add(ampEvent)
	clientMetricsLog.
		With("event-type", e.EventType).
		With("event-data", e.EventData).
		With("user-id", c.userId).
		With("platform", c.platform).
		With("device-id", c.deviceId).
		Debug("event added to batcher")
}

func (c *client) AggregateEvent(ev EventAggregatable) {
	select {
	case c.aggregatableChan <- ev:
	default:
	}
}
