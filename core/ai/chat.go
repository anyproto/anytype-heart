package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChatRequest represents the structure of the request payload for the chat API.
type ChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []map[string]string    `json:"messages"`
	Temperature    float32                `json:"temperature,omitempty"`
	Stream         bool                   `json:"stream"`
	ResponseFormat map[string]interface{} `json:"response_format,omitempty"`
}

// ChatResponse represents the structure of the response from the chat API.
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

// prefixStrippingReader is a custom reader that strips a specific prefix from each line.
type prefixStrippingReader struct {
	reader *bufio.Reader
	prefix string
}

// Read reads data from the underlying reader and strips the specified prefix.
func (psr *prefixStrippingReader) Read(p []byte) (int, error) {
	for {
		line, err := psr.reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		line = strings.TrimPrefix(line, psr.prefix)

		// Ignore the "[DONE]" line
		if strings.TrimSpace(line) == "[DONE]" {
			continue
		}

		n := copy(p, line)
		return n, nil
	}
}

// createChatRequest creates the JSON payload for the chat API request.
func (ai *AIService) createChatRequest(mode int, promptConfig *PromptConfig) ([]byte, error) {
	payload := ChatRequest{
		Model: ai.apiConfig.Model,
		Messages: []map[string]string{
			{
				"role":    "system",
				"content": promptConfig.SystemPrompt,
			},
			{
				"role":    "user",
				"content": promptConfig.UserPrompt,
			},
		},
		Temperature: promptConfig.Temperature,
		Stream:      true,
	}

	if promptConfig.JSONMode {
		key, exists := ai.responseParser.ModeToField()[mode]
		if !exists {
			return nil, fmt.Errorf("unknown mode: %d", mode)
		}

		schemaFunc, exists := ai.responseParser.ModeToSchema()[mode]
		if !exists {
			return nil, fmt.Errorf("no schema function defined for mode: %d", mode)
		}

		payload.ResponseFormat = schemaFunc(key)
	}

	return json.Marshal(payload)
}

// sendChatRequest sends the chat API request and returns the response.
func (ai *AIService) sendChatRequest(ctx context.Context, jsonData []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", ai.apiConfig.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if ai.apiConfig.AuthRequired {
		req.Header.Set("Authorization", "Bearer "+ai.apiConfig.AuthToken)
	}

	return ai.httpClient.Do(req)
}

// parseChatResponse parses the chat API response and returns the chat response chunks.
func (ai *AIService) parseChatResponse(body io.Reader) (*[]ChatResponse, error) {
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

// chat sends a chat request and returns the parsed chat response as a string.
func (ai *AIService) chat(ctx context.Context, mode int, promptConfig *PromptConfig) (string, error) {
	jsonData, err := ai.createChatRequest(mode, promptConfig)
	if err != nil {
		return "", fmt.Errorf("error creating the payload: %w", err)
	}

	resp, err := ai.sendChatRequest(ctx, jsonData)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrEndpointNotReachable, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("error reading response body: %w", err)
		}
		bodyString := string(bodyBytes)
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%s %w: %s", ai.apiConfig.Model, ErrModelNotFound, ai.apiConfig.Endpoint)
		} else if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("%w %s", ErrAuthRequired, ai.apiConfig.Endpoint)
		} else if resp.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("%w %s", ErrRateLimitExceeded, ai.apiConfig.Endpoint)
		} else {
			return "", fmt.Errorf("error: received non-200 status code %d: %s", resp.StatusCode, bodyString)
		}
	}

	answerChunks, err := ai.parseChatResponse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error parsing the response: %w", err)
	}

	// build answer string from answer chunks
	var answerBuilder strings.Builder
	for _, chunk := range *answerChunks {
		for _, choice := range chunk.Choices {
			answerBuilder.WriteString(choice.Delta.Content)
		}
	}

	log.Info("chat response: ", answerBuilder.String())
	return answerBuilder.String(), nil
}
