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

func TestQdrantClient_EnsureCollection(t *testing.T) {
	t.Run("creates collection successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "/collections/anytype_space1", r.URL.Path)

			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			vectors := body["vectors"].(map[string]any)
			assert.Equal(t, float64(1536), vectors["size"])
			assert.Equal(t, "Cosine", vectors["distance"])

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewQdrantClient(server.URL)
		err := client.EnsureCollection(context.Background(), "anytype_space1", 1536)
		require.NoError(t, err)
	})

	t.Run("409 conflict is ok", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()

		client := NewQdrantClient(server.URL)
		err := client.EnsureCollection(context.Background(), "anytype_space1", 1536)
		require.NoError(t, err)
	})
}

func TestQdrantClient_UpsertPoints(t *testing.T) {
	t.Run("upserts points successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "/collections/anytype_space1/points", r.URL.Path)

			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			points := body["points"].([]any)
			assert.Len(t, points, 1)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewQdrantClient(server.URL)
		err := client.UpsertPoints(context.Background(), "anytype_space1", []QdrantPoint{
			{
				ID:     "obj1/vs/0",
				Vector: []float32{0.1, 0.2},
				Payload: map[string]any{
					"object_id": "obj1",
					"text":      "hello",
				},
			},
		})
		require.NoError(t, err)
	})

	t.Run("empty points is no-op", func(t *testing.T) {
		client := NewQdrantClient("http://unused")
		err := client.UpsertPoints(context.Background(), "col", nil)
		require.NoError(t, err)
	})
}

func TestQdrantClient_DeletePointsByObjectID(t *testing.T) {
	t.Run("deletes by object_id filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/collections/anytype_space1/points/delete", r.URL.Path)

			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			filter := body["filter"].(map[string]any)
			must := filter["must"].([]any)
			assert.Len(t, must, 1)
			cond := must[0].(map[string]any)
			assert.Equal(t, "object_id", cond["key"])

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewQdrantClient(server.URL)
		err := client.DeletePointsByObjectID(context.Background(), "anytype_space1", "obj1")
		require.NoError(t, err)
	})
}
