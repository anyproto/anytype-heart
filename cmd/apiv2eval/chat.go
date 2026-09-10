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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// chatMessage is one message in the conversation.
type chatMessage struct {
	Role string `json:"role"`
	// content is always emitted, never omitted: an assistant message that
	// carries tool_calls and no content field is refused by LM Studio with
	// "Invalid 'content': 'content' field must be a string or an array of
	// objects". That is reachable whenever the model's visible content is
	// empty — which is exactly the salvaged-call case, where the whole
	// message lived in the thinking channel.
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
	// ReasoningContent carries the model's own thinking BACK into the next
	// turn, under the spelling the server used to serve it. Off by default:
	// whether replaying it helps or breaks the chat template is exactly the
	// A/B this field exists to run (-replay-reasoning).
	ReasoningContent string `json:"reasoning_content,omitempty"`
	ToolCallId       string `json:"tool_call_id,omitempty"`
	Name             string `json:"name,omitempty"`
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
	// retryBudget bounds how long a completion keeps retrying an endpoint
	// that could not answer at all. A local model server on a laptop
	// disappears whenever the machine sleeps, and the first dial timeout
	// used to burn the attempt: every remaining cell then recorded an
	// environment failure, so a whole matrix was lost to an outage that
	// outlived a single request. Zero keeps the old fail-fast behaviour,
	// which is what a test pointing at a dead server wants.
	retryBudget time.Duration
	// sleep is the backoff wait, hooked so a test need not spend it.
	sleep func(context.Context, time.Duration) error
	// sampling carries the vendor's recommended decoding knobs. Qwen's own
	// model cards say, verbatim, "DO NOT use greedy decoding, as it can
	// lead to performance degradation and endless repetitions" — and this
	// harness ran every measurement at temperature 0. Zero values are
	// omitted from the request, so an unset knob keeps the server default.
	topP, presencePenalty float64
	topK                  int
	// thinking is a tri-state: "" leaves the server default alone, "off"
	// and "on" send chat_template_kwargs.enable_thinking.
	thinking string
	// captureRaw stores the raw choices[0] bytes on every response, so a
	// turn that returns no tool call and no content can be told apart from
	// one whose tool call the host failed to parse and dropped.
	captureRaw bool
}

func newChatClient(baseURL, apiKey string, timeout time.Duration) *chatClient {
	return &chatClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
		sleep:   sleepCtx,
	}
}

// sleepCtx waits for d unless the context ends first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// chatResponse is what one completion returned.
type chatResponse struct {
	// Raw is the untouched choices[0] object when -capture-raw is on: the
	// only way to tell "the model said nothing" from "the host dropped a
	// tool call it could not parse".
	Raw          json.RawMessage
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
		// a dependent chain breaks when the model emits every call at once:
		// the ids it targets shift as the earlier calls apply, so the later
		// ones address blocks that no longer exist
		req["parallel_tool_calls"] = false
	}
	if c.topP > 0 {
		req["top_p"] = c.topP
	}
	if c.topK > 0 {
		req["top_k"] = c.topK
	}
	if c.presencePenalty != 0 {
		req["presence_penalty"] = c.presencePenalty
	}
	if c.thinking != "" {
		req["chat_template_kwargs"] = map[string]any{"enable_thinking": c.thinking == "on"}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}
	body, err := c.postWithRetry(ctx, "/chat/completions", payload, model)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				chatMessage
				// two spellings, because the field is not in the OpenAI
				// schema and servers disagree: LM Studio and vLLM emit
				// reasoning_content, OpenRouter and Ollama emit reasoning.
				// Reading only one of them made every run blind to the
				// model's thinking while usage still billed reasoning
				// tokens — 194 qwen turns recorded 0 characters of it.
				Reasoning        string `json:"reasoning"`
				ReasoningContent string `json:"reasoning_content"`
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
	var rawChoice json.RawMessage
	if c.captureRaw {
		var envelope struct {
			Choices []json.RawMessage `json:"choices"`
		}
		if json.Unmarshal(body, &envelope) == nil && len(envelope.Choices) > 0 {
			rawChoice = envelope.Choices[0]
		}
	}
	return &chatResponse{
		Raw:          rawChoice,
		Message:      choice.Message.chatMessage,
		Reasoning:    firstNonEmptyString(choice.Message.Reasoning, choice.Message.ReasoningContent),
		FinishReason: choice.FinishReason,
		Usage:        decoded.Usage,
	}, nil
}

// postWithRetry sends payload, retrying while the endpoint cannot answer at
// all — a transport error, or a 5xx/429 the server itself calls temporary.
// A 4xx is the server rejecting THIS call and is returned as it stands:
// retrying it would only collect the same rejection.
func (c *chatClient) postWithRetry(ctx context.Context, path string, payload []byte, model string) ([]byte, error) {
	deadline := time.Now().Add(c.retryBudget)
	backoff := 2 * time.Second
	for {
		body, retryable, err := c.postOnce(ctx, path, payload, model)
		switch {
		case err == nil:
			return body, nil
		case !retryable || ctx.Err() != nil:
			return nil, err
		case !time.Now().Before(deadline):
			// the budget is the whole point of the message: an operator
			// reading "still failing after 20m" knows the endpoint went
			// away for good, not that one request was unlucky
			if c.retryBudget > 0 {
				return nil, fmt.Errorf("%w (still failing after %s of retries)", err, c.retryBudget)
			}
			return nil, err
		}
		if waitErr := c.sleep(ctx, backoff); waitErr != nil {
			return nil, err
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// postOnce sends payload once, reporting whether the failure it returns is
// worth another try.
func (c *chatClient) postOnce(ctx context.Context, path string, payload []byte, model string) ([]byte, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		// a CLIENT timeout is NOT a transient fault worth repeating: the
		// request reached the endpoint, which is still generating — and at
		// temperature 0 a resent request reproduces the same runaway
		// completion, so every retry costs another full timeout and returns
		// the same nothing. Measured: 12b attempts that answered took
		// 21-30s, while the ones that ran away burned the whole 20m budget
		// four timeouts at a time. Only a connection that could not be
		// established is worth trying again.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, false, fmt.Errorf("call chat completions for %s: %w", model, err)
		}
		return nil, true, fmt.Errorf("call chat completions for %s: %w", model, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, true, fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("chat completions for %s answered %d: %s", model, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false, fmt.Errorf("chat completions for %s answered %d: %s", model, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, false, nil
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

// firstNonEmptyString returns the first non-empty argument.
func firstNonEmptyString(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
