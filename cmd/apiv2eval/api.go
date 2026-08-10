package main

// api.go — the harness's own thin client over the local /v2 API. Fixture
// setup and the programmatic success checks run through it, deliberately NOT
// through the wrapper under test: a check that shares the code under test
// proves nothing. It also carries the recording transport every arm shares,
// so every HTTP exchange a run makes — including the ones the wrapper makes
// on the model's behalf — lands in the attempt record with its status, C6
// code and issue paths.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// maxRecordedBody bounds how much of a request body an exchange record
// keeps: the model's own payloads are the interesting part and they are
// small, while a fixture's markdown or a full document read is not worth
// carrying into every JSONL line.
const maxRecordedBody = 8 << 10

// exchange is one recorded HTTP call to the local API.
type exchange struct {
	At          time.Time       `json:"at"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Status      int             `json:"status"`
	Code        string          `json:"code,omitempty"`
	Message     string          `json:"message,omitempty"`
	Issues      []v2model.Issue `json:"issues,omitempty"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
	// Transport is set when the call never got an HTTP response at all (the
	// API is down): that is an environment fact, not an API error.
	Transport string `json:"transport_error,omitempty"`
}

// recorder collects exchanges for the attempt currently running.
type recorder struct {
	mu sync.Mutex
	ex []exchange
}

func (r *recorder) add(e exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ex = append(r.ex, e)
}

// take returns the exchanges recorded so far and clears the buffer, so one
// attempt's record never carries another's.
func (r *recorder) take() []exchange {
	r.mu.Lock()
	defer r.mu.Unlock()
	ex := r.ex
	r.ex = nil
	return ex
}

// mark returns the current position; since returns everything recorded
// after one. Together they attribute exchanges to the tool call that made
// them — which is what turns a text tip the model saw into the (status,
// code, path) triple the report needs.
func (r *recorder) mark() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ex)
}

func (r *recorder) since(mark int) []exchange {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mark < 0 || mark > len(r.ex) {
		return nil
	}
	out := make([]exchange, len(r.ex)-mark)
	copy(out, r.ex[mark:])
	return out
}

// recordingTransport records every request/response pair. It is the ONE
// place structured error facts are captured: the MCP delivery hands the
// model a text tip (by design), so status, C6 code and issue paths would
// otherwise be unrecoverable from the transcript alone.
type recordingTransport struct {
	base http.RoundTripper
	rec  *recorder
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	e := exchange{At: time.Now(), Method: req.Method, Path: req.URL.Path}
	if req.Body != nil && req.Method != http.MethodGet {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read request body for recording: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) <= maxRecordedBody && json.Valid(body) {
			e.RequestBody = json.RawMessage(body)
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		e.Transport = err.Error()
		t.rec.add(e)
		return nil, err
	}
	e.Status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response body for recording: %w", readErr)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		var apiErr v2model.Error
		if json.Unmarshal(body, &apiErr) == nil {
			e.Code, e.Message, e.Issues = apiErr.Code, apiErr.Message, apiErr.Issues
		} else {
			e.Message = strings.TrimSpace(string(body))
		}
	}
	t.rec.add(e)
	return resp, nil
}

// apiClient is the harness's direct /v2 client.
type apiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newAPIClient(baseURL, apiKey string, rt http.RoundTripper) *apiClient {
	return &apiClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second, Transport: rt},
	}
}

// apiError carries a non-2xx answer with its C6 fields intact.
type apiError struct {
	Status  int
	Code    string
	Message string
	Issues  []v2model.Issue
}

func (e *apiError) Error() string {
	return fmt.Sprintf("api %d %s: %s", e.Status, e.Code, e.Message)
}

