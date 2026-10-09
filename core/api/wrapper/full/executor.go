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
	"bytes"
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
func (e *Executor) Tools() []wrapper.ToolListing {
	out := make([]wrapper.ToolListing, 0, len(e.table.Tools))
	for _, t := range e.table.Tools {
		out = append(out, wrapper.ToolListing{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema, ReadOnly: t.ReadOnly, Destructive: t.Destructive})
	}
	return out
}

// the loop's contract, checked at compile time
var _ wrapper.Executor = (*Executor)(nil)

// Instructions is the full tier's initialize text (spec §3.5).
func (e *Executor) Instructions() string { return Instructions }

// Instructions is the full tier's workflow steering, short because the
// tools carry their own shapes.
const Instructions = "Anytype tools over the local API, one per operation. " +
	"Edits are op envelopes: patch_object and update_type take ops, a list of {op, …} entries applied in order. " +
	"Each op's members are listed in the tool's description; get_op_schema with the op name serves their value shapes and a worked example. " +
	"get_schema serves a full document kind's schema with a worked example (object, type_document, template, document); the shortcut bodies need no lookup. " +
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
	body = respellWarnings(body)
	return &wrapper.Result{Text: string(body), JSON: json.RawMessage(body)}, nil
}

// respellWarnings re-spells the routes in a success body's top-level
// warnings — the repair a warning names is a route on the REST surface and
// a tool here — and leaves every other byte of the body as the server
// wrote it. A body without warnings, or one this cannot read, is returned
// unchanged.
func respellWarnings(body []byte) []byte {
	start, end, ok := topLevelMember(body, "warnings")
	if !ok {
		return body
	}
	var warnings []map[string]json.RawMessage
	if err := json.Unmarshal(body[start:end], &warnings); err != nil {
		return body
	}
	changed := false
	for _, w := range warnings {
		var refs []v2model.Ref
		if raw, ok := w["see_also"]; ok {
			_ = json.Unmarshal(raw, &refs)
			delete(w, "see_also")
			changed = true
		}
		for _, field := range []string{"message", "hint"} {
			raw, ok := w[field]
			if !ok {
				continue
			}
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				continue
			}
			fieldRefs := refs
			if field == "message" {
				fieldRefs = nil // references are the hint's, by contract
			}
			if respelled := wrapper.RespellRefs(text, fieldRefs, Spelling); respelled != text {
				if encoded, err := json.Marshal(respelled); err == nil {
					w[field] = encoded
					changed = true
				}
			}
		}
	}
	if !changed {
		return body
	}
	encoded, err := json.Marshal(warnings)
	if err != nil {
		return body
	}
	out := make([]byte, 0, len(body)-(end-start)+len(encoded))
	out = append(out, body[:start]...)
	out = append(out, encoded...)
	return append(out, body[end:]...)
}

// topLevelMember finds the byte span of one member's value in a JSON
// object, without re-encoding anything around it.
func topLevelMember(body []byte, name string) (int, int, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return 0, 0, false
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return 0, 0, false
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return 0, 0, false
		}
		if key == name {
			end := int(dec.InputOffset())
			return end - len(value), end, true
		}
	}
	return 0, 0, false
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
			if !ok || !onePathSegment(s) {
				return req, fmt.Errorf("%s: %q takes an id, one path segment — not empty, not . or .., and no /", tool.Name, name)
			}
			req.Path = strings.ReplaceAll(req.Path, "{"+arg.Wire+"}", url.PathEscape(s))
		case known && arg.In == ArgQuery && arg.List:
			items, ok := scalarList(value)
			if !ok {
				return req, fmt.Errorf("%s: %q takes an array of strings, such as [\"status\",\"due_date\"]", tool.Name, name)
			}
			for _, s := range items {
				req.Query.Add(arg.Wire, s)
			}
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
			if arg.Wire == wrapper.IdempotencyKeyHeader {
				if !wrapper.ValidIdempotencyKey(s) {
					return req, fmt.Errorf("%s: %q takes at most %d visible ASCII characters", tool.Name, name, wrapper.MaxIdempotencyKeyLen)
				}
				req.IdempotencyKey = s
			} else {
				req.Headers.Set(arg.Wire, s)
			}
		case known:
			body[arg.Wire] = value
		case name == callerRetryKeyArg:
			// the retry key is the executor's, except where the overlay hands
			// it to the caller; an open body must not swallow it either
			return req, fmt.Errorf("%s does not take %q — the retry key is managed for you; arguments: %s", tool.Name, name, strings.Join(argNames(tool), ", "))
		case tool.HasBody && tool.OpenBody:
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

// onePathSegment reports whether s can fill one path parameter without
// changing the route: gin routes on the decoded path, so an escaped slash
// would still split the segment, and . or .. would be resolved away.
func onePathSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.Contains(s, "/")
}

// callerRetryKeyArg is the argument name the overlay can hand a caller.
const callerRetryKeyArg = "idempotency_key"

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

// scalarList renders an array argument's items for a query parameter the
// document declares as an exploded array, one value per item.
func scalarList(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := scalar(item)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
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
