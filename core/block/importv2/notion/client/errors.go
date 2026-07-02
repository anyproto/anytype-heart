package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Sentinel errors callers branch on; the adapter maps them to wire codes.
var (
	ErrUnauthorized = errors.New("notion: unauthorized")
	ErrForbidden    = errors.New("notion: forbidden")
	ErrRateLimited  = errors.New("notion: rate limited")
	ErrUnavailable  = errors.New("notion: service unavailable")
	ErrNotFound     = errors.New("notion: object not found")
)

// apiError is one non-200 response, keeping the API's code/message and the
// server's retry hint.
type apiError struct {
	status     int
	code       string
	message    string
	retryAfter time.Duration
	sentinel   error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("notion api %d %s: %s", e.status, e.code, e.message)
}

func (e *apiError) Unwrap() error {
	return e.sentinel
}

// transportError wraps network-level failures (always retryable).
type transportError struct {
	err error
}

func (e *transportError) Error() string { return "notion transport: " + e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func errorFromResponse(res *http.Response) error {
	apiErr := &apiError{status: res.StatusCode}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if data, err := io.ReadAll(io.LimitReader(res.Body, 1<<16)); err == nil {
		if json.Unmarshal(data, &body) == nil {
			apiErr.code, apiErr.message = body.Code, body.Message
		}
	}
	if header := res.Header.Get("Retry-After"); header != "" {
		if seconds, err := strconv.ParseFloat(header, 64); err == nil && seconds > 0 {
			apiErr.retryAfter = time.Duration(seconds * float64(time.Second))
		}
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		apiErr.sentinel = ErrUnauthorized
	case res.StatusCode == http.StatusForbidden:
		apiErr.sentinel = ErrForbidden
	case res.StatusCode == http.StatusNotFound:
		apiErr.sentinel = ErrNotFound
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 529:
		// 529 is Notion's service_overload; the docs mandate identical
		// handling to 429.
		apiErr.sentinel = ErrRateLimited
	case res.StatusCode >= 500:
		apiErr.sentinel = ErrUnavailable
	}
	return apiErr
}

// isRetryable: network errors, rate limiting (429/529) and 5xx. Client
// errors (auth, not-found, validation) return to the caller immediately.
func isRetryable(err error) bool {
	var transport *transportError
	if errors.As(err, &transport) {
		return true
	}
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable)
}

func retryAfterOf(err error) time.Duration {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.retryAfter
	}
	return 0
}
