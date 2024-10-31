package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/anyproto/anytype-heart/pb"
)

type Model struct {
	Id      string `json:"Id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

func getChatModels(config APIConfig) ([]Model, error) {
	switch config.Provider {
	case pb.RpcAIWritingToolsRequest_OLLAMA:
		resp, err := getModels(config)
		if err != nil {
			return nil, fmt.Errorf("error getting Ollama models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "llama") || strings.Contains(model.Id, "gemma")
		}), nil
	case pb.RpcAIWritingToolsRequest_OPENAI:
		resp, err := getModels(config)
		if err != nil {
			return nil, fmt.Errorf("error getting OpenAI models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "gpt")
		}), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", config.Provider)
	}
}

func getEmbedModels(config APIConfig) ([]Model, error) {
	switch config.Provider {
	case pb.RpcAIWritingToolsRequest_OLLAMA:
		resp, err := getModels(config)
		if err != nil {
			return nil, fmt.Errorf("error getting Ollama models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "embed") || strings.Contains(model.Id, "all-minilm")
		}), nil
	case pb.RpcAIWritingToolsRequest_OPENAI:
		resp, err := getModels(config)
		if err != nil {
			return nil, fmt.Errorf("error getting OpenAI models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "embed")
		}), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", config.Provider)
	}
}

func getModels(config APIConfig) ([]Model, error) {
	parsedURL, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing the URL: %w", err)
	}
	parsedURL.Path = strings.Replace(parsedURL.Path, "chat/completions", "models", 1)

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating the request: %w", err)
	}

	if config.AuthRequired {
		req.Header.Set("Authorization", "Bearer "+config.AuthToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making the request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error: received non-200 status code: %d", resp.StatusCode)
	}

	var modelsResp ModelsResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading the response body: %w", err)
	}

	err = json.Unmarshal(body, &modelsResp)
	if err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	models := make([]Model, 0, len(modelsResp.Data))
	for _, model := range modelsResp.Data {
		models = append(models, model)
	}

	return models, nil
}

func filterModels(models []Model, filterFunc func(model Model) bool) []Model {
	var filteredModels []Model
	for _, model := range models {
		if filterFunc(model) {
			filteredModels = append(filteredModels, model)
		}
	}
	return filteredModels
}
