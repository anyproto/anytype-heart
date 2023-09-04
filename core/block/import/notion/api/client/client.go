package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var logger = logging.Logger("notion-api-client")

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

// DoWithRetry retries in case of network error, 429 and >500 response codes
// in case retry-after header is available it uses it, otherwise gradually increase the delay
// can be canceled with the request's timeout
// 0 maxAttempts means no limit
func (c *Client) DoWithRetry(loggerInfo string, maxAttempts int, req *http.Request) (*http.Response, error) {
	var (
		delay   = time.Second * 5
		attempt = 0
		body    []byte
	)
	lg := logger.With("info", loggerInfo)
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
	}
retry:
	for {
		retryReason := ""
		if body != nil {
			// replace body reader cause it could be already read
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() {
					lg.Warnf("network timeout error: %s", netErr)
				} else if netErr.Temporary() {
					lg.Warnf("network temporary error: %s", netErr)
				}
				retryReason = netErr.Error()
			} else {
				return nil, fmt.Errorf("http error: %s", err)
			}
		} else if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			e := GetRetryAfterError(res.Header)
			if e.RetryAfterSeconds > 0 {
				delay = time.Second * time.Duration(e.RetryAfterSeconds)
			}
			retryReason = fmt.Sprintf("code%d", res.StatusCode)
		} else {
			return res, nil
		}
		lg = lg.With("reason", retryReason)
		attempt++
		if maxAttempts > 0 && attempt >= maxAttempts {
			lg.Warnf("max attempts exceeded")
			return res, err
		}
		lg.With("delay", delay.Seconds()).With("attempt", attempt).Warnf("retry request")

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
			delay = delay * 2
			continue retry
		}
	}
}
