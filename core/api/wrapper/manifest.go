// Package wrapper is the task-tool layer over the /v2 REST surface: one
// curated tool table, exposed
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
	// set_properties value shape). The one non-scalar arg type.
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
	// maxRefListLen bounds delete_block's comma-separated reference list —
	// room for a few dozen full 24-hex ids, far past any real edit, while
	// still C13-bounded.
	maxRefListLen = 1024
	// maxSortLen bounds update_view's sort string: at most 10 sorts (the
	// op's advertised sorts maxItems) of a key plus a direction — 1024 holds
	// several max-length keys and every realistic list, while staying
	// C13-bounded.
	maxSortLen = 1024
	// maxColumnListLen bounds update_view's comma-separated column list —
	// room for a screenful of max-length property keys (the per-call column
	// cap is enforced by count, not bytes).
	maxColumnListLen = 4096
	// maxTypePropertiesLen bounds create_type's property DDL: 32 properties
	// (maxTypeProperties) with names, formats and option lists fit
	// comfortably, while the string stays C13-bounded.
	maxTypePropertiesLen = 4096
)

// objectArgDescription is the shared reference-channel contract text.
const objectArgDescription = "the object: a handle number from the last find (1, 2, …) or a full object id"

// spaceArgDescription is the shared space-slot contract text. Every tool
// that addresses an object also accepts the space it lives in: without a
// space slot, a caller holding a space id put it in `object` instead — in
// the small-model eval 12 of 212 `object` arguments were the space id, and
// every one of them was on a tool that offered no space argument.
const spaceArgDescription = "the space the object is in — optional: needed only when no find has run yet, and ignored for a handle number, which the last find already places"

// spaceIdArgDescription is the space slot on the tools that address a SPACE
// and no object (find, describe, create, create_type). There is no handle to
// place them, so the space is REQUIRED — spaceArgDescription above describes
// the optional companion slot on object-addressing tools and is false here.
// Stated once so four tools cannot drift into four spellings of one
// argument.
const spaceIdArgDescription = "space id"

// blockArgDescription is the shared block-reference contract text.
const blockArgDescription = "a block label from read (5 chars) or a full block id"

