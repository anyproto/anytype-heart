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

		line = strings.TrimPrefix(line, psr.prefix)

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

func extractContentByMode(jsonData string, promptConfig PromptConfig) (string, error) {
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

	content, exists := modeToContent[int(promptConfig.Mode)]
	if !exists {
		return "", fmt.Errorf("unknown mode: %d", promptConfig.Mode)
	}
	if content == "" {
		return "", fmt.Errorf("response content is empty")
	}

	return content, nil
}

func chat(config APIConfig, promptConfig PromptConfig) (*[]ChatResponse, error) {
	jsonData, err := createChatRequest(config, promptConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating the payload: %w", err)
	}

	resp, err := sendChatRequest(jsonData, config)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrEndpointNotReachable, err.Error())
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
