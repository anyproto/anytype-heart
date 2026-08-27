package main

// chat.go — the OpenAI-compatible chat client used to drive the local
// models (Ollama/LM Studio-class hosts serve this shape at /v1). Nothing
// here is Anytype-specific: it carries messages, tool definitions and the
// usage numbers back, and normalizes the two encodings servers use for tool
// arguments (JSON string, or an already-decoded object).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// chatMessage is one message in the conversation.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallId string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// toolCall is one function call the model emitted. Arguments stay RAW: the
// whole point of the run is what the model actually wrote, so the record
// keeps the bytes and the classifiers decode a copy.
type toolCall struct {
	Id       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// argsJSON returns the call's arguments as a JSON object, accepting both
// wire encodings: a JSON-encoded STRING (OpenAI's own shape, and Ollama's)
// or an inline object (some servers).
func (c toolCall) argsJSON() ([]byte, error) {
	raw := bytes.TrimSpace(c.Function.Arguments)
	if len(raw) == 0 {
		return []byte("{}"), nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("decode tool arguments string: %w", err)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return []byte("{}"), nil
		}
		return []byte(s), nil
	}
	return raw, nil
}

// argsMap decodes the arguments into the map the tool executors take.
func (c toolCall) argsMap() (map[string]any, error) {
	raw, err := c.argsJSON()
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode tool arguments object: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// toolDef is one function-calling tool definition.
type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func newToolDef(name, description string, parameters json.RawMessage) toolDef {
	var t toolDef
	t.Type = "function"
	t.Function.Name = name
	t.Function.Description = description
	t.Function.Parameters = parameters
	return t
}

// usage is one completion's token accounting.
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatClient talks to one OpenAI-compatible endpoint.
type chatClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newChatClient(baseURL, apiKey string, timeout time.Duration) *chatClient {
	return &chatClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
	}
}

// chatResponse is what one completion returned.
type chatResponse struct {
	Message      chatMessage
	Reasoning    string
	FinishReason string
	Usage        usage
}

// complete runs one chat completion.
func (c *chatClient) complete(ctx context.Context, model string, messages []chatMessage, tools []toolDef, temperature float64) (*chatResponse, error) {
	req := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": temperature,
		"stream":      false,
	}
	if len(tools) > 0 {
		req["tools"] = tools
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call chat completions for %s: %w", model, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("chat completions for %s answered %d: %s", model, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				chatMessage
				Reasoning string `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("chat completions for %s returned no choices", model)
	}
	choice := decoded.Choices[0]
	return &chatResponse{
		Message:      choice.Message.chatMessage,
		Reasoning:    choice.Message.Reasoning,
		FinishReason: choice.FinishReason,
		Usage:        decoded.Usage,
	}, nil
}

// listModels returns the ids the endpoint serves — the preflight check that
// a requested model is actually pulled.
func (c *chatClient) listModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build models request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("models endpoint answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Data []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	ids := make([]string, 0, len(decoded.Data))
	for _, m := range decoded.Data {
		ids = append(ids, m.Id)
	}
	return ids, nil
}
