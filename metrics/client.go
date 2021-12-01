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
	bufferSize       = 500
)

type EventRepresentable interface {
	ToEvent() Event
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

	SetOSVersion(v string)
	SetAppVersion(v string)
	SetDeviceType(t string)
	SetUserId(id string)

	RecordEvent(ev EventRepresentable)
	AggregateEvent(ev EventAggregatable)

	Run()
	Close()
}

type client struct {
	lock             sync.RWMutex
	osVersion        string
	appVersion       string
	userId           string
	deviceType       string
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

func (c *client) SetOSVersion(v string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.osVersion = v
}

func (c *client) SetDeviceType(t string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.deviceType = t
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

func (c *client) sendAggregatedData() {
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
	go func() {
		ticker := time.NewTicker(sendInterval)
		for {
			select {
			case <-ticker.C:
				c.sendAggregatedData()
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
			case <-c.ctx.Done():
				c.sendAggregatedData()
				return
			}
		}
	}()
}

func (c *client) startSendingBatchMessages() {
	for {
		c.lock.Lock()
		b := c.batcher
		c.lock.Unlock()
		if b == nil {
			return
		}
		msgs := b.WaitMinMax(10, 100)
		if len(msgs) == 0 {
			c.sendNextBatch(nil, b.GetAll())
			close(c.closeChannel)
			return
		}
		c.sendNextBatch(b, msgs)
		<-time.After(time.Second * 2)
	}
}

func (c *client) Close() {
	c.lock.Lock()
	if c.batcher == nil {
		c.lock.Unlock()
		return
	}
	err := c.batcher.Close()
	if err != nil {
		clientMetricsLog.Errorf("failed to close batcher")
	}
	c.lock.Unlock()

	c.cancel()
	<-c.closeChannel

	c.lock.Lock()
	defer c.lock.Unlock()
	c.batcher = nil
}

func (c *client) sendNextBatch(b *mb.MB, msgs []interface{}) {
	if len(msgs) == 0 {
		return
	}

	var events []amplitude.Event
	for _, ev := range msgs {
		events = append(events, ev.(amplitude.Event))
	}
	err := c.amplitude.Events(events)
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
}

func (c *client) RecordEvent(ev EventRepresentable) {
	if c.amplitude == nil || ev == nil {
		return
	}
	e := ev.ToEvent()
	c.lock.RLock()
	ampEvent := amplitude.Event{
		UserId:          c.userId,
		OsVersion:       c.osVersion,
		DeviceType:      c.deviceType,
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
		Debug("event added to batcher")
}

func (c *client) AggregateEvent(ev EventAggregatable) {
	select {
	case c.aggregatableChan <- ev:
	default:
	}
}
