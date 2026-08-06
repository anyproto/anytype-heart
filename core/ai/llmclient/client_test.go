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

	t.Run("temperature rejection drops the parameter and retries once", func(t *testing.T) {
		// given — a reasoning-model endpoint that 400s on any temperature
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {})
		fs.respond = func(w http.ResponseWriter, call int64) {
			if _, hasTemp := fs.lastBody["temperature"]; hasTemp {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 1e-45 with this model."}}`))
				return
			}
			respondContent(w, `{"answer":"ok"}`)
		}
		c := newTestClient(t, fs.URL)

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"ok"}`), got)
		assert.Equal(t, int64(2), fs.calls.Load())
		_, hasTemp := fs.lastBody["temperature"]
		assert.False(t, hasTemp, "second attempt must omit temperature")
	})

	t.Run("oversized response body is truncated and fails cleanly", func(t *testing.T) {
		// given — an endpoint streaming far past the cap
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"`))
			junk := make([]byte, 1<<20)
			for i := range junk {
				junk[i] = 'a'
			}
			for i := 0; i < 12; i++ { // ~12MB > maxResponseBytes
				_, _ = w.Write(junk)
			}
			_, _ = w.Write([]byte(`"}}]}`))
		})
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then — decode fails at the cap instead of buffering 12MB
		require.Error(t, err)
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

	t.Run("unknown provider without endpoint is an error", func(t *testing.T) {
		_, _, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAIProvider(99), Model: "m"})
		require.Error(t, err)
	})

	t.Run("openai key over plain http to a remote host is refused", func(t *testing.T) {
		_, _, err := FromProto(&pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OPENAI, Model: "gpt-4o", Token: "sk-x",
			Endpoint: "http://proxy.example.com/v1",
		})
		require.ErrorContains(t, err, "plain http")
	})

	t.Run("openai key over http to localhost is fine", func(t *testing.T) {
		cfg, ok, err := FromProto(&pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OPENAI, Model: "gpt-4o", Token: "sk-x",
			Endpoint: "http://localhost:8080/v1",
		})
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "http://localhost:8080/v1", cfg.Endpoint)
	})

	t.Run("openai with token", func(t *testing.T) {
		cfg, ok, err := FromProto(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI, Model: "gpt-4o", Token: "sk-x"})
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "https://api.openai.com/v1", cfg.Endpoint)
		assert.Equal(t, "sk-x", cfg.Token)
	})
}

func TestTruncatedCompletion(t *testing.T) {
	t.Run("a completion cut off at the token cap is reported as truncation", func(t *testing.T) {
		// given — the provider stopped at max_tokens, so the JSON is a
		// fragment. Reporting it as a parse error would send the caller into a
		// corrective retry that truncates identically, and tell the user their
		// model answered badly when it was cut off.
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message":       map[string]any{"role": "assistant", "content": `{"types":[{"key":"spr`},
					"finish_reason": "length",
				}},
				"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 8192},
			})
		})
		c := newTestClient(t, fs.URL)

		// when
		_, usage, err := c.CompleteJSON(context.Background(), Request{
			System: "sys", User: "usr", SchemaName: "import_plan", Schema: testSchema, MaxTokens: 8192,
		})

		// then
		require.ErrorIs(t, err, ErrResponseTruncated)
		assert.Equal(t, 8192, usage.CompletionTokens, "usage still reported, so the cap is diagnosable")
	})

	t.Run("a normal stop is not mistaken for truncation", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message":       map[string]any{"role": "assistant", "content": `{"answer":"42"}`},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 3},
			})
		})
		c := newTestClient(t, fs.URL)

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{System: "sys", User: "usr", SchemaName: "n", Schema: testSchema})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"42"}`), got)
	})
}

