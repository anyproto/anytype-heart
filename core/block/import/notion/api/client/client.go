package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	notionUrl     = "https://api.notion.com/v1"
	apiVersion    = "2022-06-28"
)

type Client struct {
	HttpClient *http.Client
	BasePath string 
}

func NewClient() *Client {
	c := &Client{
		HttpClient:&http.Client{Timeout: time.Minute},
		BasePath: notionUrl,
	}
	return c
}

func (c *Client) PrepareRequest(ctx context.Context, apiKey, method, url string, body *bytes.Buffer) (*http.Request, error) {
	resultUrl := c.BasePath + url
	req, err := http.NewRequestWithContext(ctx, method, resultUrl, body)
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
