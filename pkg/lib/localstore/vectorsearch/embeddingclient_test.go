package vectorsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIEmbeddingClient_Embed(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req embeddingRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "test-model", req.Model)
			assert.Equal(t, 0, req.Dimensions) // dimensions not sent
			assert.Equal(t, []string{"hello", "world"}, req.Input)

			resp := embeddingResponse{
				Data: []embeddingData{
					{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
					{Index: 1, Embedding: []float32{0.4, 0.5, 0.6}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &openaiEmbeddingClient{
			httpClient: server.Client(),
			apiURL:     server.URL,
			apiKey:     "test-key",
			model:      "test-model",
		}

		result, err := client.Embed(context.Background(), []string{"hello", "world"})
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, result[0])
		assert.Equal(t, []float32{0.4, 0.5, 0.6}, result[1])
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		client := NewEmbeddingClient("https://unused", "key", "model")
		result, err := client.Embed(context.Background(), nil)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("API error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": {"message": "invalid api key", "type": "auth_error"}}`))
		}))
		defer server.Close()

		client := &openaiEmbeddingClient{
			httpClient: server.Client(),
			apiURL:     server.URL,
			apiKey:     "bad-key",
			model:      "model",
		}
		_, err := client.Embed(context.Background(), []string{"test"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}
