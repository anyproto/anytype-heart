package llmclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newModelsServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestListModels(t *testing.T) {
	t.Run("healthy list response", func(t *testing.T) {
		// given
		srv := newModelsServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/models", r.URL.Path)
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object": "list",
				"data": [
					{"id": "gpt-4o", "object": "model", "created": 1, "owned_by": "openai"},
					{"id": "text-embedding-3-small", "object": "model", "created": 1, "owned_by": "openai"}
				]
			}`))
		})
		want := []Model{
			{ID: "gpt-4o", OwnedBy: "openai"},
			{ID: "text-embedding-3-small", OwnedBy: "openai"},
		}

		// when
		got, err := ListModels(context.Background(), Config{Endpoint: srv.URL + "/v1", Token: "tok"})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("401 fails as auth required", func(t *testing.T) {
		// given
		srv := newModelsServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
		})

		// when
		_, err := ListModels(context.Background(), Config{Endpoint: srv.URL + "/v1", Token: "bad-tok"})

		// then
		require.ErrorIs(t, err, ErrAuthRequired)
	})

	t.Run("unreachable endpoint", func(t *testing.T) {
		// given — a server that is closed before the call, so the dial fails
		srv := newModelsServer(t, func(w http.ResponseWriter, r *http.Request) {})
		url := srv.URL
		srv.Close()

		// when
		_, err := ListModels(context.Background(), Config{Endpoint: url + "/v1", Token: "tok"})

		// then
		require.ErrorIs(t, err, ErrEndpointUnreachable)
	})

	t.Run("malformed response body", func(t *testing.T) {
		// given — 200 OK but not JSON at all: an endpoint that isn't actually
		// OpenAI-compatible, e.g. a plain web server or a wrong path
		srv := newModelsServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html>not json</html>`))
		})

		// when
		_, err := ListModels(context.Background(), Config{Endpoint: srv.URL + "/v1", Token: "tok"})

		// then — no HTTP status to classify by, so this is reported the same
		// way a transport failure is: the endpoint doesn't behave like an
		// OpenAI-compatible server.
		require.ErrorIs(t, err, ErrEndpointUnreachable)
	})

	t.Run("empty catalog is not an error", func(t *testing.T) {
		// given
		srv := newModelsServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		})

		// when
		got, err := ListModels(context.Background(), Config{Endpoint: srv.URL + "/v1", Token: "tok"})

		// then
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("endpoint is required", func(t *testing.T) {
		_, err := ListModels(context.Background(), Config{})
		require.Error(t, err)
	})
}

func TestFilterChatModels(t *testing.T) {
	// given — a mixed OpenAI-style catalog: chat models alongside every
	// non-chat family the heuristic is documented to exclude
	models := []Model{
		{ID: "gpt-4o"},
		{ID: "gpt-4o-mini"},
		{ID: "gpt-4.1"},
		{ID: "gpt-5"},
		{ID: "gpt-5-nano"},
		{ID: "chatgpt-4o-latest"},
		{ID: "gpt-3.5-turbo"},
		{ID: "o1"},
		{ID: "o3-mini"},

		{ID: "text-embedding-3-small"},
		{ID: "text-embedding-3-large"},
		{ID: "text-embedding-ada-002"},
		{ID: "whisper-1"},
		{ID: "gpt-4o-transcribe"},
		{ID: "tts-1"},
		{ID: "tts-1-hd"},
		{ID: "gpt-4o-mini-tts"},
		{ID: "gpt-4o-audio-preview"},
		{ID: "gpt-4o-realtime-preview"},
		{ID: "dall-e-2"},
		{ID: "dall-e-3"},
		{ID: "gpt-image-1"},
		{ID: "omni-moderation-latest"},
		{ID: "text-moderation-stable"},
		{ID: "davinci-002"},
		{ID: "babbage-002"},
		{ID: "gpt-3.5-turbo-instruct"},
		{ID: "computer-use-preview"},
	}
	want := []Model{
		{ID: "gpt-4o"},
		{ID: "gpt-4o-mini"},
		{ID: "gpt-4.1"},
		{ID: "gpt-5"},
		{ID: "gpt-5-nano"},
		{ID: "chatgpt-4o-latest"},
		{ID: "gpt-3.5-turbo"},
		{ID: "o1"},
		{ID: "o3-mini"},
	}

	// when
	got := FilterChatModels(models)

	// then
	assert.Equal(t, want, got)
}

func TestFilterChatModelsEmpty(t *testing.T) {
	assert.Empty(t, FilterChatModels(nil))
	assert.Empty(t, FilterChatModels([]Model{}))
}