func TestMaxTokensParameterFallback(t *testing.T) {
	t.Run("a model rejecting max_tokens is retried with max_completion_tokens", func(t *testing.T) {
		// given — the gpt-5 class and o-series reject max_tokens outright:
		// "this model is not supported MaxTokens, please use
		// MaxCompletionTokens". It is a deterministic 400, so retrying the
		// same request can never succeed.
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {})
		fs.respond = func(w http.ResponseWriter, call int64) {
			if _, legacy := fs.lastBody["max_tokens"]; legacy {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"this model is not supported MaxTokens, please use MaxCompletionTokens"}}`))
				return
			}
			respondContent(w, `{"answer":"ok"}`)
		}
		c := newTestClient(t, fs.URL)

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema, MaxTokens: 4096})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"ok"}`), got)
		assert.Equal(t, int64(2), fs.calls.Load(), "one corrective retry, not the full retry budget")
		_, legacy := fs.lastBody["max_tokens"]
		assert.False(t, legacy, "the rejected parameter must be gone")
		assert.EqualValues(t, 4096, fs.lastBody["max_completion_tokens"], "the cap must survive the switch")
	})

	t.Run("the legacy parameter is used by default so local servers keep working", func(t *testing.T) {
		// given — ollama/LM Studio/llama.cpp generally speak max_tokens only
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, `{"answer":"ok"}`) })
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema, MaxTokens: 4096})

		// then
		require.NoError(t, err)
		assert.EqualValues(t, 4096, fs.lastBody["max_tokens"])
	})
}

func TestReasoningModelMaxTokens(t *testing.T) {
	t.Run("go-openai's client-side refusal is handled without a request", func(t *testing.T) {
		// given — for reasoning models go-openai rejects max_tokens BEFORE
		// sending, so the failure is a bare sentinel and no APIError ever
		// exists. A server-shaped test cannot reach this path, which is why
		// every gpt-5 model failed while the 400-based test passed.
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, `{"answer":"ok"}`) })
		c, err := New(Config{Endpoint: fs.URL + "/v1", Model: "gpt-5-nano", Token: "tok"})
		require.NoError(t, err)
		c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema, MaxTokens: 4096})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"ok"}`), got)
		_, legacy := fs.lastBody["max_tokens"]
		assert.False(t, legacy)
		assert.EqualValues(t, 4096, fs.lastBody["max_completion_tokens"])
	})
}

func TestReasoningEffort(t *testing.T) {
	t.Run("the requested effort is sent", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, `{"answer":"ok"}`) })
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema, ReasoningEffort: "low"})

		// then
		require.NoError(t, err)
		assert.Equal(t, "low", fs.lastBody["reasoning_effort"])
	})

	t.Run("a model that rejects the parameter is retried without it", func(t *testing.T) {
		// given — non-reasoning models 400 on reasoning_effort, and the same
		// config may be pointed at either kind, so the parameter must degrade
		// rather than fail the plan step.
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) {})
		fs.respond = func(w http.ResponseWriter, call int64) {
			if _, present := fs.lastBody["reasoning_effort"]; present {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"Unsupported parameter: 'reasoning_effort' is not supported with this model."}}`))
				return
			}
			respondContent(w, `{"answer":"ok"}`)
		}
		c := newTestClient(t, fs.URL)

		// when
		got, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema, ReasoningEffort: "high"})

		// then
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`{"answer":"ok"}`), got)
		assert.Equal(t, int64(2), fs.calls.Load())
		_, present := fs.lastBody["reasoning_effort"]
		assert.False(t, present)
	})

	t.Run("no effort requested sends no parameter", func(t *testing.T) {
		// given
		fs := newFakeServer(t, func(w http.ResponseWriter, call int64) { respondContent(w, `{"answer":"ok"}`) })
		c := newTestClient(t, fs.URL)

		// when
		_, _, err := c.CompleteJSON(context.Background(), Request{Schema: testSchema})

		// then
		require.NoError(t, err)
		_, present := fs.lastBody["reasoning_effort"]
		assert.False(t, present)
	})
}
