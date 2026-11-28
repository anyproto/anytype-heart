package forward

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
)

const loopback = "127.0.0.1"

// dialError marks a failure to connect: nothing was sent.
type dialError struct{ err error }

func (e *dialError) Error() string { return e.err.Error() }
func (e *dialError) Unwrap() error { return e.err }

// newHTTPClient builds the only client the bearer ever travels on: literal
// 127.0.0.1, no proxy, no redirects, no reused connections.
func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _ string, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil || host != loopback {
				return nil, &dialError{errors.New("refusing a non-loopback address")}
			}
			conn, err := dialer.DialContext(ctx, "tcp4", addr)
			if err != nil {
				return nil, &dialError{err}
			}
			return conn, nil
		},
		// one connection per request: Go's transport replays a request
		// only on a connection it reused, and a replay would carry the
		// bearer past the gate
		DisableKeepAlives:  true,
		DisableCompression: true,
	}
	return &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type outcome int

const (
	// outRefused: the connection could not be made; nothing was sent.
	outRefused outcome = iota
	// outLost: the request may have been sent and no usable reply came
	// back: a dropped connection, a truncated body, the deadline.
	outLost
	// outReply: a status line and as much body as the rules allow.
	outReply
)

type exchange struct {
	out     outcome
	status  int
	session string // Mcp-Session-Id of the reply
	isJSON  bool
	body    []byte
	bodyErr error // the body could not be read whole
}

type upstream struct {
	client     *http.Client
	timeout    time.Duration
	errBodyFor time.Duration
}

func endpoint(snap snapshot) string {
	return fmt.Sprintf("http://%s:%d/mcp/%s", loopback, snap.port, snap.tier)
}

// do runs one exchange. The deadline covers the body.
func (u *upstream) do(ctx context.Context, method string, snap snapshot, session, version string, body []byte) exchange {
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint(snap), rd)
	if err != nil {
		return exchange{out: outRefused}
	}
	req.Header.Set("Authorization", "Bearer "+snap.key)
	req.Header.Set("Accept", "application/json, text/event-stream")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set(contract.SessionHeader, session)
	}
	if version != "" {
		req.Header.Set(contract.ProtocolHeader, version)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		var de *dialError
		if errors.As(err, &de) && ctx.Err() == nil {
			return exchange{out: outRefused}
		}
		return exchange{out: outLost}
	}
	defer resp.Body.Close()
	ex := exchange{out: outReply, status: resp.StatusCode, session: resp.Header.Get(contract.SessionHeader)}
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil && mt == "application/json" {
		ex.isJSON = true
	}
	if resp.StatusCode == http.StatusOK {
		data, err := io.ReadAll(io.LimitReader(resp.Body, contract.MaxReplyBytes+1))
		switch {
		case err != nil:
			ex.bodyErr = err
		case len(data) > contract.MaxReplyBytes:
			ex.bodyErr = errors.New("reply too large")
		default:
			ex.body = data
		}
		return ex
	}
	// a non-200 body is only ever a short message: read a bounded piece
	// and never wait long for it, so a stalled error body can't hold the
	// status back
	type read struct {
		data []byte
		err  error
	}
	done := make(chan read, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(resp.Body, contract.MaxErrorBodyBytes))
		done <- read{data, err}
	}()
	timer := time.NewTimer(u.errBodyFor)
	defer timer.Stop()
	select {
	case r := <-done:
		ex.body, ex.bodyErr = r.data, r.err
	case <-timer.C:
		ex.bodyErr = errors.New("error body stalled")
	case <-ctx.Done():
		ex.bodyErr = ctx.Err()
	}
	return ex
}

// restError is heart's REST error shape (util.CodeToApiError).
type restError struct {
	Object  string `json:"object"`
	Message string `json:"message"`
}

func restMessage(body []byte) (string, bool) {
	var e restError
	if json.Unmarshal(body, &e) != nil || e.Object != "error" || e.Message == "" {
		return "", false
	}
	return e.Message, true
}

// rpcBody is an upstream JSON-RPC response.
type rpcBody struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`

	code    int
	message string
}

// parseRPC reads an upstream body as a JSON-RPC response with exactly one
// of result and error.
func parseRPC(body []byte) (*rpcBody, bool) {
	var b rpcBody
	if json.Unmarshal(body, &b) != nil || b.JSONRPC != "2.0" {
		return nil, false
	}
	hasResult := len(b.Result) > 0 && !isNull(b.Result)
	hasError := len(b.Error) > 0 && !isNull(b.Error)
	if hasResult == hasError {
		return nil, false
	}
	if hasError {
		var e struct {
			Code    *int    `json:"code"`
			Message *string `json:"message"`
		}
		if json.Unmarshal(b.Error, &e) != nil || e.Code == nil || e.Message == nil {
			return nil, false
		}
		b.code, b.message = *e.Code, *e.Message
	} else {
		b.Error = nil
	}
	return &b, true
}

// redact makes an upstream message safe and short for a reply: no key, no
// control characters, one line, bounded.
func redact(msg, key string) string {
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[redacted]")
	}
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, msg)
	msg = strings.TrimSpace(msg)
	const max = 400
	if r := []rune(msg); len(r) > max {
		msg = string(r[:max]) + "…"
	}
	return msg
}
