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
	SearchPoints(ctx context.Context, collection string, vector []float32, limit int) ([]SearchResult, error)
	ScrollByObjectID(ctx context.Context, collection string, objectID string) ([]ScrollPoint, error)
}

type ScrollPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type SearchResult struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
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

func (c *qdrantClient) ScrollByObjectID(ctx context.Context, collection string, objectID string) ([]ScrollPoint, error) {
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
		"with_payload": true,
		"with_vector":  true,
		"limit":        500,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/scroll", collection), body)
	if err != nil {
		return nil, fmt.Errorf("scroll points: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read scroll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scroll points returned %d: %s", resp.StatusCode, string(respBody))
	}

	var scrollResp struct {
		Result struct {
			Points []ScrollPoint `json:"points"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &scrollResp); err != nil {
		return nil, fmt.Errorf("unmarshal scroll response: %w", err)
	}

	return scrollResp.Result.Points, nil
}

func (c *qdrantClient) SearchPoints(ctx context.Context, collection string, vector []float32, limit int) ([]SearchResult, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/search", collection), body)
	if err != nil {
		return nil, fmt.Errorf("search points: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search points returned %d: %s", resp.StatusCode, string(respBody))
	}

	var searchResp struct {
		Result []SearchResult `json:"result"`
	}
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("unmarshal search response: %w", err)
	}

	return searchResp.Result, nil
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
