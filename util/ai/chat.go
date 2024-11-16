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
	"time"
)

// ChatRequest represents the structure of the request payload for the chat API.
type ChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []map[string]string    `json:"messages"`
	Temperature    float32                `json:"temperature"`
	Stream         bool                   `json:"stream"`
	ResponseFormat map[string]interface{} `json:"response_format"`
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

// ContentResponse represents the structure of the content response for different modes.
type ContentResponse struct {
	Summary                string `json:"summary,omitempty"`
	Corrected              string `json:"corrected,omitempty"`
	Shortened              string `json:"shortened,omitempty"`
	Expanded               string `json:"expanded,omitempty"`
	Bullet                 string `json:"bullet,omitempty"`
	ContentAsTable         string `json:"content_as_table,omitempty"`
	Translation            string `json:"translation,omitempty"`
	CasualContent          string `json:"casual_content,omitempty"`
	FunnyContent           string `json:"funny_content,omitempty"`
	ConfidentContent       string `json:"confident_content,omitempty"`
	StraightforwardContent string `json:"straightforward_content,omitempty"`
	ProfessionalContent    string `json:"professional_content,omitempty"`
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
func (ai *AIService) createChatRequest() ([]byte, error) {
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
		payload.ResponseFormat = map[string]interface{}{
			"type": "json_object",
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

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
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
func (ai *AIService) chat(ctx context.Context) (string, error) {
	jsonData, err := ai.createChatRequest()
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

	return answerBuilder.String(), nil
}

// extractAnswerByMode extracts the relevant content from the JSON response based on the mode.
func (ai *AIService) extractAnswerByMode(jsonData string) (string, error) {
	var response ContentResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	if err != nil {
		return "", fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	modeToContent := map[int]string{
		1:  response.Summary,
		2:  response.Corrected,
		3:  response.Shortened,
		4:  response.Expanded,
		5:  response.Bullet,
		6:  response.ContentAsTable,
		7:  response.CasualContent,
		8:  response.FunnyContent,
		9:  response.ConfidentContent,
		10: response.StraightforwardContent,
		11: response.ProfessionalContent,
		12: response.Translation,
	}

	content, exists := modeToContent[int(ai.promptConfig.Mode)]
	if !exists {
		return "", fmt.Errorf("unknown mode: %d", ai.promptConfig.Mode)
	}
	if content == "" {
		return "", fmt.Errorf("response content is empty")
	}

	return content, nil
}
