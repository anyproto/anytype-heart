package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Model struct {
	Id       string `json:"id"`
	Object   string `json:"object"`
	created  string `json:"created"`
	owned_by string `json:"owned_by"`
}

type Response struct {
	Models []Model `json:"models"`
}

func getChatModels(config APIConfig) (Response, error) {
	switch config.Provider {
	case ProviderOllama:
		resp, err := getModels(config)
		if err != nil {
			return Response{}, fmt.Errorf("error getting Ollama models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "llama") || strings.Contains(model.Id, "gemma")
		}), nil
	case ProviderOpenAI:
		resp, err := getModels(config)
		if err != nil {
			return Response{}, fmt.Errorf("error getting OpenAI models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "gpt")
		}), nil
	default:
		return Response{}, fmt.Errorf("unknown provider: %s", config.Provider)
	}
}

func getEmbedModels(config APIConfig) (Response, error) {
	switch config.Provider {
	case ProviderOllama:
		resp, err := getModels(config)
		if err != nil {
			return Response{}, fmt.Errorf("error getting Ollama models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "embed") || strings.Contains(model.Id, "all-minilm")
		}), nil
	case ProviderOpenAI:
		resp, err := getModels(config)
		if err != nil {
			return Response{}, fmt.Errorf("error getting OpenAI models: %w", err)
		}
		return filterModels(resp, func(model Model) bool {
			return strings.Contains(model.Id, "embed")
		}), nil
	default:
		return Response{}, fmt.Errorf("unknown provider: %s", config.Provider)
	}
}

func getModels(config APIConfig) (Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", config.EndpointModels, nil)
	if err != nil {
		return Response{}, fmt.Errorf("error creating the request: %w", err)
	}

	if config.AuthRequired {
		req.Header.Set("Authorization", "Bearer "+config.AuthToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("error making the request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("error: received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("error reading the response body: %w", err)
	}

	// TODO data prefix stripping reader
	var apiResponse struct {
		Data []struct {
			Id       string `json:"id"`
			object   string `json:"object"`
			created  string `json:"created"`
			owned_by string `json:"owned_by"`
		} `json:"data"`
	}

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		return Response{}, fmt.Errorf("error parsing JSON: %w", err)
	}

	var models []Model
	for _, data := range apiResponse.Data {
		models = append(models, Model{
			Id:       data.Id,
			Object:   data.object,
			created:  data.created,
			owned_by: data.owned_by,
		})
	}

	return Response{Models: models}, nil
}

func filterModels(response Response, filterFunc func(model Model) bool) Response {
	var filteredModels []Model
	for _, model := range response.Models {
		if filterFunc(model) {
			filteredModels = append(filteredModels, model)
		}
	}
	return Response{Models: filteredModels}
}
