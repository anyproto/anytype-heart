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
	Adid               string                 `json:"adid,omitempty"`
	AppVersion         string                 `json:"app_version,omitempty"`
	Carrier            string                 `json:"carrier,omitempty"`
	City               string                 `json:"city,omitempty"`
	Country            string                 `json:"country,omitempty"`
	DeviceBrand        string                 `json:"device_brand,omitempty"`
	DeviceID           string                 `json:"device_id,omitempty"`
	DeviceManufacturer string                 `json:"device_manufacturer,omitempty"`
	DeviceModel        string                 `json:"device_model,omitempty"`
	DeviceType         string                 `json:"device_type,omitempty"`
	Dma                string                 `json:"dma,omitempty"`
	EventID            int                    `json:"event_id,omitempty"`
	EventProperties    map[string]interface{} `json:"event_properties,omitempty"`
	EventType          string                 `json:"event_type,omitempty"`
	Groups             map[string]interface{} `json:"groups,omitempty"`
	Ifda               string                 `json:"ifda,omitempty"`
	InsertID           string                 `json:"insert_id,omitempty"`
	IP                 string                 `json:"ip,omitempty"`
	Language           string                 `json:"language,omitempty"`
	LocationLat        string                 `json:"location_lat,omitempty"`
	LocationLng        string                 `json:"location_lng,omitempty"`
	OsName             string                 `json:"os_name,omitempty"`
	OsVersion          string                 `json:"os_version,omitempty"`
	Paying             string                 `json:"paying,omitempty"`
	Platform           string                 `json:"platform,omitempty"`
	Price              float64                `json:"price,omitempty"`
	ProductID          string                 `json:"productId,omitempty"`
	Quantity           int                    `json:"quantity,omitempty"`
	Region             string                 `json:"region,omitempty"`
	Revenue            float64                `json:"revenue,omitempty"`
	RevenueType        string                 `json:"revenueType,omitempty"`
	SessionID          int64                  `json:"session_id,omitempty"`
	StartVersion       string                 `json:"start_version,omitempty"`
	Time               int64                  `json:"time,omitempty"`
	UserID             string                 `json:"user_id,omitempty"`
	UserProperties     map[string]interface{} `json:"user_properties,omitempty"`
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
