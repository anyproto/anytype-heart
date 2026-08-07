package wrapper

// mcp.go — the MCP (Model Context Protocol) delivery of the tool table
// (APIV2.md §8.20): a long-lived stdio server hosting the same manifest the
// CLI verbs are generated from, tier-filtered (tier.go). This is the §8.6
// "long-lived host" — one Runner over a MemoryStore, constructed once by
// cmd/anytype's mcp verb.
//
// Transport decision (recorded in §8.20): the JSON-RPC surface an MCP tool
// server needs — initialize, tools/list, tools/call, ping — is small enough
// to implement directly over newline-delimited JSON, and a third-party MCP
// SDK would be a real dependency this repo does not otherwise carry, pulled
// in for a protocol subset. Hand-rolling keeps the wire shapes pinned by
// OUR tests instead of a vendor's release cadence.
//
// The error contract is the §8.20 repair loop: a failed tools/call is an
// IN-BAND result (isError: true) whose text is the wrapper's own
// agent-tuned tip — the server's C6 hints survive through ToolError, the
// ops→tool vocabulary translation has already run, and this layer adds the
// two tips only a process boundary can know (the API server is unreachable;
// the key was rejected — both "ask the user", not "change the call").
// Protocol-level errors (unknown tool, malformed JSON) are JSON-RPC errors,
// per spec; the unknown-tool message still lists the tier's tools.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// mcpLatestVersion is the newest MCP protocol revision this server knows.
const mcpLatestVersion = "2025-06-18"

// mcpSupportedVersions are the revisions the server can answer verbatim —
// the tools surface (initialize / tools/list / tools/call, text content) is
// identical across them, so a known requested version is simply echoed.
var mcpSupportedVersions = map[string]bool{
	"2024-11-05":     true,
	"2025-03-26":     true,
	mcpLatestVersion: true,
}

// mcpScanBuffer bounds one inbound JSON-RPC line: the largest tool argument
// is add_blocks' 1 MiB markdown, ~2x under JSON escaping — 8 MiB is
// comfortable headroom without letting a runaway line eat the process.
const mcpScanBuffer = 8 << 20

// mcpMessage is one inbound JSON-RPC message (request or notification).
type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse is one outbound JSON-RPC response.
type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

// mcpError is the JSON-RPC error object.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC 2.0 error codes.
const (
	mcpParseError     = -32700
	mcpInvalidRequest = -32600
	mcpMethodNotFound = -32601
	mcpInvalidParams  = -32602
)

// mcpTool is one tools/list entry (the MCP Tool shape).
type mcpTool struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema json.RawMessage     `json:"inputSchema"`
	Annotations *mcpToolAnnotations `json:"annotations,omitempty"`
}

// mcpToolAnnotations carries the read-only hint for the four non-mutating
// tools, so hosts can skip write confirmation on them.
type mcpToolAnnotations struct {
	ReadOnlyHint bool `json:"readOnlyHint"`
}

// mcpContent is one content block of a tools/call result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mcpCallResult is the tools/call result: text content, with tool failures
// in-band (isError) so the model sees the repair tip.
type mcpCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPServer serves the tool table over MCP stdio framing.
type MCPServer struct {
	runner *Runner
	tier   Tier
}

// NewMCPServer builds a server over a runner (typically NewRunner(client,
// NewMemoryStore()) — the long-lived delivery holds handle state in memory).
func NewMCPServer(runner *Runner, tier Tier) *MCPServer {
	return &MCPServer{runner: runner, tier: tier}
}

// Serve reads newline-delimited JSON-RPC messages from in and writes
// responses to out until EOF (the host closing stdin is the shutdown
// signal) or ctx cancellation. Malformed lines answer JSON-RPC errors;
// they never kill the server.
func (s *MCPServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64<<10), mcpScanBuffer)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if resp := s.handleLine(ctx, line); resp != nil {
			if err := writeMCP(out, resp); err != nil {
				return fmt.Errorf("write mcp response: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read mcp input: %w", err)
	}
	return nil
}

// handleLine parses and dispatches one message; nil means no response (a
// notification, or an unparseable notification-shaped line).
func (s *MCPServer) handleLine(ctx context.Context, line string) *mcpResponse {
	if strings.HasPrefix(line, "[") {
		// JSON-RPC batching predates MCP 2025-06-18 and no known host sends
		// it; refusing beats half-implementing
		return &mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &mcpError{Code: mcpInvalidRequest, Message: "JSON-RPC batching is not supported — send one message per line"}}
	}
	var msg mcpMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return &mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &mcpError{Code: mcpParseError, Message: fmt.Sprintf("parse JSON-RPC message: %v", err)}}
	}
	isRequest := len(msg.ID) > 0 && string(msg.ID) != "null"
	result, rpcErr := s.dispatch(ctx, &msg)
	if !isRequest {
		return nil // notifications never answer, not even errors
	}
	resp := &mcpResponse{JSONRPC: "2.0", ID: msg.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	return resp
}

// dispatch routes one message to its method handler.
func (s *MCPServer) dispatch(ctx context.Context, msg *mcpMessage) (any, *mcpError) {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg.Params), nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return s.handleToolsList(), nil
	case "tools/call":
		return s.handleToolsCall(ctx, msg.Params)
	case "notifications/initialized", "notifications/cancelled", "notifications/roots/list_changed":
		return nil, nil // acknowledged by silence
	default:
		return nil, &mcpError{Code: mcpMethodNotFound, Message: fmt.Sprintf("method %q not found", msg.Method)}
	}
}

