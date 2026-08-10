package main

// agent.go — the agent loop: the same shape a real host runs. System
// prompt (the arm's own instructions — for the wrapper arm those are the
// product's MCP initialize instructions, not text the harness invented),
// the task as the user turn, then call → execute → feed the result back
// until the model stops calling tools or the turn budget runs out.
//
// Nothing here judges success: whether the document actually says what it
// should is decided afterwards, against the API, by the task's own check.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// turnRecord is one model completion and the tool calls it produced.
type turnRecord struct {
	Index        int          `json:"index"`
	Reasoning    string       `json:"reasoning,omitempty"`
	Content      string       `json:"content,omitempty"`
	FinishReason string       `json:"finish_reason,omitempty"`
	Usage        usage        `json:"usage"`
	LatencyMs    int64        `json:"latency_ms"`
	Calls        []callRecord `json:"calls,omitempty"`
}

// callRecord is one tool call exactly as the model emitted it, with what
// came back.
type callRecord struct {
	Turn int             `json:"turn"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
	// ArgsError is set when the emitted arguments were not a JSON object —
	// a malformed call never reaches the tool, and that is its own failure
	// mode worth counting separately from a refusal.
	ArgsError  string `json:"args_error,omitempty"`
	ResultText string `json:"result_text"`
	IsError    bool   `json:"is_error,omitempty"`
	// Exchanges are the HTTP calls this tool call made — the structured
	// (status, code, issue path) facts behind the text the model saw.
	Exchanges []exchange `json:"exchanges,omitempty"`
}

// transcript is one agent run.
type transcript struct {
	Turns            []turnRecord `json:"turns"`
	Calls            []callRecord `json:"calls"`
	FinalContent     string       `json:"final_content,omitempty"`
	StoppedBy        string       `json:"stopped_by"` // model_done | turn_budget
	PromptTokens     int          `json:"prompt_tokens"`
	CompletionTokens int          `json:"completion_tokens"`
}

// errEnvironment marks a failure that is NOT the model's and NOT the API's
// contract: the model host timed out, the API went away mid-run. Attempts
// ending this way are recorded and excluded from the success rate.
var errEnvironment = errors.New("environment failure")

// maxResultChars bounds one tool result fed back to the model. A full
// document read of a fixture is far under this; the bound only stops a
// pathological result from blowing the context window, and when it fires
// the record says so rather than pretending the model saw everything.
const maxResultChars = 24000

// agentConfig is what one run needs beyond its toolset.
type agentConfig struct {
	chat        *chatClient
	model       string
	temperature float64
	maxTurns    int
	// rec attributes HTTP exchanges to the tool call that made them.
	rec *recorder
}

// runAgent drives one attempt's conversation.
func runAgent(ctx context.Context, cfg agentConfig, ts toolset, systemPrompt, userPrompt string) (*transcript, error) {
	tools := make([]toolDef, 0, len(ts.tools()))
	for _, spec := range ts.tools() {
		tools = append(tools, newToolDef(spec.Name, spec.Description, spec.Parameters))
	}
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	tr := &transcript{StoppedBy: "turn_budget"}
	for i := 0; i < cfg.maxTurns; i++ {
		start := time.Now()
		resp, err := cfg.chat.complete(ctx, cfg.model, messages, tools, cfg.temperature)
		if err != nil {
			return tr, fmt.Errorf("%w: turn %d: %w", errEnvironment, i, err)
		}
		turn := turnRecord{
			Index:        i,
			Reasoning:    resp.Reasoning,
			Content:      resp.Message.Content,
			FinishReason: resp.FinishReason,
			Usage:        resp.Usage,
			LatencyMs:    time.Since(start).Milliseconds(),
		}
		tr.PromptTokens += resp.Usage.PromptTokens
		tr.CompletionTokens += resp.Usage.CompletionTokens

		if len(resp.Message.ToolCalls) == 0 {
			turn.FinishReason = resp.FinishReason
			tr.Turns = append(tr.Turns, turn)
			tr.FinalContent = resp.Message.Content
			tr.StoppedBy = "model_done"
			return tr, nil
		}

		assistant := chatMessage{Role: "assistant", Content: resp.Message.Content, ToolCalls: resp.Message.ToolCalls}
		messages = append(messages, assistant)
		for _, call := range resp.Message.ToolCalls {
			rec := callRecord{Turn: i, Tool: call.Function.Name}
			raw, argsErr := call.argsJSON()
			if argsErr == nil {
				rec.Args = json.RawMessage(raw)
			} else {
				rec.Args = json.RawMessage(`null`)
			}
			args, err := call.argsMap()
			if err != nil {
				rec.ArgsError = err.Error()
				rec.IsError = true
				rec.ResultText = fmt.Sprintf("the arguments were not a JSON object: %v", err)
			} else {
				mark := 0
				if cfg.rec != nil {
					mark = cfg.rec.mark()
				}
				outcome := ts.call(ctx, call.Function.Name, args)
				rec.ResultText = outcome.Text
				rec.IsError = outcome.IsError
				if cfg.rec != nil {
					rec.Exchanges = cfg.rec.since(mark)
				}
			}
			turn.Calls = append(turn.Calls, rec)
			tr.Calls = append(tr.Calls, rec)
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallId: call.Id,
				Name:       call.Function.Name,
				Content:    clampResult(rec.ResultText),
			})
		}
		tr.Turns = append(tr.Turns, turn)
		if err := ctx.Err(); err != nil {
			return tr, fmt.Errorf("%w: %w", errEnvironment, err)
		}
	}
	return tr, nil
}

// clampResult bounds one tool result, marking the cut so the transcript
// never implies the model saw more than it did.
func clampResult(s string) string {
	if len(s) <= maxResultChars {
		return s
	}
	cut := maxResultChars
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n… [truncated by the harness]"
}

// summarizeCalls renders the tool calls of an attempt as one line each —
// the readable form used in the failure quotes.
func summarizeCalls(calls []callRecord) string {
	var b strings.Builder
	for _, c := range calls {
		status := "ok"
		if c.IsError {
			status = "ERR"
		}
		fmt.Fprintf(&b, "  t%d %s %s → %s %s\n", c.Turn, c.Tool, string(c.Args), status, firstLine(c.ResultText))
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
