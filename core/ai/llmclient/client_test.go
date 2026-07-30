package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

var testSchema = json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`)

type fakeServer struct {
	*httptest.Server
	calls    atomic.Int64
	lastBody map[string]any
	respond  func(w http.ResponseWriter, call int64)
}

func newFakeServer(t *testing.T, respond func(w http.ResponseWriter, call int64)) *fakeServer {
	fs := &fakeServer{respond: respond}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		fs.lastBody = body
		fs.respond(w, fs.calls.Add(1))
	}))
	t.Cleanup(fs.Close)
	return fs
}

func respondContent(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": content}}},
		"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": 3},
	})
}

func newTestClient(t *testing.T, url string, opts ...Option) *Client {
	c, err := New(Config{Endpoint: url + "/v1", Model: "test-model", Token: "tok"}, opts...)
	require.NoError(t, err)
	c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() } // no real backoff in tests
	return c
}

func TestCompleteJSON(t *testing.T) {
	t.Run("success carries schema, strict mode and near-zero temperature", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, `{"answer":"42"}`) })
		c := newTestClient(t, fs.URL)
		want := json.RawMessage(`{"answer":"42"}`)

		// when
		got, usage, err := c.CompleteJSON(context.Background(), Request{
			System: "sys", User: "usr", SchemaName: "import_plan", Schema: testSchema,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, Usage{PromptTokens: 12, CompletionTokens: 3}, usage)

		assert.Equal(t, "test-model", fs.lastBody["model"])
		temp, ok := fs.lastBody["temperature"].(float64)
		require.True(t, ok, "temperature must be present, not dropped by omitempty")
		assert.Less(t, temp, 1e-6)
		rf := fs.lastBody["response_format"].(map[string]any)
		assert.Equal(t, "json_schema", rf["type"])
		js := rf["json_schema"].(map[string]any)
		assert.Equal(t, "import_plan", js["name"])
		assert.Equal(t, true, js["strict"])
		schemaBytes, err := json.Marshal(js["schema"])
		require.NoError(t, err)
		assert.JSONEq(t, string(testSchema), string(schemaBytes))
		msgs := fs.lastBody["messages"].([]any)
		require.Len(t, msgs, 2)
		assert.Equal(t, "system", msgs[0].(map[string]any)["role"])
		assert.Equal(t, "user", msgs[1].(map[string]any)["role"])
	})

	t.Run("401 fails immediately as auth error", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
		})
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.ErrorIs(t, err, ErrAuthRequired)
		assert.Equal(t, int64(1), fs.calls.Load())
	})

	t.Run("404 fails immediately as model not found", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"no such model"}}`))
		})
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.ErrorIs(t, err, ErrModelNotFound)
		assert.Equal(t, int64(1), fs.calls.Load())
	})

	t.Run("429 retried then succeeds", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			if call == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
				return
			}
			respondContent(w, `{"answer":"ok"}`)
		})
		c := newTestClient(t, fs.URL)

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"ok"}`), got)
		assert.Equal(t, int64(2), fs.calls.Load())
	})

	t.Run("persistent 5xx exhausts retries", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
		})
		c := newTestClient(t, fs.URL, WithRetryPolicy(RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}))
		c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.ErrorIs(t, err, ErrEndpointUnreachable)
		assert.Equal(t, int64(3), fs.calls.Load())
	})

	t.Run("connection refused maps to unreachable", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {})
		url := fs.URL
		fs.Close()
		c := newTestClient(t, url, WithRetryPolicy(RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond}))
		c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.ErrorIs(t, err, ErrEndpointUnreachable)
	})

	t.Run("empty content is an empty-response error", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, "") })
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.ErrorIs(t, err, ErrEmptyResponse)
	})

	t.Run("cancelled context stops the retry loop", func(t *testing.T) {
		// given
		ctx, cancel := context.WithCancel(context.Background())
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			cancel() // cancel while the first attempt is in flight
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
		})
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(ctx, Request{Schema: testSchema})

		// then
		require.Error(t, err)
		assert.Equal(t, int64(1), fs.calls.Load())
	})
}

func TestFromProto(t *testing.T) {
	t.Run("nil config means feature off", func(t *testing.T) {
		_, ok, err := FromProto(nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("empty model means feature off", func(t *testing.T) {
		_, ok, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OLLAMA})
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("provider default endpoints", func(t *testing.T) {
		for provider, want := range map[pb.RpcAIProvider]string{
			pb.RpcAI_OLLAMA:   "http://localhost:11434/v1",
			pb.RpcAI_LMSTUDIO: "http://localhost:1234/v1",
			pb.RpcAI_LLAMACPP: "http://localhost:8080/v1",
		} {
			cfg, ok, err := FromProto(&pb.RpcAIProviderConfig{Provider: provider, Model: "m"})
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, want, cfg.Endpoint)
		}
	})

	t.Run("explicit endpoint wins", func(t *testing.T) {
		cfg, ok, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OLLAMA, Model: "m", Endpoint: "http://box:9999/v1"})
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "http://box:9999/v1", cfg.Endpoint)
	})

	t.Run("openai without token is an error", func(t *testing.T) {
		_, _, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI, Model: "gpt-4o"})
		require.Error(t, err)
	})

	t.Run("openai with token", func(t *testing.T) {
		cfg, ok, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI, Model: "gpt-4o", Token: "sk-x"})
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "https://api.openai.com/v1", cfg.Endpoint)
		assert.Equal(t, "sk-x", cfg.Token)
	})
}