// handleInitialize negotiates the protocol version (echo a known requested
// version, else answer with ours) and serves the tier's instructions — the
// workflow steering a small model needs before its first call.
func (s *MCPServer) handleInitialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := mcpLatestVersion
	if mcpSupportedVersions[p.ProtocolVersion] {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": struct{}{}},
		"serverInfo":      map[string]any{"name": "anytype", "version": "1"},
		"instructions":    mcpInstructions(s.tier),
	}
}

// mcpInstructions renders the tier's workflow steering (the SKILL.md loop,
// compressed to what fits an initialize response).
func mcpInstructions(tier Tier) string {
	var b strings.Builder
	b.WriteString("Anytype task tools over the local API. The loop: " +
		"spaces lists space ids when none is known; " +
		"find (space + query/type/filter) numbers matching objects 1, 2, … — pass that number as `object` to the other tools, and re-run find to renumber; " +
		"describe a type BEFORE create or set_properties — property keys and select option names must match exactly; " +
		"read (mode=outline) lists the short block labels the editing tools take as `block`.")
	if hasToolInTier(tier, "set_cell") {
		b.WriteString(" Table row and column labels come from read mode=full.")
	}
	b.WriteString(" Dates accept today, tomorrow, +3d, weekday names; @me means the calling user." +
		" To complete a task-like object, set its done/status property with set_properties." +
		" Every error says how to fix the call — follow it and retry once; do not loop.")
	return b.String()
}

// hasToolInTier reports whether the tier serves the named tool.
func hasToolInTier(tier Tier, name string) bool {
	for _, t := range ToolsForTier(tier) {
		if t.Name == name {
			return true
		}
	}
	return false
}

// handleToolsList serves the tier's tool set. No pagination: the whole
// point of the tier split is a set small enough to list whole.
func (s *MCPServer) handleToolsList() any {
	tools := ToolsForTier(s.tier)
	out := make([]mcpTool, 0, len(tools))
	for _, t := range tools {
		schema, err := toolSchema(t)
		if err != nil {
			// unreachable for a well-formed table (pinned by manifest tests);
			// degrade to an empty open schema rather than break the list
			schema = json.RawMessage(`{"type":"object"}`)
		}
		entry := mcpTool{Name: t.Name, Description: t.Description, InputSchema: schema}
		if t.ReadOnly {
			entry.Annotations = &mcpToolAnnotations{ReadOnlyHint: true}
		}
		out = append(out, entry)
	}
	return map[string]any{"tools": out}
}

// handleToolsCall executes one tool. Tool failures are IN-BAND results
// (isError + the repair tip) so the model can read and fix them; only a
// name outside the tier is a protocol error — with the tier's tool list as
// the tip.
func (s *MCPServer) handleToolsCall(ctx context.Context, params json.RawMessage) (any, *mcpError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &mcpError{Code: mcpInvalidParams, Message: fmt.Sprintf("parse tools/call params: %v", err)}
	}
	if !hasToolInTier(s.tier, p.Name) {
		return nil, &mcpError{Code: mcpInvalidParams,
			Message: fmt.Sprintf("unknown tool %q — tools: %s", p.Name, toolListForError(s.tier))}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	result, err := s.runner.Run(ctx, p.Name, p.Arguments)
	if err != nil {
		return mcpCallResult{
			Content: []mcpContent{{Type: "text", Text: s.errorText(err)}},
			IsError: true,
		}, nil
	}
	return mcpCallResult{Content: []mcpContent{{Type: "text", Text: result.Text}}}, nil
}

// errorText renders an error as the repair tip the model reads. Wrapper and
// server errors already carry their own steering (validateArgs, C6 hints,
// the ops→tool translation); this layer adds the two conditions whose fix
// is outside the model's reach — both must say "ask the user", or a small
// model burns its retries re-sending variants of a call that can never
// succeed.
func (s *MCPServer) errorText(err error) string {
	var te *ToolError
	if errors.As(err, &te) {
		if te.Status == 401 {
			return te.Text + "\nfix: the API key was rejected — ask the user to check ANYTYPE_API_KEY (Anytype app → Settings → API keys); no change to the call will help"
		}
		return te.Text
	}
	// not a ToolError: the request never reached the API server (transport)
	// or failed wrapper-side; transport failures name the base URL so the
	// user knows what to start
	if isTransportError(err) {
		return fmt.Sprintf("cannot reach the local Anytype API at %s — ask the user to start the Anytype app; no change to the call will help (%v)", s.runner.client.BaseURL, err)
	}
	return err.Error()
}

// isTransportError reports whether the request never got an HTTP response:
// client.do wraps http.Client.Do's *url.Error (dial refused, timeout, DNS)
// with %w, so the chain identifies it structurally.
func isTransportError(err error) bool {
	var ue *url.Error
	return errors.As(err, &ue)
}

// writeMCP marshals one response and terminates it with the newline the
// stdio framing requires (json.Marshal never emits raw newlines).
func writeMCP(out io.Writer, resp *mcpResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	data = append(data, '\n')
	_, err = out.Write(data)
	return err
}
