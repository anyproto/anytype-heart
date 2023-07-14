package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
	"strconv"
)

const (
	notionURL  = "https://api.notion.com/v1"
	apiVersion = "2022-06-28"
)

type ErrRateLimited struct {
	RetryAfterSeconds int64
}

func (e *ErrRateLimited) Error() string {
	return "rate limited"
}

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
	body io.Reader) (*http.Request, error) {
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
func GetRetryAfterError(h http.Header) *ErrRateLimited {
	var retryAfter int64
	retryAfterStr := h.Get("retry-after")
	if retryAfterStr != "" {
		retryAfter, _ = strconv.ParseInt(retryAfterStr, 10, 64)
	}
	return &ErrRateLimited{RetryAfterSeconds: retryAfter}
}
