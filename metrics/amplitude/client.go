package amplitude

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const eventEndpoint = "https://amplitude.anytype.io/2/httpapi"

// Client manages the communication to the Amplitude API
type Client struct {
	eventEndpoint string
	key           string
	client        *http.Client
}

type Event struct {
	AppVersion      string                 `json:"app_version,omitempty"`
	DeviceID        string                 `json:"device_id,omitempty"`
	EventID         int                    `json:"event_id,omitempty"`
	EventProperties map[string]interface{} `json:"event_properties,omitempty"`
	EventType       string                 `json:"event_type,omitempty"`
	Groups          map[string]interface{} `json:"groups,omitempty"`
	OsName          string                 `json:"os_name,omitempty"`
	OsVersion       string                 `json:"os_version,omitempty"`
	Platform        string                 `json:"platform,omitempty"`
	ProductID       string                 `json:"productId,omitempty"`
	Quantity        int                    `json:"quantity,omitempty"`
	SessionID       int64                  `json:"session_id,omitempty"`
	StartVersion    string                 `json:"start_version,omitempty"`
	Time            int64                  `json:"time,omitempty"`
	UserID          string                 `json:"user_id,omitempty"`
	UserProperties  map[string]interface{} `json:"user_properties,omitempty"`
}

type EventRequest struct {
	APIKey string  `json:"api_key,omitempty"`
	Events []Event `json:"events,omitempty"`
}

// New client with API key
func New(key string) *Client {
	return &Client{
		eventEndpoint: eventEndpoint,
		key:           key,
		client:        new(http.Client),
	}
}

func (c *Client) SetClient(client *http.Client) {
	c.client = client
}

func (c *Client) Events(events []Event) error {
	req := EventRequest{
		APIKey: c.key,
		Events: events,
	}
	evJSON, err := json.Marshal(req)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "POST", eventEndpoint, bytes.NewReader(evJSON))
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

func (c *Client) Event(msg Event) error {
	return c.Events([]Event{msg})
}
