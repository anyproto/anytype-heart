package main

// salvage.go — recovering tool calls the HOST dropped.
//
// LM Studio's MLX path does not always attach a tool parser for Qwen 3.5/3.6
// class models (mlx-lm#1293), and the model emits its call inside the
// thinking channel; the reasoning parser then consumes the whole span and
// `tool_calls` arrives empty with finish_reason "stop". Measured on this
// harness: 5 of 5 turns that parsed no tool call contained a complete,
// well-formed <tool_call> block the model had written correctly.
//
// Scoring those as the model giving up measures the host, not the API
// surface under test — so the harness reads them back. This is deliberately
// a LAST resort: it runs only when the server returned no tool_calls at all.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// the Hermes-style XML these models fall back to when the tool channel
	// is not wired up: <tool_call><function=NAME><parameter=K>V</parameter>…
	salvageCallRe  = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)
	salvageFuncRe  = regexp.MustCompile(`(?s)<function\s*=\s*([A-Za-z0-9_.-]+)\s*>(.*?)(?:</function>|\z)`)
	salvageParamRe = regexp.MustCompile(`(?s)<parameter\s*=\s*([A-Za-z0-9_.-]+)\s*>(.*?)(?:</parameter>|\z)`)
)

// reconsiderRe marks a trace where the model CHANGED ITS MIND after writing
// a call. Reasoning is where a model deliberates, so a <tool_call> found
// there is not automatically the one it meant to run: LM Studio's own
// parser scanned think blocks and reported firing on calls the model was
// merely discussing (lmstudio-bug-tracker#1592), and Qwen measured think
// blocks containing "speculative reasoning about hypothetical results"
// rather than genuine planning (QwenLM/Qwen3#1817). Executing a rejected
// call is worse than losing it — a wasted turn is recoverable, an
// unintended write is not.
var reconsiderRe = regexp.MustCompile(`(?i)\b(instead|actually|wait|on second thought|no, I should|let me reconsider|but first)\b`)

// salvageable reports whether a response is the SHAPE a dropped call takes,
// rather than a model that reasoned about tools and then answered in prose.
// All three must hold, and all three held in every captured drop: the host
// parsed no call, the model said nothing to the user, and it stopped rather
// than yielding the tool channel.
func salvageable(resp *chatResponse) bool {
	return len(resp.Message.ToolCalls) == 0 &&
		strings.TrimSpace(resp.Message.Content) == "" &&
		resp.FinishReason == "stop"
}

// salvageToolCalls extracts tool calls written as XML in text the server did
// not parse. It returns nil when the text holds none, so a caller can tell
// "nothing to recover" from "recovered nothing".
//
// Only the LAST block in a span is taken, and only when nothing after it
// reads as reconsideration: a trace that writes a call and then argues
// itself out of it must not be executed.
func salvageToolCalls(texts ...string) []toolCall {
	var out []toolCall
	for _, text := range texts {
		blocks := salvageCallRe.FindAllStringSubmatchIndex(text, -1)
		if len(blocks) == 0 {
			continue
		}
		last := blocks[len(blocks)-1]
		if reconsiderRe.MatchString(text[last[1]:]) {
			// the model kept talking after the call, and changed direction
			continue
		}
		for _, block := range [][]string{{text[last[0]:last[1]], text[last[2]:last[3]]}} {
			fn := salvageFuncRe.FindStringSubmatch(block[1])
			if fn == nil {
				continue
			}
			args := map[string]any{}
			for _, p := range salvageParamRe.FindAllStringSubmatch(fn[2], -1) {
				args[p[1]] = strings.TrimSpace(p[2])
			}
			// arguments travel as a JSON *string* on the wire, not an
			// object: the assistant message is echoed back on the next turn,
			// and a raw object there is refused with "Invalid 'messages' in
			// payload". Real calls round-trip because we keep the server's
			// own bytes; a salvaged one has to be encoded the same way.
			obj, err := json.Marshal(args)
			if err != nil {
				continue
			}
			raw, err := json.Marshal(string(obj))
			if err != nil {
				continue
			}
			var call toolCall
			call.Type = "function"
			call.Id = fmt.Sprintf("salvaged-%d", len(out)+1)
			call.Function.Name = fn[1]
			call.Function.Arguments = json.RawMessage(raw)
			out = append(out, call)
		}
	}
	return out
}
