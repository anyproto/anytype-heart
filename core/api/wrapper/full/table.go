// Package full is the full tool table: one tool per /v2 operation, derived
// at runtime from the embedded OpenAPI document and the served op schemas
// (docs/superpowers/specs/2026-09-18-full-wrapper-in-heart-design.md §2).
//
// It is the second table beside the curated one in package wrapper, and it
// is shaped for capable models: tool names are operationIds, arguments are
// the operation's path, query and header parameters plus its body members,
// flat, and results pass through. There are no handles, no session store
// and no grammar — the curated machinery stays where it is.
//
// Nothing in this package reaches the server or the service layer: Derive
// takes the embedded document and returns a table, so the table cannot
// drift from what a REST caller is told. The large schemas — each op's
// members, the AnyBlock documents — are lookups the model makes with
// get_op_schema and get_schema, as the npm bridge serves them.
package full

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Inputs is what Derive builds the table from.
type Inputs struct {
	// OpenAPI is the v2 document (core/api/docs/v2/openapi.json). Its
	// request bodies are the body contract: core/api/v2/service's
	// openapibodies.go has spliced the small discovery schemas into it, and
	// left the large ones — each op's members, the AnyBlock document forms —
	// as lookups: an ops envelope types an op by name and points at
	// get_op_schema, a document body is open and points at get_schema with
	// its kind. The table serves them as the document does.
	OpenAPI []byte
	// OpMembers is each op's members as the op list in a tool description
	// shows them — required ones bare, optional ones marked ?, see
	// OpMembers — keyed by op name. Nil leaves the list bare; set, every op
	// an envelope names must be in it.
	OpMembers map[string][]string
}

// ArgIn says where an argument goes on the wire.
type ArgIn string

const (
	ArgPath   ArgIn = "path"
	ArgQuery  ArgIn = "query"
	ArgHeader ArgIn = "header"
	ArgBody   ArgIn = "body"
)

// Arg is one tool argument and its wire placement. Body members the schema
// names are listed so the executor can tell a misplaced parameter from a
// body member; on an open body (a document form) members beyond the listed
// ones route to the body as well.
type Arg struct {
	// Name is the argument as the tool shows it (snake_case).
	Name string
	In   ArgIn
	// Wire is the name on the wire: the path or query parameter, the HTTP
	// header, or the body member.
	Wire     string
	Required bool
	// List marks a query parameter declared as an array: the caller sends
	// an array and the wire repeats the parameter per item (style form,
	// exploded — the OpenAPI default for a query array).
	List bool
}

// Tool is one derived tool.
type Tool struct {
	Name        string
	Description string
	Method      string
	// Path is the route template with {param} placeholders.
	Path string
	Args []Arg
	// InputSchema is the tool's JSON Schema: the arguments as properties,
	// the body's constraints hoisted, shared shapes under $defs.
	InputSchema json.RawMessage
	ReadOnly    bool
	Destructive bool
	// HasBody says the operation takes a JSON body; OpenBody says the body
	// accepts members beyond the ones Args lists (a document form).
	HasBody  bool
	OpenBody bool
}

// Arg returns the named argument.
func (t Tool) Arg(name string) (Arg, bool) {
	for _, a := range t.Args {
		if a.Name == name {
			return a, true
		}
	}
	return Arg{}, false
}

// Table is the derived tool table.
type Table struct {
	Tools []Tool
	// Excluded maps each operationId the overlay leaves out to its reason.
	Excluded map[string]string
	byName   map[string]int
}

// Tool returns the tool named name.
func (t *Table) Tool(name string) (Tool, bool) {
	i, ok := t.byName[name]
	if !ok {
		return Tool{}, false
	}
	return t.Tools[i], true
}

// Names returns the tool names in table order.
func (t *Table) Names() []string {
	names := make([]string, len(t.Tools))
	for i, tool := range t.Tools {
		names[i] = tool.Name
	}
	return names
}

// ListEntry is one tools/list entry in the MCP Tool shape.
type ListEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations *Annotations    `json:"annotations,omitempty"`
}

// Annotations are the MCP tool hints the table sets: advisory, enforcing
// nothing.
type Annotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint,omitempty"`
	DestructiveHint bool `json:"destructiveHint,omitempty"`
}

// List renders the table as tools/list entries.
func (t *Table) List() []ListEntry {
	out := make([]ListEntry, 0, len(t.Tools))
	for _, tool := range t.Tools {
		entry := ListEntry{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema}
		if tool.ReadOnly || tool.Destructive {
			entry.Annotations = &Annotations{ReadOnlyHint: tool.ReadOnly, DestructiveHint: tool.Destructive}
		}
		out = append(out, entry)
	}
	return out
}

// ListJSON renders the tools/list result object as compact JSON — the
// artifact the size ceiling and the golden snapshot measure.
func (t *Table) ListJSON() ([]byte, error) {
	data, err := json.Marshal(map[string]any{"tools": t.List()})
	if err != nil {
		return nil, fmt.Errorf("encode tools list: %w", err)
	}
	return data, nil
}

func (t *Table) index() {
	t.byName = make(map[string]int, len(t.Tools))
	for i, tool := range t.Tools {
		t.byName[tool.Name] = i
	}
}

// sortedKeys returns a map's keys sorted, for deterministic output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
