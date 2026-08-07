// Package wrapper is the §7 task-tool layer over the /v2 REST surface
// (APIV2.md §7, the Phase-5 deliverable): one curated tool table, exposed
// as CLI verbs (cmd/anytype), as a machine-readable function-calling
// manifest, and as an MCP stdio server for local models (mcp.go,
// tier-filtered per tier.go/§8.20) — ONE definition, three deliveries.
//
// The wrapper is deliberately NOT a 1:1 re-export of the REST endpoints
// (the documented anti-pattern). Each tool picks the channel a small model
// can drive: markdown-in for authoring (add_blocks, create), anchor-in for
// editing (edit_text), enumerated handles and short block labels for
// reference (find/read — resolved wrapper-side so the model never echoes a
// 24-hex id). The wrapper owns the machinery a small model cannot author:
// Idempotency-Key derivation and retries; If-Match is deliberately omitted
// (the etag advances on background sync — a 409 the model cannot reason
// about; C7 advisory mode is the contract here).
//
// Packaging decision (recorded): the manifest is Go data in this package —
// versioned in lockstep with the op names, routes and error texts it wraps;
// the CLI imports it (verbs are generated from the same table, so verb set
// == tool set by construction). Tools call the API over localhost HTTP —
// the CLI is out-of-process by nature, and HTTP keeps the single
// enforcement point (auth, rate limit, the C8 idempotency store, analytics
// all live in server middleware; in-process service calls would bypass the
// idempotency store the retry machinery depends on). Two surfaces, not
// three: the wrapper stays a client of /v2.
package wrapper

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// ArgType is a tool argument's JSON type (flat, C13: no recursion).
type ArgType string

const (
	ArgString  ArgType = "string"
	ArgInteger ArgType = "integer"
	ArgBoolean ArgType = "boolean"
	// ArgObject is a flat string-keyed map of scalar/array values (the
	// setProperties value shape). The one non-scalar arg type.
	ArgObject ArgType = "object"
)

// Arg is one tool argument. The same definition renders the JSON schema,
// the GBNF grammar, and the CLI flag.
type Arg struct {
	Name     string
	Type     ArgType
	Required bool
	// AllowEmpty marks a required string whose EMPTY value is meaningful
	// (edit_text.replace deletes the found text, set_cell.value clears the
	// cell): Required then means "present", not "non-empty".
	AllowEmpty  bool
	Description string
	Enum        []string // string args only
	MaxLen      int      // string args: schema maxLength
	Min, Max    int      // integer args: schema bounds
}

// Tool is one task tool: the single source for the manifest entry, the CLI
// verb, the schema, the grammar and the executor lookup.
type Tool struct {
	Name        string
	Description string
	Args        []Arg
	// Example is the one minimal worked example (C12).
	Example map[string]any
	// Tier is the smallest model tier this tool is served to (tier.go):
	// TierSmall tools reach both tiers, TierLarge tools only the large one.
	// Every tool must declare a tier (pinned by test).
	Tier Tier
	// ReadOnly marks a tool that never mutates (spaces, find, read,
	// describe) — surfaced as the MCP readOnlyHint annotation so hosts can
	// skip write confirmation.
	ReadOnly bool
}

// Verb is the tool's CLI verb (underscores become hyphens: set_properties →
// set-properties).
func (t Tool) Verb() string { return strings.ReplaceAll(t.Name, "_", "-") }

// arg returns the named argument definition.
func (t Tool) arg(name string) (Arg, bool) {
	for _, a := range t.Args {
		if a.Name == name {
			return a, true
		}
	}
	return Arg{}, false
}

const (
	maxNameLen     = 4096
	maxMarkdownLen = 1 << 20
	maxKeyLen      = 256
	maxRefLen      = 64
	maxFindLen     = 65536
	maxFilterLen   = 4096 // the filterstring parser's own input bound
)

// objectArgDescription is the shared reference-channel contract text.
const objectArgDescription = "the object: a handle number from the last find (1, 2, …) or a full object id"

// blockArgDescription is the shared block-reference contract text.
const blockArgDescription = "a block label from read (5 chars) or a full block id"

