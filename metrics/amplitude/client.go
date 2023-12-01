package amplitude

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/valyala/fastjson"
)

// Client manages the communication to the Amplitude API
type Client struct {
	eventEndpoint string
	key           string
	client        *http.Client
	arenaPool     *fastjson.ArenaPool
}

type AppInfoProvider interface {
	GetAppVersion() string
	GetStartVersion() string
	GetDeviceId() string
	GetPlatform() string
	GetUserId() string
}

type Event interface {
	GetBackend() MetricsBackend
	MarshalFastJson(arena *fastjson.Arena) JsonEvent
	SetTimestamp()
	GetTimestamp() int64
}

type MetricsBackend int
type JsonEvent *fastjson.Value

// New client with API key
func New(eventEndpoint string, key string) *Client {
	return &Client{
		eventEndpoint: eventEndpoint,
		key:           key,
		client:        new(http.Client),
		arenaPool:     &fastjson.ArenaPool{},
	}
}

func (c *Client) SetClient(client *http.Client) {
	c.client = client
}

func (c *Client) SendEvents(amplEvents []Event, info AppInfoProvider) error {
	arena := c.arenaPool.Get()
	appVersion := arena.NewString(info.GetAppVersion())
	deviceId := arena.NewString(info.GetDeviceId())
	platform := arena.NewString(info.GetPlatform())
	startVersion := arena.NewString(info.GetStartVersion())
	userId := arena.NewString(info.GetUserId())

	req := arena.NewObject()
	req.Set("api_key", arena.NewString(c.key))

	events := arena.NewArray()
	for i, ev := range amplEvents {
		ampEvent := *ev.MarshalFastJson(arena)
		ampEvent.Set("app_version", appVersion)
		ampEvent.Set("device_id", deviceId)
		ampEvent.Set("platform", platform)
		ampEvent.Set("start_version", startVersion)
		ampEvent.Set("user_id", userId)
		ampEvent.Set("time", arena.NewNumberInt(int(ev.GetTimestamp())))

		events.SetArrayItem(i, &ampEvent)
	}

	req.Set("events", events)

	evJSON := req.MarshalTo(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
		// flush arena
		arena.Reset()
		c.arenaPool.Put(arena)
	}()
	r, err := http.NewRequestWithContext(ctx, "POST", c.eventEndpoint, bytes.NewReader(evJSON))
	if err != nil {
		return err
	}

	r.Header.Set("content-type", "application/json")
	resp, err := c.client.Do(r)
	if err == nil {
		return resp.Body.Close()
	}

	return err
}
