package wrapper

// mcp.go — the MCP (Model Context Protocol) delivery of a tool table
// (APIV2.md §8.20): one protocol loop — initialize, tools/list, tools/call,
// ping — over an Executor, which is either the curated table for a tier
// (the Runner) or the full table (core/api/wrapper/full). Two framings
// share it: newline-delimited JSON over stdio (cmd/anytype's mcp verb) and
// one message per POST on the API server's /mcp/{tier} route
// (core/api/server/mcp.go). Both call HandleMessage, which classifies a
// message before anything runs: a request is dispatched, a notification or
// a client response is acknowledged and never executed, a malformed
// envelope is refused.
//
// Transport decision (recorded in §8.20): the JSON-RPC surface an MCP tool
// server needs is small enough to implement directly, and a third-party
// MCP SDK would be a real dependency this repo does not otherwise carry,
// pulled in for a protocol subset. Hand-rolling keeps the wire shapes
// pinned by OUR tests instead of a vendor's release cadence.
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

// mcpLatestVersion is the newest MCP protocol revision the stdio delivery
// knows; the HTTP delivery pins its own (SetProtocolVersions).
const mcpLatestVersion = "2025-06-18"

// mcpSupportedVersions are the revisions the stdio delivery can answer
// verbatim — the tools surface (initialize / tools/list / tools/call, text
// content) is identical across them, so a known requested version is
// simply echoed.
var mcpSupportedVersions = map[string]bool{
	"2024-11-05":     true,
	"2025-03-26":     true,
	mcpLatestVersion: true,
}

// mcpScanBuffer bounds one inbound JSON-RPC line: the largest tool argument
// is add_blocks' 1 MiB markdown, ~2x under JSON escaping — 8 MiB is
// comfortable headroom without letting a runaway line eat the process.
const mcpScanBuffer = 8 << 20

// MCPMaxMessageBytes is the bound every delivery puts on one inbound
// message, exported so the HTTP framing caps its body at the same number.
const MCPMaxMessageBytes = mcpScanBuffer

// mcpMessage is one inbound JSON-RPC message (request or notification).
type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

// Response is one outbound JSON-RPC response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC 2.0 error codes.
const (
	mcpParseError     = -32700
	mcpInvalidRequest = -32600
	mcpMethodNotFound = -32601
	mcpInvalidParams  = -32602
	mcpInternalError  = -32603
)

// ToolListing is one tools/list entry as an Executor publishes it.
type ToolListing struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	Destructive bool
}

// Executor is what the MCP loop serves: a tool table, its initialize
// text, and one Run. The curated tiers satisfy it through the Runner
// (tierExecutor); the full table through its own executor.
type Executor interface {
	Tools() []ToolListing
	Instructions() string
	Run(ctx context.Context, name string, args map[string]any) (*Result, error)
}

// mcpTool is one tools/list entry (the MCP Tool shape).
type mcpTool struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema json.RawMessage     `json:"inputSchema"`
	Annotations *mcpToolAnnotations `json:"annotations,omitempty"`
}

// mcpToolAnnotations carries the advisory hints: read-only so hosts can
// skip write confirmation, destructive so they can ask for it.
type mcpToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint,omitempty"`
	DestructiveHint bool `json:"destructiveHint,omitempty"`
}

// mcpContent is one content block of a tools/call result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mcpCallResult is the tools/call result: text content, the structured
// form beside it when the tool has one, and tool failures in-band
// (isError) so the model sees the repair tip.
type mcpCallResult struct {
	Content           []mcpContent `json:"content"`
	StructuredContent any          `json:"structuredContent,omitempty"`
	IsError           bool         `json:"isError,omitempty"`
}

// MCPServer serves an Executor over the MCP protocol.
type MCPServer struct {
	exec  Executor
	tier  Tier
	tools []mcpTool
	names map[string]bool
	// apiAddress is named in the unreachable tip.
	apiAddress string
	versions   map[string]bool
	latest     string
}

// NewMCPServer builds a server over a runner for one curated tier
// (typically NewRunner(client, NewMemoryStore()) — the long-lived delivery
// holds handle state in memory).
func NewMCPServer(runner *Runner, tier Tier) *MCPServer {
	return NewMCPServerOver(&tierExecutor{runner: runner, tier: tier}, tier, runner.client.BaseURL)
}

// NewMCPServerOver builds a server over any executor. apiAddress is what
// the unreachable tip names. The tool listing is taken once: a table is
// static for the life of the server.
func NewMCPServerOver(exec Executor, tier Tier, apiAddress string) *MCPServer {
	listing := exec.Tools()
	tools := make([]mcpTool, 0, len(listing))
	names := make(map[string]bool, len(listing))
	for _, t := range listing {
		entry := mcpTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
		if t.ReadOnly || t.Destructive {
			entry.Annotations = &mcpToolAnnotations{ReadOnlyHint: t.ReadOnly, DestructiveHint: t.Destructive}
		}
		tools = append(tools, entry)
		names[t.Name] = true
	}
	return &MCPServer{exec: exec, tier: tier, tools: tools, names: names, apiAddress: apiAddress,
		versions: mcpSupportedVersions, latest: mcpLatestVersion}
}