// call runs one request; out (when non-nil) receives the decoded 2xx body.
func (c *apiClient) call(ctx context.Context, method, path string, query url.Values, body, out any) ([]byte, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if method != http.MethodGet {
		// C8: every mutation the harness itself makes carries a key too —
		// fixture creation is a mutation and the retry policy is the server's
		req.Header.Set("Idempotency-Key", newNonce(16))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("read response of %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := &apiError{Status: resp.StatusCode}
		var decoded v2model.Error
		if json.Unmarshal(raw, &decoded) == nil && decoded.Message != "" {
			e.Code, e.Message, e.Issues = decoded.Code, decoded.Message, decoded.Issues
		} else {
			e.Message = strings.TrimSpace(string(raw))
		}
		return raw, e
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("decode response of %s %s: %w", method, path, err)
		}
	}
	return raw, nil
}

// whoami is the preflight call: it proves the server is up AND the key is
// accepted, which are two different failures with two different fixes.
func (c *apiClient) whoami(ctx context.Context) error {
	_, err := c.call(ctx, http.MethodGet, "/v2/auth/whoami", nil, nil, nil)
	if err != nil {
		return fmt.Errorf("whoami: %w", err)
	}
	return nil
}

type spaceRow struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (c *apiClient) listSpaces(ctx context.Context) ([]spaceRow, error) {
	var resp struct {
		Data []spaceRow `json:"data"`
	}
	if _, err := c.call(ctx, http.MethodGet, "/v2/spaces", url.Values{"limit": {"100"}}, nil, &resp); err != nil {
		return nil, fmt.Errorf("list spaces: %w", err)
	}
	return resp.Data, nil
}

func (c *apiClient) createSpace(ctx context.Context, name string) (string, error) {
	var resp v2model.CreateResult
	if _, err := c.call(ctx, http.MethodPost, "/v2/spaces", nil, map[string]any{"name": name}, &resp); err != nil {
		return "", fmt.Errorf("create space %q: %w", name, err)
	}
	return resp.Id, nil
}

// createObject makes one fixture object.
func (c *apiClient) createObject(ctx context.Context, spaceId, typeKey, name, markdown string) (string, error) {
	body := map[string]any{"type": typeKey, "name": name}
	if markdown != "" {
		body["markdown"] = markdown
	}
	var resp v2model.CreateResult
	path := "/v2/spaces/" + url.PathEscape(spaceId) + "/objects"
	if _, err := c.call(ctx, http.MethodPost, path, nil, body, &resp); err != nil {
		return "", fmt.Errorf("create object %q: %w", name, err)
	}
	return resp.Id, nil
}

// searchPollInterval is how often waitSearchable re-asks.
const searchPollInterval = 500 * time.Millisecond

// waitSearchable blocks until a search for the fixture's title returns it.
// Full-text indexing is asynchronous, so a fixture created a moment ago can
// be invisible to the search the wrapper arm's `find` runs — an attempt that
// starts before the index catches up fails for a reason that has nothing to
// do with the model or the API contract. Returns whether it became visible
// and how long that took.
func (c *apiClient) waitSearchable(ctx context.Context, spaceId, title, objectId string, timeout time.Duration) (bool, time.Duration, error) {
	deadline := time.Now().Add(timeout)
	path := "/v2/spaces/" + url.PathEscape(spaceId) + "/search"
	start := time.Now()
	for {
		var resp struct {
			Data []struct {
				Id string `json:"id"`
			} `json:"data"`
		}
		if _, err := c.call(ctx, http.MethodPost, path, url.Values{"limit": {"25"}},
			map[string]any{"query": title}, &resp); err != nil {
			return false, time.Since(start), fmt.Errorf("search for the fixture: %w", err)
		}
		for _, row := range resp.Data {
			if row.Id == objectId {
				return true, time.Since(start), nil
			}
		}
		if time.Now().After(deadline) {
			return false, time.Since(start), nil
		}
		select {
		case <-time.After(searchPollInterval):
		case <-ctx.Done():
			return false, time.Since(start), ctx.Err()
		}
	}
}

// typeExists reports whether a type key resolves in the space.
func (c *apiClient) typeExists(ctx context.Context, spaceId, typeKey string) bool {
	path := "/v2/spaces/" + url.PathEscape(spaceId) + "/types/" + url.PathEscape(typeKey)
	_, err := c.call(ctx, http.MethodGet, path, nil, nil, nil)
	return err == nil
}

