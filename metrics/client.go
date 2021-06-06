package metrics

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/msingleton/amplitude-go"
)

var clientMetricsLog = logging.Logger("client-metrics")

type EventRepresentable interface {
	ToEvent() Event
}

type Event struct {
	EventType string
	EventData map[string]interface{}
}

type Client interface {
	SetOSVersion(v string)
	SetDeviceType(t string)
	SetUserId(id string)
	RecordEvent(ev EventRepresentable)
}

type client struct {
	osVersion  string
	userId     string
	deviceType string
	amplitude  *amplitude.Client
}

func NewClient(apiKey string) Client {
	return &client{
		amplitude: amplitude.New(apiKey),
	}
}

func (c *client) SetOSVersion(v string) {
	c.osVersion = v
}

func (c *client) SetDeviceType(t string) {
	c.deviceType = t
}

func (c *client) SetUserId(id string) {
	c.userId = id
}

func (c client) RecordEvent(ev EventRepresentable) {
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