// Tools returns the task-tool set — 14 tools, deliberately under the
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
			Name: "find",
			// the last sentence is the §8.33 listing, stated because the
			// description must not claim a bare space returns matches — not
			// because prose is expected to steer (arm B2 measured that it does
			// not); the behaviour change is what carries the fix
			Description: "Search objects in a space by query, type or filter. Returns numbered handles (1, 2, …) the other tools accept as `object`. Each find renumbers the handles. Given none of the three it matches nothing and lists the space instead — unnumbered, and not addressable.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: spaceIdArgDescription},
				{Name: "query", Type: ArgString, MaxLen: maxNameLen, Description: "full-text words to match"},
				{Name: "type", Type: ArgString, MaxLen: maxKeyLen, Description: "a type name, e.g. Task"},
				// the filter grammar's keys are identifiers (no spaces) — the
				// underscore-join teaching below is what keeps multi-word
				// property NAMES reachable from the compact string (the
				// server's fold resolves Due_date onto "Due date")
				{Name: "filter", Type: ArgString, MaxLen: maxFilterLen, Description: `compact filter string, e.g. Done = false AND Due_date < currentWeek() — string values in double quotes; write multi-word property names with underscores (Due_date); a name no identifier can spell (C++, 50% done) cannot be filtered here`},
				{Name: "limit", Type: ArgInteger, Min: 1, Max: 50, Description: "max results (default 10)"},
			},
			Example:  map[string]any{"space": "space1", "type": "Task", "filter": `Done = false`},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name: "read",
			// the last sentence states the list read (tools_list.go), because
			// the answer is a different SHAPE, not a longer document: a model
			// told only "read an object" has no reason to expect addressable
			// rows back
			Description: "Read an object. mode=full (the default) returns every block with its TEXT and the short label the editing tools take as `block` — this is the read an edit needs. mode=outline returns the same blocks with text truncated to 80 runes — a cheaper survey; copy exact text from mode=full. Reading a Query or Collection returns its definition (source type, filter, sort) and numbers its rows like find does, so they can be passed as `object`.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "mode", Type: ArgString, Enum: []string{"full", "outline"}, Description: "default full; outline truncates each block's text to 80 runes"},
			},
			Example:  map[string]any{"object": "1"},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "describe",
			Description: "Describe a type before creating or editing objects of it: its property names, formats, and live select option names. Call this first — property names and option names must match exactly. When a property shows more options than fit, ask again with options set to that property name.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: spaceIdArgDescription},
				{Name: "type", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "a type name, e.g. Task"},
				{Name: "options", Type: ArgString, MaxLen: maxNameLen, Description: "list ONE property's select options in full instead of describing the type, e.g. Status"},
				{Name: "starting_with", Type: ArgString, MaxLen: maxNameLen, Description: "with options: only options starting with this text"},
			},
			Example:  map[string]any{"space": "space1", "type": "Task"},
			Tier:     TierSmall,
			ReadOnly: true,
		},
		{
			Name:        "create",
			Description: "Create an object. properties uses the type's property names (describe first); markdown becomes the body. Date values accept today, tomorrow, +Nd, weekday names; @me means the calling user; object properties (assignee, related objects) accept a handle number from the last find or the object's exact name.",
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: spaceIdArgDescription},
				{Name: "type", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: "a type name, e.g. Task"},
				{Name: "name", Type: ArgString, Required: true, MaxLen: maxNameLen, Description: "object name"},
				{Name: "properties", Type: ArgObject, Description: "property name → value; select values are option NAMES"},
				{Name: "markdown", Type: ArgString, MaxLen: maxMarkdownLen, Description: "markdown body: headings, lists, - [ ] checkboxes, ``` fences, quotes, tables"},
			},
			Example: map[string]any{"space": "space1", "type": "Task", "name": "Prepare the Q3 report", "properties": map[string]any{"Due date": "friday"}},
			Tier:    TierSmall,
		},
		{
			Name: "set_properties",
			// One call takes SEVERAL objects as a comma-separated list, under
			// delete_block's separator rule and for the same measured reason:
			// a 4-call dependent chain scored 0/6 across every model until a
			// batch primitive collapsed it to 2 calls, which then scored 5/5.
			// "Mark these three done" must not be three calls a model can
			// drop the last of. Unlike delete_block's blocks, these are N
			// separate PATCHes — no cross-object transaction exists — so the
			// description promises per-object honesty rather than atomicity
			// (runSetProperties states the whole rule).
			Description: "Change property values on an object — or on several at once, passed as a comma-separated list. set replaces a value; add/remove edit list values (tags, assignees) without rewriting the whole list. Option names must exist (describe shows them). Values of object properties (assignee, related objects) take a handle number or the object's exact name as well as an id.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxRefListLen, Description: objectArgDescription + "; several objects separate with commas (\"1,2,3\") — the same change is written to each, one at a time"},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "set", Type: ArgObject, Description: "property name → new value"},
				{Name: "add", Type: ArgObject, Description: "list property name → entries to append"},
				{Name: "remove", Type: ArgObject, Description: "list property name → entries to delete"},
			},
			// the example shows the LIST form deliberately: the measured
			// failure is models splitting N property writes into N dependent
			// calls and dropping one, and delete_block's batch form went
			// unused until its own example showed it
			Example: map[string]any{"object": "1,2,3", "set": map[string]any{"Status": "Done"}},
			Tier:    TierSmall,
		},
		{
			Name:        "check_item",
			Description: "Check or uncheck one checkbox block. For completing a task-like OBJECT, set its done/status property with set_properties instead.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
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
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "markdown", Type: ArgString, Required: true, MaxLen: maxMarkdownLen, Description: "the content to add"},
				{Name: "after", Type: ArgString, MaxLen: maxRefLen, Description: "insert after this block (" + blockArgDescription + ")"},
				{Name: "under", Type: ArgString, MaxLen: maxRefLen, Description: "insert as last child of this block"},
			},
			Example: map[string]any{"object": "1", "after": "ab3f2", "markdown": "- [ ] follow up"},
			Tier:    TierSmall,
		},
		{
			Name:        "edit_text",
			Description: "Replace text inside one block: give a short exact snippet to find and its replacement. The snippet must occur exactly once in the block. block is optional — when omitted, the snippet itself must pin down exactly one block. Text is markdown source — ** [ ] etc. format.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "find", Type: ArgString, Required: true, MaxLen: maxFindLen, Description: "exact text to replace, as it appears in the block"},
				{Name: "replace", Type: ArgString, Required: true, AllowEmpty: true, MaxLen: maxFindLen, Description: "the new text — empty deletes the found text"},
				// block is OPTIONAL (§8.21): a required block id is unknowable
				// on turn one, and both benchmarked small models routed around
				// the tool entirely rather than call read first
				{Name: "block", Type: ArgString, MaxLen: maxRefLen, Description: blockArgDescription + "; optional — when omitted the find snippet locates the block, and must match exactly one"},
			},
			Example: map[string]any{"object": "1", "find": "Q3", "replace": "Q4"},
			Tier:    TierSmall,
		},
		{
			Name:        "set_cell",
			Description: "Write one table cell. row and col take the text read shows — a column's header, a row's first cell — or a row/column id; value replaces the cell's text.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "table", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "the table block (" + blockArgDescription + ")"},
				{Name: "row", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "the row's first-cell text from read, or a row id"},
				{Name: "col", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: "the column's header text from read (each column carries `header`), or a column id"},
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
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefLen, Description: blockArgDescription},
				{Name: "after", Type: ArgString, MaxLen: maxRefLen, Description: "place after this block"},
				{Name: "under", Type: ArgString, MaxLen: maxRefLen, Description: "place as last child of this block"},
			},
			Example: map[string]any{"object": "1", "block": "ab3f2", "under": "c81d0"},
			Tier:    TierLarge,
		},
		{
			Name: "delete_block",
			// One call takes SEVERAL blocks as a comma-separated list. The
			// list lives inside the existing `block` string rather than in an
			// array type or a second `blocks` slot, because the choice has to
			// hold in every place an arg materialises — JSON schema, GBNF,
			// CLI flag, validateArgs — and a string is the one shape all four
			// already render; an ArgArray would be a fifth arg type built for
			// one tool, and a second slot invites the both-given refusal from
			// small models, which fill every field they are shown. The comma
			// is safe as a separator: the reference vocabulary is hex labels,
			// 24-hex minted ids and fixed legacy names, none of which contain
			// one. Batching exists because of a measured chain-length cliff:
			// the restructure-section eval needs delete×3 + add, and across
			// 56 attempts models that reached all 3 deletes passed ~30/31
			// while those that stalled at 2 passed 0/8 — they can do the
			// task, they drop the third dependent call. One delete call
			// removes the chain.
			Description: "Delete one or several blocks — pass several as a comma-separated list; the call removes all of them or none. Deleting a block that has children needs recursive=true (the error names the child count).",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				{Name: "block", Type: ArgString, Required: true, MaxLen: maxRefListLen, Description: blockArgDescription + "; several blocks separate with commas (\"ab3f2,c81d0,e0001\") — one call deletes them all, or none"},
				{Name: "recursive", Type: ArgBoolean, Description: "also delete the blocks' children (default false)"},
			},
			// the example shows the batch form deliberately: the measured
			// failure is models splitting N deletes into N dependent calls
			// and dropping one — the one worked example is where they learn
			// the list exists
			Example: map[string]any{"object": "1", "block": "ab3f2,c81d0"},
			Tier:    TierLarge,
		},
		{
			Name: "update_view",
			// ONE tool for the three view channels, not three tools: the set
			// grows 12 → 13, still under the >15 cliff, and the three
			// arguments share every line of targeting. `filter` reuses the
			// compact syntax `find` already publishes — the model has been
			// taught this vocabulary, so the marginal learning cost is near
			// zero — and `sort` is ORDER BY's grammar for the same reason.
			Description: "Change how a dataview shows its objects: the filter (same compact syntax as find's filter), the sort order, and which property columns are visible — give at least one of filter, sort, columns. One call changes one view, atomically.",
			Args: []Arg{
				{Name: "object", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: objectArgDescription},
				{Name: "space", Type: ArgString, MaxLen: maxKeyLen, Description: spaceArgDescription},
				// the two targeting args mirror the served op schema's own
				// wording (schemas_ops.go v2ViewBlockPropDef and the update_view
				// view slot) — one resolution rule, one spelling of it
				{Name: "block", Type: ArgString, MaxLen: maxRefLen, Description: "a dataview block — optional when the object has exactly one (types, sets and collections do)"},
				{Name: "view", Type: ArgString, MaxLen: maxRefLen, Description: "view id, full or unique suffix — optional when the dataview has exactly one view"},
				{Name: "filter", Type: ArgString, MaxLen: maxFilterLen, Description: `compact filter string, e.g. Done = false AND Due_date < currentWeek() — string values in double quotes, multi-word property names written with underscores (Due_date); replaces the view's filters. Write "none" to REMOVE the filter and show everything again`},
				{Name: "sort", Type: ArgString, MaxLen: maxSortLen, Description: `the sort order: a property name with optional asc or desc ("Due date desc" — asc is the default), several separated by commas; replaces the view's sorts`},
				{Name: "columns", Type: ArgString, MaxLen: maxColumnListLen, Description: `the property columns to show, comma-separated ("Name,Status,Due date") — these become visible and every other visible column is hidden`},
			},
			Example: map[string]any{"object": "1", "filter": `Done = false`, "sort": "Due date desc"},
			Tier:    TierLarge,
		},
		{
			Name: "create_type",
			// THE FIRST SPEND of the >15-tool headroom (13 → 14). The set
			// stays under the cliff and the remaining room is now one tool
			// (measurements put 15-16 as comfortable, 20+ as risky), so the
			// next candidate has to be at least this good: the wrapper could
			// USE types and not make one, which left "set up a Recipe type
			// with ingredients, cook time and rating" unanswerable on this
			// surface — not harder, unanswerable.
			//
			// Listed LAST rather than beside `create`: the two are one word
			// apart and do entirely different things, so the description
			// opens by saying which is which and the table keeps them apart.
			Description: `Create a new object TYPE — the schema objects are then made from. This does not create an object: create does that. properties is a comma-separated list of "Name: format" pairs, a select or multi_select naming its options in parentheses — the same form describe prints, so a describe output can be handed straight back. Formats: text, number, select, multi_select, date, files, checkbox, url, email, phone, objects. A type cannot be renamed or deleted from this surface, so run describe or find first: the name and the properties are permanent.`,
			Args: []Arg{
				{Name: "space", Type: ArgString, Required: true, MaxLen: maxKeyLen, Description: spaceIdArgDescription},
				{Name: "name", Type: ArgString, Required: true, MaxLen: maxNameLen, Description: "the type's name, e.g. Cookbook entry"},
				{Name: "properties", Type: ArgString, MaxLen: maxTypePropertiesLen, Description: `the type's properties: comma-separated "Name: format" pairs, with a select's options in parentheses — "Cook time: number, Rating: select(Low, Medium, High), Source: url". A property name containing a comma, a colon or a parenthesis cannot be written here`},
			},
			// the example shows a SELECT WITH OPTIONS deliberately: C12's one
			// worked example is where a model learns a form exists at all —
			// delete_block's batch list went unused until its example showed
			// it. The type name is not "Recipe" for a measured reason: a dozen
			// everyday names (Recipe, Book, Movie, Project, Contact) are
			// reserved by bundled types and refused, and an example the server
			// rejects verbatim teaches the wrong thing.
			Example: map[string]any{"space": "space1", "name": "Cookbook entry", "properties": "Cook time: number, Rating: select(Low, Medium, High), Source: url"},
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
