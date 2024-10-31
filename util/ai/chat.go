package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []map[string]string    `json:"messages"`
	Temperature    float32                `json:"temperature"`
	Stream         bool                   `json:"stream"`
	ResponseFormat map[string]interface{} `json:"response_format"`
}

type ChatResponse struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	Created           int64  `json:"created"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type prefixStrippingReader struct {
	reader *bufio.Reader
	prefix string
}

func (psr *prefixStrippingReader) Read(p []byte) (int, error) {
	for {
		line, err := psr.reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		if strings.HasPrefix(line, psr.prefix) {
			line = strings.TrimPrefix(line, psr.prefix)
		}

		// Ignore the "[DONE]" line
		if strings.TrimSpace(line) == "[DONE]" {
			continue
		}

		n := copy(p, line)
		return n, nil
	}
}

func createChatRequest(config APIConfig, prompt PromptConfig) ([]byte, error) {
	payload := ChatRequest{
		Model: config.Model,
		Messages: []map[string]string{
			{
				"role":    "system",
				"content": prompt.SystemPrompt,
			},
			{
				"role":    "user",
				"content": prompt.UserPrompt,
			},
		},
		Temperature: prompt.Temperature,
		Stream:      true,
	}

	if prompt.JSONMode {
		payload.ResponseFormat = map[string]interface{}{
			"type": "json_object",
		}
	}

	return json.Marshal(payload)
}

func sendChatRequest(jsonData []byte, config APIConfig) (*http.Response, error) {
	req, err := http.NewRequest("POST", config.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if config.AuthRequired {
		req.Header.Set("Authorization", "Bearer "+config.AuthToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

func parseChatResponse(body io.Reader) (*[]ChatResponse, error) {
	psr := &prefixStrippingReader{
		reader: bufio.NewReader(body),
		prefix: "data: ",
	}

	decoder := json.NewDecoder(psr)
	responses := make([]ChatResponse, 0)
	for {
		var response ChatResponse
		if err := decoder.Decode(&response); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("error decoding the response: %w", err)
		}

		responses = append(responses, response)
	}

	return &responses, nil
}

func chat(config APIConfig, promptConfig PromptConfig) (*[]ChatResponse, error) {
	jsonData, err := createChatRequest(config, promptConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating the payload: %w", err)
	}

	resp, err := sendChatRequest(jsonData, config)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrEndpointNotReachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}
		bodyString := string(bodyBytes)
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s %w: %s", config.Model, ErrModelNotFound, config.Endpoint)
		} else if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w %s", ErrAuthRequired, config.Endpoint)
		} else {
			return nil, fmt.Errorf("error: received non-200 status code %d: %s", resp.StatusCode, bodyString)
		}
	}

	return parseChatResponse(resp.Body)
}
