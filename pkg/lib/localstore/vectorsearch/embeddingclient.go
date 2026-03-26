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

type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type openaiEmbeddingClient struct {
	httpClient *http.Client
	apiURL     string
	apiKey     string
	model      string
	dimensions int
}

func NewOpenAIEmbeddingClient(apiKey, model string, dimensions int) EmbeddingClient {
	return &openaiEmbeddingClient{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		apiURL:     defaultOpenAIEmbeddingsURL,
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
	}
}

const (
	defaultOpenAIEmbeddingsURL = "https://api.openai.com/v1/embeddings"
	maxBatchSize               = 2048
)

type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *openaiError    `json:"error,omitempty"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (c *openaiEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result := make([][]float32, len(texts))

	for start := 0; start < len(texts); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		embeddings, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch starting at %d: %w", start, err)
		}
		for i, emb := range embeddings {
			result[start+i] = emb
		}
	}

	return result, nil
}

func (c *openaiEmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := embeddingRequest{
		Input:      texts,
		Model:      c.model,
		Dimensions: c.dimensions,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("http request: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			// Recreate request for retry since body was consumed
			req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("create retry request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
			continue
		}
		break
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embeddings API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if embResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", embResp.Error.Message)
	}

	result := make([][]float32, len(texts))
	for _, d := range embResp.Data {
		if d.Index < len(result) {
			result[d.Index] = d.Embedding
		}
	}
	return result, nil
}
