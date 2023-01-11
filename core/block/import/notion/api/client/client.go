package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	notionURL  = "https://api.notion.com/v1"
	apiVersion = "2022-06-28"
)

type Client struct {
	HTTPClient *http.Client
	BasePath   string
}

// NewClient is a constructor for Client
func NewClient() *Client {
	c := &Client{
		HTTPClient: &http.Client{Timeout: time.Minute},
		BasePath:   notionURL,
	}
	return c
}

// PrepareRequest create http.Request based on given method, url and body
func (c *Client) PrepareRequest(ctx context.Context,
	apiKey, method, url string,
	body *bytes.Buffer) (*http.Request, error) {
	resultURL := c.BasePath + url
	req, err := http.NewRequestWithContext(ctx, method, resultURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", apiKey))
	req.Header.Set("Notion-Version", apiVersion)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}