// SetProtocolVersions pins the revisions this server answers; the last one
// is what an unknown request gets. The HTTP delivery pins its own set
// (spec §3.2), the stdio delivery keeps the default.
func (s *MCPServer) SetProtocolVersions(versions ...string) {
	s.versions = make(map[string]bool, len(versions))
	for _, v := range versions {
		s.versions[v] = true
	}
	s.latest = versions[len(versions)-1]
}

// SupportsProtocolVersion reports whether a revision is one this server
// answers.
func (s *MCPServer) SupportsProtocolVersion(v string) bool { return s.versions[v] }

// tierExecutor is the curated table of one tier over a Runner.
type tierExecutor struct {
	runner *Runner
	tier   Tier
}

func (e *tierExecutor) Tools() []ToolListing {
	tools := ToolsForTier(e.tier)
	out := make([]ToolListing, 0, len(tools))
	for _, t := range tools {
		schema, err := toolSchema(t)
		if err != nil {
			// unreachable for a well-formed table (pinned by manifest tests);
			// degrade to an empty open schema rather than break the list
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, ToolListing{Name: t.Name, Description: t.Description, InputSchema: schema, ReadOnly: t.ReadOnly})
	}
	return out
}

func (e *tierExecutor) Instructions() string { return mcpInstructions(e.tier) }

func (e *tierExecutor) Run(ctx context.Context, name string, args map[string]any) (*Result, error) {
	return e.runner.Run(ctx, name, args)
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
		if handled := s.HandleMessage(ctx, []byte(line)); handled.Response != nil {
			if err := writeMCP(out, handled.Response); err != nil {
				return fmt.Errorf("write mcp response: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read mcp input: %w", err)
	}
	return nil
}

// MessageKind is what HandleMessage made of a message.
type MessageKind int

const (
	// MessageInvalid is a malformed envelope: the response is the error.
	MessageInvalid MessageKind = iota
	// MessageRequest carries an id and was dispatched: the response is the
	// answer.
	MessageRequest
	// MessageNotification carries no id: acknowledged, never executed.
	MessageNotification
	// MessageResponse is a client's answer to a server request (this
	// server sends none): acknowledged.
	MessageResponse
)

// Handled is HandleMessage's outcome.
type Handled struct {
	Kind   MessageKind
	Method string
	// Response is nil for a notification or a client response.
	Response *Response
}

// ClassifyMessage validates one JSON-RPC message and says what it is,
// running nothing: the framing decides what to admit (a session, a slot)
// from this before any method is dispatched. Response is set only for an
// invalid message — the error to answer with.
func ClassifyMessage(raw []byte) Handled {
	h, _ := parseMessage(raw)
	return h
}

// parseMessage is ClassifyMessage plus the decoded message for dispatch.
func parseMessage(raw []byte) (Handled, *mcpMessage) {
	raw = []byte(strings.TrimSpace(string(raw)))
	invalid := func(code int, message string) (Handled, *mcpMessage) {
		return Handled{Kind: MessageInvalid, Response: &Response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &RPCError{Code: code, Message: message}}}, nil
	}
	if len(raw) > 0 && raw[0] == '[' {
		// JSON-RPC batching was removed in MCP 2025-06-18 and no known host
		// sends it; refusing beats half-implementing
		return invalid(mcpInvalidRequest, "JSON-RPC batching is not supported — send one message at a time")
	}
	var msg mcpMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return invalid(mcpParseError, fmt.Sprintf("parse JSON-RPC message: %v", err))
	}
	if msg.JSONRPC != "2.0" {
		return invalid(mcpInvalidRequest, `jsonrpc must be "2.0"`)
	}
	if msg.Method == "" {
		if len(msg.Result) > 0 || len(msg.Error) > 0 {
			return Handled{Kind: MessageResponse}, &msg
		}
		return invalid(mcpInvalidRequest, "a request names a method")
	}
	if len(msg.ID) == 0 {
		return Handled{Kind: MessageNotification, Method: msg.Method}, &msg
	}
	if !validRequestId(msg.ID) {
		return invalid(mcpInvalidRequest, "id must be a string or a number")
	}
	return Handled{Kind: MessageRequest, Method: msg.Method}, &msg
}

