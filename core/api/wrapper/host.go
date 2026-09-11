package wrapper

// host.go — the in-process delivery of the tool table (the mobile tool
// bridge spec, docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md
// §4): a long-lived Runner over a memory session store, called directly by
// an embedding client (the gomobile exports in clientlibrary/service).
// Fourth delivery of the ONE definition — CLI verbs, manifest, MCP stdio,
// and this — sharing the manifest, the runner and the in-band error
// contract with the MCP server.

import (
	"context"
	"errors"
	"fmt"
)

// CallCodeToolError marks an in-band tool failure on a CallResult: the
// text is the repair tip the model reads.
const CallCodeToolError = "tool_error"

// CallCodeInvalidArguments marks a pre-flight refusal (runner.ArgumentError):
// the call's SHAPE was wrong — unknown tool, unknown or missing argument,
// wrong type, a value outside an enum — and nothing reached the workspace.
// Distinct from CallCodeToolError so a client budgeting repairs can leave
// a self-correctable shape mistake out of the count: a measured run spent
// its whole repair budget on one such refusal and a genuine error.
const CallCodeInvalidArguments = "invalid_arguments"

// Host is the in-process delivery: one Runner, one session, for the life
// of the embedding process.
type Host struct {
	runner *Runner
	store  *MemoryStore
}

// NewHost builds a host over a client. The session store is in memory:
// handles live until ResetSession or the process ends.
func NewHost(client *Client) *Host {
	store := NewMemoryStore()
	return &Host{runner: NewRunner(client, store), store: store}
}

// CallResult is one tool call's outcome as the embedding client receives
// it: Text is what the model reads (the MCP content text), JSON the
// machine shape for the app (the CLI --json shape). On IsError, Text is
// the repair tip and Code is CallCodeToolError.
type CallResult struct {
	Text    string `json:"text"`
	JSON    any    `json:"json,omitempty"`
	IsError bool   `json:"is_error"`
	Code    string `json:"code,omitempty"`
}

// Call runs one tool. Every failure — wrapper-side validation, a server
// C6 refusal (already in tool vocabulary), an unknown tool name — comes
// back IN-BAND, the §8.20 repair-loop contract: the model reads the tip
// and repairs the call.
func (h *Host) Call(ctx context.Context, name string, args map[string]any) CallResult {
	if args == nil {
		args = map[string]any{}
	}
	result, err := h.runner.Run(ctx, name, args)
	if err != nil {
		code := CallCodeToolError
		var argErr *ArgumentError
		if errors.As(err, &argErr) {
			code = CallCodeInvalidArguments
		}
		return CallResult{Text: h.errorText(err), IsError: true, Code: code}
	}
	return CallResult{Text: result.Text, JSON: result.JSON}
}

// ResetSession forgets the handle table and the working space — the
// conversation boundary for an embedding client.
func (h *Host) ResetSession() error {
	if err := h.store.Save(&Session{}); err != nil {
		return fmt.Errorf("reset session: %w", err)
	}
	return nil
}

// errorText renders the host's two delivery tips. Neither condition can be
// repaired by editing the call: a 401 cannot happen behind the in-process
// transport (the key is the process's own) and is a bug; an unreachable
// API means no account is running.
func (h *Host) errorText(err error) string {
	return repairTip(err, deliveryTips{
		unauthorized: func(te *ToolError) string {
			return te.Text + "\nfix: the in-process credential was rejected — this is a bug in the Anytype app, report it; no change to the call will help"
		},
		unreachable: func(err error) string {
			return fmt.Sprintf("the Anytype account is not running — ask the user to sign in; no change to the call will help (%v)", err)
		},
	})
}
