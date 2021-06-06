package metrics

import (
	"sync"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/msingleton/amplitude-go"
)

var (
	SharedClient     = NewClient()
	clientMetricsLog = logging.Logger("client-metrics")
)

type EventRepresentable interface {
	ToEvent() Event
}

type Event struct {
	EventType string
	EventData map[string]interface{}
}

type Client interface {
	InitWithKey(k string)
	SetOSVersion(v string)
	SetDeviceType(t string)
	SetUserId(id string)
	RecordEvent(ev EventRepresentable)
}

type client struct {
	lock       sync.Mutex
	osVersion  string
	userId     string
	deviceType string
	amplitude  *amplitude.Client
}

func NewClient() Client {
	return &client{}
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

func (c *client) RecordEvent(ev EventRepresentable) {
	if c.amplitude == nil {
		return
	}
	go func() {
		e := ev.ToEvent()
		err := c.amplitude.Event(amplitude.Event{
			UserId:          c.userId,
			OsVersion:       c.osVersion,
			DeviceType:      c.deviceType,
			EventType:       e.EventType,
			EventProperties: e.EventData,
		})
		if err != nil {
			clientMetricsLog.Errorf("error logging event %s", err)
			return
		}
		clientMetricsLog.
			With("event-type", e.EventType).
			With("event-data", e.EventData).
			Debugf("event sent")
	}()
}