// HandleMessage classifies one JSON-RPC message and dispatches it only if
// it is a request. The classification is the safety property every
// framing relies on: an id-less tools/call is a notification and runs
// nothing; a batch, a wrong jsonrpc member or a malformed id is refused
// before any method is looked at. A request whose context has already
// ended is answered with an error and not run.
func (s *MCPServer) HandleMessage(ctx context.Context, raw []byte) Handled {
	handled, msg := parseMessage(raw)
	if handled.Kind != MessageRequest {
		return handled
	}
	resp := &Response{JSONRPC: "2.0", ID: msg.ID}
	if err := ctx.Err(); err != nil {
		resp.Error = &RPCError{Code: mcpInternalError, Message: fmt.Sprintf("the request ended before it ran: %v", err)}
		handled.Response = resp
		return handled
	}
	result, rpcErr := s.dispatch(ctx, msg)
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	handled.Response = resp
	return handled
}

// validRequestId accepts a JSON string or number; MCP forbids null and
// JSON-RPC's other shapes are not ids a client sends.
func validRequestId(id json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(id))
	if trimmed == "" || trimmed == "null" {
		return false
	}
	switch trimmed[0] {
	case '"':
		return json.Valid(id)
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return json.Valid(id)
	}
	return false
}

// dispatch routes one request to its method handler.
func (s *MCPServer) dispatch(ctx context.Context, msg *mcpMessage) (any, *RPCError) {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg.Params), nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools}, nil
	case "tools/call":
		return s.handleToolsCall(ctx, msg.Params)
	default:
		return nil, &RPCError{Code: mcpMethodNotFound, Message: fmt.Sprintf("method %q not found", msg.Method)}
	}
}

// handleInitialize negotiates the protocol version (echo a known requested
// version, else answer with ours) and serves the executor's instructions —
// the workflow steering a model needs before its first call.
func (s *MCPServer) handleInitialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := s.latest
	if s.versions[p.ProtocolVersion] {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": struct{}{}},
		"serverInfo":      map[string]any{"name": "anytype", "version": "1"},
		"instructions":    s.exec.Instructions(),
	}
}

// mcpInstructions renders a curated tier's workflow steering (the SKILL.md
// loop, compressed to what fits an initialize response).
func mcpInstructions(tier Tier) string {
	var b strings.Builder
	b.WriteString("Anytype task tools over the local API. The loop: " +
		"spaces lists space ids when none is known; " +
		"find (space + query/type/filter) numbers matching objects 1, 2, … — pass that number as `object` to the other tools, and re-run find to renumber; " +
		"find with a space and none of the three matches nothing, so it lists the space unnumbered and no number exists to pass on; " +
		"describe a type BEFORE create or set_properties — property names and select option names must match exactly; " +
		"read lists every block with its text and the short label the editing tools take as `block` — " +
		"edit_text alone can skip it: omit block and the find snippet locates the block when it matches exactly one.")
	if hasToolInTier(tier, "set_cell") {
		b.WriteString(" set_cell's row and col take the text read mode=full shows: a column's header (each column carries it), a row's first cell — or a row/column id.")
	}
	b.WriteString(" Dates accept today, tomorrow, +3d, weekday names; @me means the calling user." +
		" In filter strings, write multi-word property names with underscores (Due_date)." +
		" To complete a task-like object, set its done/status property with set_properties." +
		" To change an object's type (a page into a Task), pass type to set_properties; only same-family layouts convert, and a refusal names the types it can take." +
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

// handleToolsCall executes one tool. Tool failures are IN-BAND results
// (isError + the repair tip) so the model can read and fix them; only a
// name outside the table is a protocol error — with the table's tool list
// as the tip.
func (s *MCPServer) handleToolsCall(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: mcpInvalidParams, Message: fmt.Sprintf("parse tools/call params: %v", err)}
	}
	if !s.names[p.Name] {
		return nil, &RPCError{Code: mcpInvalidParams,
			Message: fmt.Sprintf("unknown tool %q — tools: %s", p.Name, s.toolList())}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	result, err := s.exec.Run(ctx, p.Name, p.Arguments)
	if err != nil {
		return mcpCallResult{
			Content: []mcpContent{{Type: "text", Text: s.errorText(err)}},
			IsError: true,
		}, nil
	}
	out := mcpCallResult{Content: []mcpContent{{Type: "text", Text: result.Text}}}
	if raw, ok := result.JSON.(json.RawMessage); ok && len(raw) > 0 {
		out.StructuredContent = raw
	}
	return out, nil
}

// toolList renders the served tool names for steering text.
func (s *MCPServer) toolList() string {
	names := make([]string, 0, len(s.tools))
	for _, t := range s.tools {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
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
		return fmt.Sprintf("cannot reach the local Anytype API at %s — ask the user to start the Anytype app; no change to the call will help (%v)", s.apiAddress, err)
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
func writeMCP(out io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	data = append(data, '\n')
	_, err = out.Write(data)
	return err
}
