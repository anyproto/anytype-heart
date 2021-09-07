package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/msingleton/amplitude-go"
)

var (
	SharedClient     = NewClient()
	clientMetricsLog = logging.Logger("client-metrics")
	sendInterval     = 10.0 * time.Second
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

	StartAggregating()
	StopAggregating()
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
}

func NewClient() Client {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	return &client{
		aggregatableMap:  make(map[string]EventAggregatable),
		aggregatableChan: make(chan EventAggregatable),
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
	c.lock.RLock()
	defer c.lock.RUnlock()
	for k, ev := range c.aggregatableMap {
		c.RecordEvent(ev)
		delete(c.aggregatableMap, k)
	}
}

func (c *client) StartAggregating() {
	c.lock.Lock()
	c.StopAggregating()
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.lock.Unlock()
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

func (c *client) StopAggregating() {
	c.cancel()
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
	}
	c.lock.RUnlock()

	go func() {
		err := c.amplitude.Event(ampEvent)
		if err != nil {
			clientMetricsLog.Errorf("error logging event %s", err)
			return
		}
		clientMetricsLog.
			With("event-type", e.EventType).
			With("event-data", e.EventData).
			With("user-id", c.userId).
			Errorf("event sent")
	}()
}

func (c *client) AggregateEvent(ev EventAggregatable) {
	c.aggregatableChan <- ev
}
