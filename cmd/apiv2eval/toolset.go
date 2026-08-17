package main

// toolset.go — the two surfaces under test, both serving schemas the
// PRODUCT publishes rather than any the harness wrote:
//
//   - the wrapper arm drives core/api/wrapper's real MCP server in-process
//     over a pipe pair: tools/list gives the tier's schemas verbatim, and
//     tools/call gives the model exactly the text (and isError framing) a
//     real MCP host would show it, repair tips and all.
//   - the ops arm serves the per-op schemas the API itself publishes at
//     GET /v2/schemas/ops/{op} as the tools' parameters, and executes each
//     call as a single-op PATCH. This is the surface the insert_blocks
//     payload-id question lives on: the wrapper's add_blocks takes markdown
//     and has no id channel at all, so only here can a model emit one.
//
// The one schema the harness authors is the ops arm's read_object: a GET has
// no published request-body schema to serve. It is flagged as such in the
// report.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// toolSpec is one tool as the model sees it.
type toolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// toolOutcome is one tool call's result as the model sees it.
type toolOutcome struct {
	Text    string
	IsError bool
}

// toolset is one arm's tool surface.
type toolset interface {
	tools() []toolSpec
	instructions() string
	call(ctx context.Context, name string, args map[string]any) toolOutcome
	close() error
}

//
// ---- the wrapper arm: the product's MCP server, in process ----
//

// mcpToolset speaks MCP JSON-RPC to wrapper.MCPServer over a pipe pair.
type mcpToolset struct {
	mu      sync.Mutex
	in      *io.PipeWriter
	out     *bufio.Scanner
	closer  *io.PipeReader
	nextId  int
	serveWG sync.WaitGroup

	toolList  []toolSpec
	instrText string
}

// newMCPToolset starts the product's MCP server on an in-process pipe and
// completes the initialize / tools/list handshake. variant rewrites what the
// tool list PUBLISHES (see editTextVariant) and nothing else — the runner
// behind it is the same object in every arm.
func newMCPToolset(ctx context.Context, runner *wrapper.Runner, tier wrapper.Tier, variant editTextVariant) (*mcpToolset, error) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	server := wrapper.NewMCPServer(runner, tier)
	ts := &mcpToolset{in: inW, closer: outR}
	ts.out = bufio.NewScanner(outR)
	ts.out.Buffer(make([]byte, 64<<10), 8<<20)
	ts.serveWG.Add(1)
	go func() {
		defer ts.serveWG.Done()
		err := server.Serve(context.Background(), inR, outW)
		outW.CloseWithError(err)
	}()

	initResp, err := ts.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "apiv2eval", "version": "1"},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	var initResult struct {
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(initResp, &initResult); err != nil {
		return nil, fmt.Errorf("decode mcp initialize result: %w", err)
	}
	ts.instrText = initResult.Instructions

	listResp, err := ts.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list: %w", err)
	}
	var list struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(listResp, &list); err != nil {
		return nil, fmt.Errorf("decode mcp tools/list result: %w", err)
	}
	for _, t := range list.Tools {
		ts.toolList = append(ts.toolList, toolSpec{Name: t.Name, Description: t.Description, Parameters: t.InputSchema})
	}
	varied, err := applyEditTextVariant(ts.toolList, variant)
	if err != nil {
		return nil, err
	}
	ts.toolList = varied
	return ts, nil
}

//
// ---- the published-surface variants: the edit_text A/B ----
//

// editTextVariant selects which edit_text definition an arm PUBLISHES.
//
// The hypothesis is §8.30's mechanism — a small model fills the fields a
// schema shows it, even when they are documented optional — measured on the
// one field these models demonstrably do emit. §8.30 removed `id` from
// insert_blocks on that argument and the 210-call probe could not confirm it
// (the models emit no payload id either way, so the control fired). `block`
// is different: gemma4:e2b supplied edit_text's documented-optional `block`
// on 3 of 3 attempts, reading the whole document first to obtain a value
// locateBlock would have derived from the snippet.
//
// The variants rewrite the tools/list ENTRY and nothing else. The same
// Runner, the same server-side locator (§8.43 — resolution moved down from
// the wrapper's locateBlock) and the same validateArgs table execute every
// call, so a `block` a model sends under B1 is still accepted and still
// works — the server's behaviour must be identical across arms or a
// difference between them is not attributable to the surface. What the model
// did with the field it was or was not shown is counted from the record
// (signals.EditTextWithBlock), never enforced by refusing it.
type editTextVariant string

const (
	// editTextAsShipped is arm A: the definition the product serves today.
	editTextAsShipped editTextVariant = ""
	// editTextNoBlock is arm B1: `block` removed from the published
	// arguments — from the schema and from the description that names it.
	// Snippet-only location.
	editTextNoBlock editTextVariant = "no-block"
	// editTextProse is arm B2: `block` still published, with the instruction
	// not to read first leading the description.
	editTextProse editTextVariant = "prose"
)

