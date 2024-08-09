package anymetry

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/klauspost/compress/gzhttp"
	"github.com/klauspost/compress/gzip"
	"github.com/valyala/fastjson"
)

type Service interface {
	SendEvents(amplEvents []Event, info AppInfoProvider) error
}

type Client struct {
	Service
	eventEndpoint string
	key           string
	client        *http.Client
	arenaPool     *fastjson.ArenaPool
	isCompressed  bool
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
func New(eventEndpoint string, key string, isCompressed bool) Service {
	var httpClient *http.Client
	if isCompressed {
		httpClient = &http.Client{
			Transport: gzhttp.Transport(http.DefaultTransport),
		}
	} else {
		httpClient = &http.Client{}
	}
	return &Client{
		eventEndpoint: eventEndpoint,
		key:           key,
		client:        httpClient,
		arenaPool:     &fastjson.ArenaPool{},
	}
}

func (c *Client) SendEvents(amplEvents []Event, info AppInfoProvider) error {
	if c.key == "" {
		return nil
	}
	if len(amplEvents) == 0 {
		return nil
	}
	arena := c.arenaPool.Get()
	appVersion := arena.NewString(info.GetAppVersion())
	deviceId := arena.NewString(info.GetDeviceId())
	platform := arena.NewString(info.GetPlatform())
	startVersion := arena.NewString(info.GetStartVersion())
	userId := arena.NewString(info.GetUserId())

	reqJSON := arena.NewObject()
	reqJSON.Set("api_key", arena.NewString(c.key))

	events := arena.NewArray()
	amIndex := 0
	for _, ev := range amplEvents {
		tryEvent := ev.MarshalFastJson(arena)
		if tryEvent == nil {
			continue
		}
		ampEvent := *tryEvent
		ampEvent.Set("app_version", appVersion)
		ampEvent.Set("device_id", deviceId)
		ampEvent.Set("platform", platform)
		ampEvent.Set("start_version", startVersion)
		ampEvent.Set("user_id", userId)
		ampEvent.Set("time", arena.NewNumberInt(int(ev.GetTimestamp())))

		events.SetArrayItem(amIndex, &ampEvent)
		amIndex++
	}

	if amIndex == 0 {
		return nil
	}

	reqJSON.Set("events", events)

	evJSON := reqJSON.MarshalTo(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
		// flush arena
		arena.Reset()
		c.arenaPool.Put(arena)
	}()

	reader, err := c.getBody(evJSON)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.eventEndpoint, reader)
	if err != nil {
		return err
	}
	if c.isCompressed {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err == nil {
		return resp.Body.Close()
	}

	return err
}

func (c *Client) getBody(evJSON []byte) (io.Reader, error) {
	if !c.isCompressed {
		return bytes.NewReader(evJSON), nil
	}

	var buf *bytes.Buffer
	gzipWriter := gzip.NewWriter(buf)

	_, err := gzipWriter.Write(evJSON)
	if err != nil {
		return nil, err
	}

	err = gzipWriter.Close()
	if err != nil {
		return nil, err
	}

	return buf, nil
}
