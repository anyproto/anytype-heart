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

	"github.com/anyproto/anytype-heart/core/ai/parsing"
)

// ChatRequest represents the structure of the request payload for the chat API.
type ChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []map[string]string    `json:"messages"`
	Temperature    float32                `json:"temperature"`
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
func (ai *AIService) createChatRequest(mode int) ([]byte, error) {
	payload := ChatRequest{
		Model: ai.apiConfig.Model,
		Messages: []map[string]string{
			{
				"role":    "system",
				"content": ai.promptConfig.SystemPrompt,
			},
			{
				"role":    "user",
				"content": ai.promptConfig.UserPrompt,
			},
		},
		Temperature: ai.promptConfig.Temperature,
		Stream:      true,
	}

	if ai.promptConfig.JSONMode {
		key, exists := ai.responseParser.ModeToField()[mode]
		if !exists {
			return nil, fmt.Errorf("unknown mode: %d", mode)
		}

		payload.ResponseFormat = map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   key + "_response",
				"strict": true,
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						key: map[string]interface{}{
							"type": "string",
						},
					},
					"additionalProperties": false,
					"required":             []string{key},
				},
			},
		}
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
func (ai *AIService) chat(ctx context.Context, mode int) (string, error) {
	jsonData, err := ai.createChatRequest(mode)
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

// extractAnswerByMode extracts the relevant content from the JSON response based on the mode.
func (ai *AIService) extractAnswerByMode(jsonData string, mode int) (string, error) {
	respStruct := ai.responseParser.NewResponseStruct()

	err := json.Unmarshal([]byte(jsonData), &respStruct)
	if err != nil {
		return "", fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	return ai.responseParser.ExtractContent(mode, respStruct)
}

// newChat should be used for concurrent chat requests to avoid conflicts with global ai.promptConfig.
func (ai *AIService) newChat(ctx context.Context, mode int, pc *PromptConfig, rp parsing.ResponseParser) (string, error) {
	payload := ChatRequest{
		Model: ai.apiConfig.Model,
		Messages: []map[string]string{
			{"role": "system", "content": pc.SystemPrompt},
			{"role": "user", "content": pc.UserPrompt},
		},
		Temperature: pc.Temperature,
		Stream:      true,
	}

	if pc.JSONMode {
		key, exists := rp.ModeToField()[mode]
		if !exists {
			return "", fmt.Errorf("unknown mode: %d", mode)
		}
		payload.ResponseFormat = map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   key + "_response",
				"strict": true,
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						key: map[string]interface{}{
							"type": "string",
						},
					},
					"additionalProperties": false,
					"required":             []string{key},
				},
			},
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := ai.sendChatRequest(ctx, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error: received non-200 status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	answerChunks, err := ai.parseChatResponse(resp.Body)
	if err != nil {
		return "", err
	}

	var answerBuilder strings.Builder
	for _, chunk := range *answerChunks {
		for _, choice := range chunk.Choices {
			answerBuilder.WriteString(choice.Delta.Content)
		}
	}
	return answerBuilder.String(), nil
}