// editTextBlockSentence is the clause B1 removes from the shipped
// description. It is MATCHED, not assumed: if the product rewords it, the
// arm fails loudly rather than quietly publishing arm A's surface under
// B1's name and reporting the difference as a null result.
const editTextBlockSentence = "block is optional — when omitted, the snippet itself must pin down exactly one block. "

// editTextNoReadFirst is B2's added lead, in the imperative-plus-reason
// register the manifest already uses ("Call this first — property keys and
// select option names must match exactly.").
const editTextNoReadFirst = "Do not read the object first — the find snippet locates the block on its own. "

// applyEditTextVariant returns the tool list an arm publishes.
func applyEditTextVariant(specs []toolSpec, variant editTextVariant) ([]toolSpec, error) {
	if variant == editTextAsShipped {
		return specs, nil
	}
	out := make([]toolSpec, len(specs))
	copy(out, specs)
	idx := -1
	for i, spec := range out {
		if spec.Name == "edit_text" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("the %q variant varies edit_text, which this arm does not publish", variant)
	}
	spec := out[idx]
	switch variant {
	case editTextProse:
		spec.Description = editTextNoReadFirst + spec.Description
	case editTextNoBlock:
		trimmed := strings.Replace(spec.Description, editTextBlockSentence, "", 1)
		if trimmed == spec.Description {
			return nil, fmt.Errorf("edit_text's description no longer contains %q — the %q arm would publish the baseline surface under another name",
				editTextBlockSentence, variant)
		}
		spec.Description = trimmed
		params, err := removeSchemaProperty(spec.Parameters, "block")
		if err != nil {
			return nil, fmt.Errorf("build the %q edit_text schema: %w", variant, err)
		}
		spec.Parameters = params
	default:
		return nil, fmt.Errorf("unknown edit_text variant %q — variants: %q, %q, %q",
			variant, editTextAsShipped, editTextNoBlock, editTextProse)
	}
	out[idx] = spec
	return out, nil
}

// removeSchemaProperty deletes one property from a published tool schema.
// A property that is not there is an error, not a no-op: the arm exists to
// publish a DIFFERENT surface, and silently publishing the same one is the
// failure mode that makes an A/B unreadable. The product renders its schemas
// from a Go map, so this re-marshal reproduces the same key order.
func removeSchemaProperty(schema json.RawMessage, name string) (json.RawMessage, error) {
	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil, fmt.Errorf("decode the published schema: %w", err)
	}
	properties, _ := decoded["properties"].(map[string]any)
	if _, ok := properties[name]; !ok {
		return nil, fmt.Errorf("the published schema has no %q property", name)
	}
	delete(properties, name)
	if required, ok := decoded["required"].([]any); ok {
		kept := make([]any, 0, len(required))
		for _, entry := range required {
			if s, _ := entry.(string); s != name {
				kept = append(kept, entry)
			}
		}
		decoded["required"] = kept
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("encode the varied schema: %w", err)
	}
	return data, nil
}