// Tools returns the task-tool set — 12 tools, deliberately under the
// >15-tool small-model cliff. Order is the documentation order.
func Tools() []Tool {
	return []Tool{
		{
			Name:        "spaces",
			Description: "List the spaces (name and id). Run this first when no space id is known — find, describe and create take a space id from here.",
			Args: []Arg{
				{Name: "limit", Type: ArgInteger, Min: 1, Max: 100, Description: "max spaces (default 25)"},
			},
			Example:  map[string]any{"limit": 25},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "find",
			Description: "Search objects in a space. Returns numbered handles (1, 2, …) the other tools accept as `object`. Each find renumbers the handles.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "space id"},
				{Name: "query", Type: ArgString, MaxLen: maxNameLen, Description: "full-text words to match"},
				{Name: "type", Type: ArgString, MaxLen: maxKeyLen, Description: "a type key, e.g. task"},
				{Name: "filter", Type: ArgString, MaxLen: maxFilterLen, Description: `compact filter string, e.g. done = false AND dueDate < currentWeek() — string values in double quotes`},
				{Name: "limit", Type: ArgInteger, Min: 1, Max: 50, Description: "max results (default 10)"},
			},
			Example:  map[string]any{"space": "space1", "type": "task", "filter": `done = false`},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "read",
			Description: "Read an object. mode=outline lists every block as {indent, id, type} (text on headings) — cheap orientation; mode=full returns the whole document with short block labels the editing tools accept.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "mode", Type: ArgString, Enum: []string{"full", "outline"}, Description: "default full"},
			},
			Example:  map[string]any{"object": "1", "mode": "outline"},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "describe",
			Description: "Describe a type before creating or editing objects of it: its property keys, formats, and live select option names. Call this first — property keys and option names must match exactly.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "space id"},
				{Name: "type", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "a type key, e.g. task"},
			},
			Example:  map[string]any{"space": "space1", "type": "task"},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "create",
			Description: "Create an object. properties uses the type's property keys (describe first); markdown becomes the body. Date values accept today, tomorrow, +Nd, weekday names; @me means the calling user.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "space id"},
				{Name: "type", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "a type key, e.g. task"},
				{Name: "name", Type: ArgString, Required: true, MaxLen: maxNameLen, Description: "object name"},
				{Name: "properties", Type: ArgObject, Description: "property key → value; select values are option NAMES"},
				{Name: "markdown", Type: ArgString, MaxLen: maxMarkdownLen, Description: "markdown body: headings, lists, - [ ] checkboxes, ``` fences, quotes, tables"},
			},
			Example: map[string]any{"space": "space1", "type": "task", "name": "Prepare the Q3 report", "properties": map[string]any{"dueDate": "friday"}},
			Tier:    TierSmall,
		},
		{
			Name:        "set_properties",
			Description: "Change an object's property values. set replaces a value; add/remove edit list values (tags, assignees) without rewriting the whole list. Option names must exist (describe shows them).",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "set", Type: ArgObject, Description: "property key → new value"},
				{Name: "add", Type: ArgObject, Description: "list property key → entries to append"},
				{Name: "remove", Type: ArgObject, Description: "list property key → entries to delete"},
			},
			Example: map[string]any{"object": "1", "set": map[string]any{"status": "Done"}},
			Tier:    TierSmall,
		},
		{
			Name:        "check_item",
			Description: "Check or uncheck one checkbox block. For completing a task-like OBJECT, set its done/status property with set_properties instead.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: blockArgDescription},
				{Name: "checked", Type: ArgBoolean, Required: true, Description: "true to check the box, false to uncheck"},
			},
			Example: map[string]any{"object": "1", "block": "ab3f2", "checked": true},
			Tier:    TierLarge,
		},
		{
			Name:        "add_blocks",
			Description: "Add content to an object, written as markdown (headings, lists, - [ ] checkboxes, ``` fences, quotes, tables). Omit after/under to append at the end of the document.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "markdown", Type: ArgString, Required: true, MaxLen: maxMarkdownLen, Description: "the content to add"},
				{Name: "after", Type: ArgString, MaxLen: maxRefLen, Description: "insert after this block (" + blockArgDescription + ")"},
				{Name: "under", Type: ArgString, MaxLen: maxRefLen, Description: "insert as last child of this block"},
			},
			Example: map[string]any{"object": "1", "after": "ab3f2", "markdown": "- [ ] follow up"},
			Tier:    TierSmall,
		},
		{
			Name:        "edit_text",
			Description: "Replace text inside one block: give a short exact snippet to find and its replacement. The snippet must occur exactly once in the block. Text is markdown source — ** [ ] etc. format.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: blockArgDescription},
				{Name: "find", Type: ArgString, Required: true, MaxLen: maxFindLen, Description: "exact text to replace, as it appears in the block"},
				{Name: "replace", Type: ArgString, Required: true, AllowEmpty: true, MaxLen: maxFindLen, Description: "the new text — empty deletes the found text"},
			},
			Example: map[string]any{"object": "1", "block": "ab3f2", "find": "Q3", "replace": "Q4"},
			Tier:    TierSmall,
		},
		{
			Name:        "set_cell",
			Description: "Write one table cell. row and col are the labels shown in read; value replaces the cell's text.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "table", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "the table block (" + blockArgDescription + ")"},
				{Name: "row", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "a row label from read, or a full row id"},
				{Name: "col", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "a column label from read, or a full column id"},
				{Name: "value", Type: ArgString, Required: true, AllowEmpty: true, MaxLen: maxFindLen, Description: "the new cell text — empty clears the cell"},
			},
			Example: map[string]any{"object": "1", "table": "t9d2c", "row": "row2", "col": "col1", "value": "done"},
			Tier:    TierLarge,
		},
		{
			Name:        "move_block",
			Description: "Move a block (with its children). Give after (a sibling) or under (the new parent); omit both to move it to the end of the document.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: blockArgDescription},
				{Name: "after", Type: ArgString, MaxLen: maxRefLen, Description: "place after this block"},
				{Name: "under", Type: ArgString, MaxLen: maxRefLen, Description: "place as last child of this block"},
			},
			Example: map[string]any{"object": "1", "block": "ab3f2", "under": "c81d0"},
			Tier:    TierLarge,
		},
		{
			Name:        "delete_block",
			Description: "Delete a block. Deleting a block that has children needs recursive=true (the error names the child count).",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: blockArgDescription},
				{Name: "recursive", Type: ArgBoolean, Description: "also delete the block's children (default false)"},
			},
			Example: map[string]any{"object": "1", "block": "ab3f2"},
			Tier:    TierLarge,
		},
	}
}