//
// ---- the served document, as the checks read it ----
//

// document is the subset of a served object the checks assert on.
type document struct {
	Id         string         `json:"id"`
	Properties map[string]any `json:"properties"`
	Blocks     []docBlock     `json:"blocks"`
}

type docBlock struct {
	Id      string        `json:"id"`
	Type    string        `json:"type"`
	Text    string        `json:"text"`
	Indent  float64       `json:"indent"`
	Checked bool          `json:"checked"`
	Columns []docTableId  `json:"columns"`
	Rows    []docTableRow `json:"rows"`
}

type docTableId struct {
	Id string `json:"id"`
}

type docTableRow struct {
	Id       string            `json:"id"`
	IsHeader bool              `json:"isHeader"`
	Cells    []json.RawMessage `json:"cells"`
}

// getDocument reads the object in the DEFAULT (compact-label) shape — the
// same bytes the model's read serves, so a check and an echo classifier
// agree about what an id looks like.
func (c *apiClient) getDocument(ctx context.Context, spaceId, objectId string) (*document, []byte, error) {
	path := "/v2/spaces/" + url.PathEscape(spaceId) + "/objects/" + url.PathEscape(objectId)
	var doc document
	raw, err := c.call(ctx, http.MethodGet, path, nil, nil, &doc)
	if err != nil {
		return nil, raw, fmt.Errorf("read object %s: %w", objectId, err)
	}
	return &doc, raw, nil
}

// patchOps sends a single-op PATCH (the ops arm's executor).
func (c *apiClient) patchOps(ctx context.Context, spaceId, objectId string, ops []any) (*v2model.EditResult, error) {
	path := "/v2/spaces/" + url.PathEscape(spaceId) + "/objects/" + url.PathEscape(objectId)
	var result v2model.EditResult
	if _, err := c.call(ctx, http.MethodPatch, path, nil, map[string]any{"ops": ops}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// opSchema fetches one op's published schema (GET /v2/schemas/ops/{op}) —
// the bytes the ops arm serves the model as that tool's parameters.
func (c *apiClient) opSchema(ctx context.Context, op string) (json.RawMessage, json.RawMessage, error) {
	var entry struct {
		Schema  json.RawMessage `json:"schema"`
		Example json.RawMessage `json:"example"`
	}
	if _, err := c.call(ctx, http.MethodGet, "/v2/schemas/ops/"+url.PathEscape(op), nil, nil, &entry); err != nil {
		return nil, nil, fmt.Errorf("fetch op schema %q: %w", op, err)
	}
	return entry.Schema, entry.Example, nil
}

//
// ---- document helpers the checks share ----
//

// blockTexts returns every block's text in document order.
func (d *document) blockTexts() []string {
	out := make([]string, 0, len(d.Blocks))
	for _, b := range d.Blocks {
		out = append(out, b.Text)
	}
	return out
}

// allText joins every block's text, table cells included.
func (d *document) allText() string {
	var b strings.Builder
	for _, blk := range d.Blocks {
		b.WriteString(blk.Text)
		b.WriteString("\n")
		for _, row := range blk.Rows {
			for _, cell := range row.Cells {
				b.WriteString(cellText(cell))
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// findBlock returns the first block satisfying pred.
func (d *document) findBlock(pred func(docBlock) bool) (docBlock, bool) {
	for _, b := range d.Blocks {
		if pred(b) {
			return b, true
		}
	}
	return docBlock{}, false
}

// table returns the first table block.
func (d *document) table() (docBlock, bool) {
	return d.findBlock(func(b docBlock) bool { return b.Type == "table" })
}

// cellText renders a §6.1 cell (string | null | object | array of blocks) as
// its text.
func cellText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Text
	}
	var arr []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		return arr[0].Text
	}
	return ""
}

// stringProperty reads a scalar string property value.
func (d *document) stringProperty(key string) (string, bool) {
	v, ok := d.Properties[key]
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case []any:
		if len(t) == 1 {
			if s, ok := t[0].(string); ok {
				return s, true
			}
		}
	}
	return "", false
}
