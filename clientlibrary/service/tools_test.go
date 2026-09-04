package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// withToolsHost stands in a host provider for one test.
func withToolsHost(t *testing.T, provider func() (*wrapper.Host, error)) {
	prev := toolsHostProvider
	toolsHostProvider = provider
	t.Cleanup(func() { toolsHostProvider = prev })
}

// stubHost builds a host over a tiny server that answers GET /v2/spaces.
func stubHost(t *testing.T) *wrapper.Host {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/spaces" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"space1","name":"Work"}],"total":1,"has_more":false}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"status":404,"code":"not_found","message":"no such route","issues":[]}`)
	}))
	t.Cleanup(srv.Close)
	client := wrapper.NewClient(srv.URL, "k")
	client.Backoff = func(int) time.Duration { return 0 }
	return wrapper.NewHost(client)
}

// recordingHandler receives the one envelope ToolsCall delivers.
type recordingHandler struct{ ch chan []byte }

func newRecordingHandler() *recordingHandler { return &recordingHandler{ch: make(chan []byte, 1)} }

func (r *recordingHandler) Handle(b []byte) { r.ch <- b }

func (r *recordingHandler) await(t *testing.T) toolsEnvelope {
	t.Helper()
	select {
	case b := <-r.ch:
		var env toolsEnvelope
		require.NoError(t, json.Unmarshal(b, &env))
		return env
	case <-time.After(5 * time.Second):
		t.Fatal("no envelope delivered")
		return toolsEnvelope{}
	}
}

func decodeEnvelope(t *testing.T, b []byte) toolsEnvelope {
	t.Helper()
	var env toolsEnvelope
	require.NoError(t, json.Unmarshal(b, &env))
	return env
}

func TestToolsManifest(t *testing.T) {
	for _, tier := range []wrapper.Tier{wrapper.TierSmall, wrapper.TierLarge} {
		t.Run(string(tier), func(t *testing.T) {
			// when
			var env struct {
				JSON    wrapper.Manifest `json:"json"`
				IsError bool             `json:"is_error"`
			}
			require.NoError(t, json.Unmarshal(ToolsManifest(string(tier)), &env))

			// then
			assert.False(t, env.IsError)
			var names []string
			for _, tool := range env.JSON.Tools {
				names = append(names, tool.Name)
			}
			assert.Equal(t, wrapper.ToolNamesForTier(tier), names)
		})
	}

	t.Run("unknown tier is bad_request", func(t *testing.T) {
		env := decodeEnvelope(t, ToolsManifest("huge"))
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeBadRequest, env.Code)
		assert.Contains(t, env.Text, `unknown tier "huge"`)
	})
}

func TestToolsCall(t *testing.T) {
	t.Run("no account delivers account_not_running", func(t *testing.T) {
		// given: the default provider and no account app
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeAccountNotRunning, env.Code)
	})

	t.Run("malformed args are bad_request before the host is consulted", func(t *testing.T) {
		// given
		withToolsHost(t, func() (*wrapper.Host, error) {
			t.Error("the host must not be consulted")
			return nil, nil
		})
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", []byte(`[1,2]`), h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeBadRequest, env.Code)
		assert.Contains(t, env.Text, "must be a JSON object")
	})

	t.Run("round trip through a host", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", []byte(`{}`), h)

		// then
		env := h.await(t)
		assert.False(t, env.IsError, env.Text)
		assert.Contains(t, env.Text, "Work — space1")
		assert.NotNil(t, env.JSON)
	})

	t.Run("empty args mean no args", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		assert.False(t, h.await(t).IsError)
	})

	t.Run("a tool error is in-band", func(t *testing.T) {
		// given: find without its space
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("find", []byte(`{"query":"x"}`), h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, wrapper.CallCodeToolError, env.Code)
		assert.Contains(t, env.Text, `find needs "space"`)
	})

	t.Run("a panic delivers internal and reaches PanicHandler", func(t *testing.T) {
		// given
		withToolsHost(t, func() (*wrapper.Host, error) { panic("boom") })
		var recovered any
		prev := PanicHandler
		PanicHandler = func(v any) { recovered = v }
		t.Cleanup(func() { PanicHandler = prev })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeInternal, env.Code)
		assert.Equal(t, "boom", recovered)
	})
}

func TestToolsResetSession(t *testing.T) {
	t.Run("no account delivers account_not_running", func(t *testing.T) {
		env := decodeEnvelope(t, ToolsResetSession())
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeAccountNotRunning, env.Code)
	})

	t.Run("resets through the host", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })

		// when
		env := decodeEnvelope(t, ToolsResetSession())

		// then
		assert.False(t, env.IsError)
		assert.Equal(t, "session reset", env.Text)
	})
}