// ToolByName looks a tool up by manifest name.
func ToolByName(name string) (Tool, bool) {
	for _, t := range Tools() {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// ToolByVerb looks a tool up by CLI verb.
func ToolByVerb(verb string) (Tool, bool) {
	for _, t := range Tools() {
		if t.Verb() == verb {
			return t, true
		}
	}
	return Tool{}, false
}

// ToolNames returns the manifest names in documentation order.
func ToolNames() []string {
	tools := Tools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

//
// ---- the machine-readable manifest ----
//

// ManifestTool is one manifest entry: the common function-calling
// denominator (name/description/parameters) plus the C12 example and the
// per-tool GBNF grammar (§7.3 item 2). Example is pre-rendered JSON in the
// GRAMMAR's key order (required args in declared order, then optional) — a
// Go map would serialize alphabetically, making the served example a string
// the served grammar rejects; a test matches every example against its own
// grammar.
type ManifestTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Example     json.RawMessage `json:"example"`
	GBNF        string          `json:"gbnf"`
}

// FilterGrammar is the compact filter-string grammar artifact served with
// the manifest: the pinned EBNF (SPEC §6.2.1), a GBNF transcription for
// constrained decoding, and worked examples. It constrains to canonical
// case/ASCII keys; the parser itself is more lenient.
type FilterGrammar struct {
	EBNF     string   `json:"ebnf"`
	GBNF     string   `json:"gbnf"`
	Examples []string `json:"examples"`
}

// Manifest is the machine-readable delivery: the same tool table the CLI
// verbs are generated from.
type Manifest struct {
	Version       int            `json:"version"`
	Tools         []ManifestTool `json:"tools"`
	FilterGrammar FilterGrammar  `json:"filterGrammar"`
}

// BuildManifest assembles the full (large-tier) manifest from the tool
// table.
func BuildManifest() (Manifest, error) {
	return BuildManifestForTier(TierLarge)
}

// BuildManifestForTier assembles the manifest for one tier (tier.go): the
// same table, filtered — never a second definition.
func BuildManifestForTier(tier Tier) (Manifest, error) {
	tools := ToolsForTier(tier)
	entries := make([]ManifestTool, 0, len(tools))
	for _, t := range tools {
		schema, err := toolSchema(t)
		if err != nil {
			return Manifest{}, fmt.Errorf("schema for tool %s: %w", t.Name, err)
		}
		example, err := exampleJSON(t)
		if err != nil {
			return Manifest{}, fmt.Errorf("example for tool %s: %w", t.Name, err)
		}
		entries = append(entries, ManifestTool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
			Example:     example,
			GBNF:        toolGBNF(t),
		})
	}
	return Manifest{
		Version: 1,
		Tools:   entries,
		FilterGrammar: FilterGrammar{
			EBNF:     filterstring.EBNF,
			GBNF:     filterStringGBNF,
			Examples: filterstring.Examples,
		},
	}, nil
}

// gbnfArgOrder returns a tool's args in the key order its GBNF pins:
// required args in declared order, then optional args in declared order.
func gbnfArgOrder(t Tool) []Arg {
	ordered := make([]Arg, 0, len(t.Args))
	for _, a := range t.Args {
		if a.Required {
			ordered = append(ordered, a)
		}
	}
	for _, a := range t.Args {
		if !a.Required {
			ordered = append(ordered, a)
		}
	}
	return ordered
}

// exampleJSON renders the C12 example with its keys in gbnfArgOrder — the
// one order the tool's grammar accepts.
func exampleJSON(t Tool) (json.RawMessage, error) {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, a := range gbnfArgOrder(t) {
		v, ok := t.Example[a.Name]
		if !ok {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		key, err := json.Marshal(a.Name)
		if err != nil {
			return nil, fmt.Errorf("encode example key %s: %w", a.Name, err)
		}
		b.Write(key)
		b.WriteByte(':')
		val, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("encode example value of %s: %w", a.Name, err)
		}
		b.Write(val)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String()), nil
}

// ManifestJSON renders the full (large-tier) manifest as compact JSON (C3).
func ManifestJSON() ([]byte, error) {
	return ManifestJSONForTier(TierLarge)
}

// ManifestJSONForTier renders one tier's manifest as compact JSON (C3).
func ManifestJSONForTier(tier Tier) ([]byte, error) {
	m, err := BuildManifestForTier(tier)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return data, nil
}
