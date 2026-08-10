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
//     call as a single-op PATCH. This is the surface the insertBlocks
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
// completes the initialize / tools/list handshake.
func newMCPToolset(ctx context.Context, runner *wrapper.Runner, tier wrapper.Tier) (*mcpToolset, error) {
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
	return ts, nil
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
	"insertBlocks",
	"replaceText",
	"replaceSubtree",
	"updateBlock",
	"moveBlock",
	"deleteBlock",
	"setCell",
	"setProperties",
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