// request sends one JSON-RPC request and reads its response.
func (t *mcpToolset) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextId++
	msg := map[string]any{"jsonrpc": "2.0", "id": t.nextId, "method": method}
	if params != nil {
		msg["params"] = params
	}
	line, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode mcp request: %w", err)
	}
	if _, err := t.in.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("write mcp request: %w", err)
	}
	if !t.out.Scan() {
		if err := t.out.Err(); err != nil {
			return nil, fmt.Errorf("read mcp response: %w", err)
		}
		return nil, fmt.Errorf("mcp server closed the connection")
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(t.out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode mcp response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (t *mcpToolset) tools() []toolSpec    { return t.toolList }
func (t *mcpToolset) instructions() string { return t.instrText }

func (t *mcpToolset) call(ctx context.Context, name string, args map[string]any) toolOutcome {
	raw, err := t.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		// a protocol error IS what the host would report to the model — an
		// unknown tool name lands here, with the tier's tool list as the tip
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return toolOutcome{Text: fmt.Sprintf("decode tools/call result: %v", err), IsError: true}
	}
	var parts []string
	for _, c := range result.Content {
		parts = append(parts, c.Text)
	}
	return toolOutcome{Text: strings.Join(parts, "\n"), IsError: result.IsError}
}

func (t *mcpToolset) close() error {
	t.in.Close()
	t.serveWG.Wait()
	t.closer.Close()
	return nil
}

//
// ---- the ops arm: the published per-op schemas, executed as PATCH ----
//

// opsArmOps is the op set the ops arm serves — the document-editing ops a
// realistic edit reaches for, kept under the small-model tool cliff. The
// view and collection ops are out of scope for these tasks.
var opsArmOps = []string{
	"insert_blocks",
	"replace_text",
	"replace_subtree",
	"update_block",
	"move_block",
	"delete_block",
	"set_cell",
	"set_properties",
}

// readObjectSchema is the ONE harness-authored schema in the run: a GET has
// no published request-body schema to serve.
const readObjectSchema = `{"type":"object","additionalProperties":false,"properties":{"outline":{"type":"boolean","description":"true = structure only ({indent, id, type}); default false = the whole document"}}}`

// opsToolset serves the published op schemas against one bound object.
type opsToolset struct {
	client   *apiClient
	spaceId  string
	objectId string
	specs    []toolSpec
}

func newOpsToolset(ctx context.Context, client *apiClient, spaceId, objectId string) (*opsToolset, error) {
	ts := &opsToolset{client: client, spaceId: spaceId, objectId: objectId}
	ts.specs = append(ts.specs, toolSpec{
		Name:        "read_object",
		Description: "Read the object being edited. Block ids come from here — use them exactly as served.",
		Parameters:  json.RawMessage(readObjectSchema),
	})
	for _, op := range opsArmOps {
		schema, example, err := client.opSchema(ctx, op)
		if err != nil {
			return nil, err
		}
		// the served example is a whole PATCH body, {"ops":[{…}]}, while the
		// schema beside it describes ONE op — the example is not an instance
		// of its own schema. The arm pairs the schema with the op inside, so
		// a long run measures the schema rather than that mismatch; the
		// mismatch itself is measured by -probe -probe-example (probe.go).
		ts.specs = append(ts.specs, toolSpec{
			Name:        op,
			Description: fmt.Sprintf("PATCH op %q on the object being edited. Example: %s", op, unwrapOpsExample(example)),
			Parameters:  schema,
		})
	}
	return ts, nil
}

func (t *opsToolset) tools() []toolSpec { return t.specs }

func (t *opsToolset) instructions() string {
	return "You edit ONE Anytype object through its HTTP API. Every tool but read_object is a single PATCH op applied to that object; " +
		"its parameters are the API's own published schema for that op — follow it exactly. " +
		"read_object returns the document: blocks are a flat array in order with an integer indent and an id. " +
		"Use block ids exactly as a read served them. Errors name the field to fix; fix that field and retry once."
}

func (t *opsToolset) call(ctx context.Context, name string, args map[string]any) toolOutcome {
	if name == "read_object" {
		var query url.Values
		if outline, _ := args["outline"].(bool); outline {
			query = url.Values{"outline": {"true"}}
		}
		path := "/v2/spaces/" + url.PathEscape(t.spaceId) + "/objects/" + url.PathEscape(t.objectId)
		raw, err := t.client.call(ctx, http.MethodGet, path, query, nil, nil)
		if err != nil {
			return toolOutcome{Text: apiErrorText(err), IsError: true}
		}
		return toolOutcome{Text: string(raw)}
	}
	// The tool NAME already says which op it is, so the const is set from it
	// — absent, wrong or right. This arm asks about the PAYLOAD; letting a
	// mis-written discriminator 400 every call would answer a different
	// question, and one that only exists because the arm splits the ops into
	// separate tools (a raw HTTP caller has no tool name and must write the
	// field). What the model wrote is preserved in the call record and
	// counted separately — see signals.OpConstAbsent / OpConstWrong.
	args["op"] = name
	result, err := t.client.patchOps(ctx, t.spaceId, t.objectId, []any{args})
	if err != nil {
		return toolOutcome{Text: apiErrorText(err), IsError: true}
	}
	return toolOutcome{Text: editReceipt(result)}
}

func (t *opsToolset) close() error { return nil }

// apiErrorText renders a server refusal the way a caller reading the HTTP
// response would: the C6 message followed by its path-addressed issues and
// hints. This is the raw surface — no wrapper vocabulary translation.
func apiErrorText(err error) string {
	var ae *apiError
	if !errors.As(err, &ae) {
		return err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s: %s", ae.Status, ae.Code, ae.Message)
	for _, issue := range ae.Issues {
		b.WriteString("\n  ")
		if issue.Path != "" {
			b.WriteString(issue.Path)
			b.WriteString(": ")
		}
		b.WriteString(issue.Message)
		if issue.Hint != "" {
			b.WriteString(" (" + issue.Hint + ")")
		}
	}
	return b.String()
}

// editReceipt renders a successful PATCH the way the API answers it.
func editReceipt(result *v2model.EditResult) string {
	data, err := json.Marshal(result)
	if err != nil {
		return "ok"
	}
	return "ok " + string(data)
}
