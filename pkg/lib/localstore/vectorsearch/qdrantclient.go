package vectorsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type QdrantClient interface {
	EnsureCollection(ctx context.Context, name string, dimension int) error
	UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error
	DeletePointsByObjectID(ctx context.Context, collection string, objectID string) error
}

type QdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type qdrantClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewQdrantClient(baseURL string) QdrantClient {
	return &qdrantClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

func (c *qdrantClient) EnsureCollection(ctx context.Context, name string, dimension int) error {
	body := map[string]any{
		"vectors": map[string]any{
			"size":     dimension,
			"distance": "Cosine",
		},
	}

	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s", name), body)
	if err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}
	defer resp.Body.Close()

	// 200 = created, 409 = already exists — both are fine
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("ensure collection returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *qdrantClient) UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error {
	if len(points) == 0 {
		return nil
	}

	body := map[string]any{
		"points": points,
	}

	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points", collection), body)
	if err != nil {
		return fmt.Errorf("upsert points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert points returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *qdrantClient) DeletePointsByObjectID(ctx context.Context, collection string, objectID string) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{
					"key": "object_id",
					"match": map[string]any{
						"value": objectID,
					},
				},
			},
		},
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/delete", collection), body)
	if err != nil {
		return fmt.Errorf("delete points by object id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete points returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *qdrantClient) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}
