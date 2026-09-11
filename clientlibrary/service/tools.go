package service

// tools.go — the mobile tool bridge (docs/superpowers/specs/
// 2026-09-04-mobile-tool-bridge-design.md §5): the API v2 task tools
// (core/api/wrapper) for an on-device model, exported through gomobile as
// ServiceToolsManifest / ServiceToolsCall / ServiceToolsResetSession, plus
// ServiceToolsSetContext / ServiceToolsPreamble (APIV2.md §8.59).
// Nothing listens: the tools run in-process through the API component's
// engine, authorized as the current account (api.Service.ToolsHost).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// Envelope codes. Success carries no code; an in-band tool failure carries
// wrapper.CallCodeToolError, and a pre-flight shape mistake (unknown tool,
// unknown or missing argument, wrong type) wrapper.CallCodeInvalidArguments.
const (
	// ToolsCodeAccountNotRunning: no account app is running.
	ToolsCodeAccountNotRunning = "account_not_running"
	// ToolsCodeBadRequest: the arguments were not a JSON object, or the
	// tier is unknown.
	ToolsCodeBadRequest = "bad_request"
	// ToolsCodeInternal: a fault inside the bridge (a recovered panic, an
	// engine that could not be built).
	ToolsCodeInternal = "internal"
)

// toolsEnvelope is the one JSON shape every export returns or delivers.
// It mirrors wrapper.CallResult field for field; the type exists so the
// bridge can mint its own codes without reaching into the wrapper.
type toolsEnvelope struct {
	Text    string `json:"text"`
	JSON    any    `json:"json,omitempty"`
	IsError bool   `json:"is_error"`
	Code    string `json:"code,omitempty"`
}

var errAccountNotRunning = errors.New("no account is running")

// toolsHostProvider resolves the in-process delivery; a var so tests can
// stand in a host without an account.
var toolsHostProvider = defaultToolsHost

// defaultToolsHost resolves the host through the running account app's API
// component.
func defaultToolsHost() (*wrapper.Host, error) {
	a := mw.GetApp()
	if a == nil {
		return nil, errAccountNotRunning
	}
	svc, ok := a.Component(api.CName).(api.Service)
	if !ok {
		return nil, errAccountNotRunning
	}
	host, err := svc.ToolsHost()
	if err != nil {
		return nil, fmt.Errorf("tools host: %w", err)
	}
	return host, nil
}

// ToolsManifest renders the tier's ("small" | "large") tool manifest in
// the envelope's json field. Pure: it works before login, so a client can
// build its tools early.
func ToolsManifest(tier string) []byte {
	parsed, err := wrapper.ParseTier(tier)
	if err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeBadRequest})
	}
	manifest, err := wrapper.BuildManifestForTier(parsed)
	if err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal})
	}
	return encodeToolsEnvelope(toolsEnvelope{Text: fmt.Sprintf("%d tools", len(manifest.Tools)), JSON: manifest})
}

// ToolsCall runs one tool asynchronously as the current account and
// delivers exactly one envelope to callback, from a Go-owned thread. The
// caller's thread is never blocked.
func ToolsCall(name string, args []byte, callback MessageHandler) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if PanicHandler != nil {
					PanicHandler(r)
				}
				callback.Handle(encodeToolsEnvelope(toolsEnvelope{Text: "internal error in the tool bridge", IsError: true, Code: ToolsCodeInternal}))
			}
		}()
		callback.Handle(encodeToolsEnvelope(toolsCall(name, args)))
	}()
}

// toolsCall is the synchronous body of ToolsCall.
func toolsCall(name string, args []byte) toolsEnvelope {
	parsed, err := parseToolsArgs(args)
	if err != nil {
		return toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeBadRequest}
	}
	host, err := toolsHostProvider()
	if err != nil {
		return hostErrorEnvelope(err)
	}
	result := host.Call(context.Background(), name, parsed)
	return toolsEnvelope{Text: result.Text, JSON: result.JSON, IsError: result.IsError, Code: result.Code}
}

// ToolsSetContext tells the tools host what the app knows and the model
// does not: the space the user is looking at, their locale, IANA time zone
// and the model's result budget —
// `{"space":"…","locale":"…","time_zone":"…","max_result_chars":2000}`.
// App state: it survives ToolsResetSession, and an absent field clears. It
// is the space default for describe, create and create_type when a call
// names none and no find has set one, the clock relative dates resolve
// against, the preamble's "current space", and the bound on one tool
// result's text. Set max_result_chars from the model's window — a
// 4,096-token on-device model wants ~2000, a large one can leave it unset
// and keep the host's own ceiling.
func ToolsSetContext(args []byte) []byte {
	var runCtx wrapper.RunContext
	if trimmed := bytes.TrimSpace(args); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &runCtx); err != nil {
			return encodeToolsEnvelope(toolsEnvelope{
				Text:    fmt.Sprintf(`context must be a JSON object, e.g. {"space":"…","time_zone":"Europe/Berlin"}: %v`, err),
				IsError: true, Code: ToolsCodeBadRequest,
			})
		}
	}
	host, err := toolsHostProvider()
	if err != nil {
		return encodeToolsEnvelope(hostErrorEnvelope(err))
	}
	host.SetContext(runCtx)
	return encodeToolsEnvelope(toolsEnvelope{Text: "context set", JSON: runCtx})
}

// ToolsPreamble renders the workspace facts to put in front of the model
// before the user's first word: the date, the current space, the space
// count and what the conversation recently touched (wrapper/preamble.go).
// One global text, bounded; call it at the start of every turn, since the
// recents move.
func ToolsPreamble() []byte {
	host, err := toolsHostProvider()
	if err != nil {
		return encodeToolsEnvelope(hostErrorEnvelope(err))
	}
	text, err := host.Preamble(context.Background())
	if err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal})
	}
	return encodeToolsEnvelope(toolsEnvelope{Text: text})
}

// ToolsResetSession forgets the handle table — a new conversation.
func ToolsResetSession() []byte {
	host, err := toolsHostProvider()
	if err != nil {
		return encodeToolsEnvelope(hostErrorEnvelope(err))
	}
	if err := host.ResetSession(); err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal})
	}
	return encodeToolsEnvelope(toolsEnvelope{Text: "session reset"})
}

// parseToolsArgs decodes the arguments: empty or null is no arguments;
// anything but a JSON object is refused with the shape named.
func parseToolsArgs(args []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, fmt.Errorf(`arguments must be a JSON object, e.g. {"space":"..."}: %w`, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// hostErrorEnvelope maps a host resolution failure to its envelope.
func hostErrorEnvelope(err error) toolsEnvelope {
	if errors.Is(err, errAccountNotRunning) {
		return toolsEnvelope{
			Text:    "no account is running — ask the user to sign in; no change to the call will help",
			IsError: true,
			Code:    ToolsCodeAccountNotRunning,
		}
	}
	return toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal}
}

// encodeToolsEnvelope marshals an envelope. A tool result that cannot be
// encoded is reported instead of dropped — the envelope itself always can.
func encodeToolsEnvelope(e toolsEnvelope) []byte {
	data, err := json.Marshal(e)
	if err != nil {
		data, _ = json.Marshal(toolsEnvelope{Text: fmt.Sprintf("encode result: %v", err), IsError: true, Code: ToolsCodeInternal})
	}
	return data
}
