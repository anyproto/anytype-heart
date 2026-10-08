package full

// executor.go — the one executor every full-table tool runs through (spec
// §4): arguments go to the path, the query, the headers or the body by the
// tool's placement table; a mutation carries an Idempotency-Key, minted
// when the caller sent none and reused across the client's own retries;
// the response passes through as text and as structured JSON; a refusal
// comes back in-band with the server's envelope, its typed references
// re-spelled as the tools of this table — which are the operations
// themselves, so the spelling is the operation's name with its bound
// arguments.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// Executor runs the full table's tools over a client.
type Executor struct {
	client *wrapper.Client
	table  *Table
	// newKey mints an Idempotency-Key; tests pin it.
	newKey func() (string, error)
}

// NewExecutor builds an executor over a client (whose transport and bearer
// are the caller's business) and a derived table.
func NewExecutor(client *wrapper.Client, table *Table) *Executor {
	return &Executor{client: client, table: table, newKey: randomKey}
}

// Tools lists the table for tools/list.
func (e *Executor) Tools() []ListEntry { return e.table.List() }

// Instructions is the full tier's initialize text (spec §3.5).
func (e *Executor) Instructions() string { return Instructions }

// Instructions is the full tier's workflow steering, short because the
// tools carry their own shapes.
const Instructions = "Anytype tools over the local API, one per operation. " +
	"Edits are op envelopes: patch_object and update_type take ops, a list of {op, …} entries applied in order; the tool schema lists every op's members. " +
	"get_op_schema serves one op's schema with a worked example and get_schema a document kind's (object, type, template). " +
	"dry_run: true previews any write and reports what it would change. " +
	"Every error says how to fix the call — follow it and retry once."

// Run executes one tool call.
func (e *Executor) Run(ctx context.Context, name string, args map[string]any) (*wrapper.Result, error) {
	tool, ok := e.table.Tool(name)
	if !ok {
		return nil, wrapper.ArgumentError{Text: fmt.Sprintf("unknown tool %q — tools: %s", name, strings.Join(e.table.Names(), ", "))}
	}
	req, err := e.assemble(tool, args)
	if err != nil {
		return nil, wrapper.ArgumentError{Text: err.Error()}
	}
	status, body, err := e.client.DoRaw(ctx, req)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, refusal(status, body)
	}
	if len(body) == 0 {
		return &wrapper.Result{Text: "ok"}, nil
	}
	return &wrapper.Result{Text: string(body), JSON: json.RawMessage(body)}, nil
}

// assemble places the arguments on the wire.
func (e *Executor) assemble(tool Tool, args map[string]any) (wrapper.RawRequest, error) {
	req := wrapper.RawRequest{Method: tool.Method, Path: tool.Path, Query: url.Values{}, Headers: http.Header{}}
	body := map[string]any{}
	for name, value := range args {
		arg, known := tool.Arg(name)
		switch {
		case known && arg.In == ArgPath:
			s, ok := scalar(value)
			if !ok || s == "" {
				return req, fmt.Errorf("%s: %q takes a non-empty string", tool.Name, name)
			}
			req.Path = strings.ReplaceAll(req.Path, "{"+arg.Wire+"}", url.PathEscape(s))
		case known && arg.In == ArgQuery:
			s, ok := scalar(value)
			if !ok {
				return req, fmt.Errorf("%s: %q takes a scalar", tool.Name, name)
			}
			req.Query.Set(arg.Wire, s)
		case known && arg.In == ArgHeader:
			s, ok := scalar(value)
			if !ok {
				return req, fmt.Errorf("%s: %q takes a string", tool.Name, name)
			}
			if arg.Wire == "Idempotency-Key" {
				req.IdempotencyKey = s
			} else {
				req.Headers.Set(arg.Wire, s)
			}
		case known || (tool.HasBody && tool.OpenBody):
			body[name] = value
		default:
			return req, fmt.Errorf("%s does not take %q — arguments: %s", tool.Name, name, strings.Join(argNames(tool), ", "))
		}
	}
	for _, a := range tool.Args {
		if _, present := args[a.Name]; a.Required && !present {
			return req, fmt.Errorf("%s needs %q", tool.Name, a.Name)
		}
	}
	if strings.Contains(req.Path, "{") {
		return req, fmt.Errorf("%s: a path argument is missing in %s", tool.Name, req.Path)
	}
	if tool.HasBody {
		encoded, err := json.Marshal(body)
		if err != nil {
			return req, fmt.Errorf("encode body: %w", err)
		}
		req.Body = encoded
	} else if len(body) > 0 {
		return req, fmt.Errorf("%s takes no body", tool.Name)
	}
	if mutates(tool.Method) && req.IdempotencyKey == "" {
		key, err := e.newKey()
		if err != nil {
			return req, fmt.Errorf("mint idempotency key: %w", err)
		}
		req.IdempotencyKey = key
	}
	return req, nil
}

// mutates reports whether a method is a write the idempotency store keys.
func mutates(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// scalar renders a scalar argument for the path, the query or a header.
func scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case json.Number:
		return x.String(), true
	}
	return "", false
}

func argNames(tool Tool) []string {
	names := make([]string, 0, len(tool.Args))
	for _, a := range tool.Args {
		names = append(names, a.Name)
	}
	return names
}

// randomKey mints a 128-bit Idempotency-Key.
func randomKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

//
// ---- refusals ----
//

// refusal turns a non-2xx body into the in-band tool error. A C6 envelope
// is rendered with its references re-spelled in this table's vocabulary;
// any other body degrades to its text.
func refusal(status int, body []byte) error {
	var envelope v2model.Error
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Message == "" {
		return &wrapper.ToolError{Status: status, Text: fmt.Sprintf("server answered %d: %s", status, strings.TrimSpace(string(body)))}
	}
	issues := make([]v2model.Issue, len(envelope.Issues))
	for i, issue := range envelope.Issues {
		issue.Message = wrapper.RespellRefs(issue.Message, nil, Spelling)
		issue.Hint = wrapper.RespellRefs(issue.Hint, issue.SeeAlso, Spelling)
		issue.SeeAlso = nil
		issues[i] = issue
	}
	message := wrapper.RespellRefs(envelope.Message, nil, Spelling)
	return &wrapper.ToolError{
		Status:  status,
		Code:    envelope.Code,
		Message: message,
		Issues:  issues,
		Text:    wrapper.RenderErrorText(message, issues),
	}
}

// Spelling is the full table's vocabulary: an operation is its own tool,
// named with the arguments the reference binds; a resend reference is the
// same call again with the parameter set.
func Spelling(ref v2model.Ref) string {
	if ref.Op == "" {
		return "this call again with " + bindings(ref.Query)
	}
	var b strings.Builder
	b.WriteString("`")
	b.WriteString(ref.Op)
	b.WriteString("`")
	bound := map[string]string{}
	for k, v := range ref.Params {
		bound[k] = v
	}
	for k, v := range ref.Query {
		bound[k] = v
	}
	if len(bound) > 0 {
		b.WriteString(" with ")
		b.WriteString(bindings(bound))
	}
	return b.String()
}

// bindings renders name: value pairs, sorted by name.
func bindings(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + m[k]
	}
	return strings.Join(parts, ", ")
}
