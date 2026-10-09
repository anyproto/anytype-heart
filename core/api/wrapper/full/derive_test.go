package full

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

var update = flag.Bool("update", false, "rewrite the golden tools list")

const goldenPath = "testdata/tools_list.golden.json"

// realInputs loads the embedded document from disk — the input package api
// hands Derive.
func realInputs(t *testing.T) Inputs {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "v2", "openapi.json"))
	require.NoError(t, err)
	served, err := v2service.ServedOpSchemas()
	require.NoError(t, err)
	members := map[string][]string{}
	for name, op := range served {
		m, err := OpMembers(op.Schema)
		require.NoError(t, err, name)
		members[name] = m
	}
	return Inputs{OpenAPI: doc, OpMembers: members}
}

func deriveReal(t *testing.T) *Table {
	t.Helper()
	table, err := Derive(realInputs(t))
	require.NoError(t, err)
	return table
}

// TestEveryOperationIsDerivedOrExcluded is the coverage contract: the
// document's operations, as the pinned operation table lists them, are each
// a tool or an exclusion with a reason — never silently absent.
func TestEveryOperationIsDerivedOrExcluded(t *testing.T) {
	table := deriveReal(t)
	derived := map[string]bool{}
	for _, name := range table.Names() {
		derived[name] = true
	}
	for _, id := range v2model.OperationIds() {
		_, excluded := table.Excluded[id]
		assert.True(t, derived[id] != excluded, "operation %q must be exactly one of derived and excluded (derived=%v excluded=%v)", id, derived[id], excluded)
	}
	for id := range table.Excluded {
		assert.NotEmpty(t, table.Excluded[id], "exclusion of %q needs a reason", id)
		assert.Contains(t, v2model.OperationIds(), id)
	}
	assert.Len(t, derived, len(v2model.OperationIds())-len(table.Excluded))
}

// TestOverlayTargetsExist: an overlay row for an operation or an argument
// the document no longer has fails derivation, so overlay rot is loud.
func TestOverlayTargetsExist(t *testing.T) {
	t.Run("unknown operation", func(t *testing.T) {
		in := realInputs(t)
		overlays["no_such_operation"] = overlay{Exclude: "test"}
		defer delete(overlays, "no_such_operation")
		_, err := Derive(in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"no_such_operation"`)
	})
	t.Run("unknown argument", func(t *testing.T) {
		in := realInputs(t)
		prev := overlays["list_spaces"]
		overlays["list_spaces"] = overlay{DropArg: []string{"no_such_arg"}}
		defer func() { overlays["list_spaces"] = prev }()
		_, err := Derive(in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"no_such_arg"`)
	})
}

// TestBodyParameterCollisionFails: a body member spelled like a parameter
// cannot be flattened; the error names the overlay rename that fixes it.
// The real document has no such member, so one is injected.
func TestBodyParameterCollisionFails(t *testing.T) {
	in := injectChatBodyMember(t, realInputs(t))

	_, err := Derive(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create_chat")
	assert.Contains(t, err.Error(), `"space_id"`)
	assert.Contains(t, err.Error(), "RenameBodyMember")
}

// TestArgumentsPlaceEveryParameter: path parameters are required
// arguments, headers are spelled snake_case, body members route to the
// body, and the schema lists exactly the arguments.
func TestArgumentsPlaceEveryParameter(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("add_chat_message")
	require.True(t, ok)
	want := map[string]Arg{
		"space_id": {Name: "space_id", In: ArgPath, Wire: "space_id", Required: true},
		"chat_id":  {Name: "chat_id", In: ArgPath, Wire: "chat_id", Required: true},
		"dry_run":  {Name: "dry_run", In: ArgQuery, Wire: "dry_run"},
	}
	for name, w := range want {
		got, ok := tool.Arg(name)
		require.True(t, ok, name)
		assert.Equal(t, w, got)
	}
	text, ok := tool.Arg("text")
	require.True(t, ok)
	assert.Equal(t, ArgBody, text.In)
	assert.True(t, tool.HasBody)
	assert.False(t, tool.OpenBody, "a strict body stays strict")

	var schema struct {
		Properties           map[string]any `json:"properties"`
		Required             []string       `json:"required"`
		AdditionalProperties *bool          `json:"additionalProperties"`
	}
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	var names []string
	for _, a := range tool.Args {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	assert.Equal(t, names, sortedKeys(schema.Properties))
	assert.Contains(t, schema.Required, "space_id")
	assert.Contains(t, schema.Required, "chat_id")
	require.NotNil(t, schema.AdditionalProperties)
	assert.False(t, *schema.AdditionalProperties)
}

// TestEveryArgumentNameIsLegal: tool hosts accept property keys matching
// ^[a-zA-Z0-9_.-]{1,64}$ only.
func TestEveryArgumentNameIsLegal(t *testing.T) {
	table := deriveReal(t)
	for _, tool := range table.Tools {
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
		for name := range schema.Properties {
			assert.Regexp(t, legalArgName, name, "%s.%s", tool.Name, name)
		}
	}
}

// TestOnlyTheReactionToggleTakesARetryKey: the executor keys every write
// itself, so the caller's retry key is exposed on exactly the tool whose
// blind replay is not harmless — toggling a reaction twice removes it.
func TestOnlyTheReactionToggleTakesARetryKey(t *testing.T) {
	table := deriveReal(t)
	var takers []string
	for _, tool := range table.Tools {
		if arg, ok := tool.Arg(idempotencyKeyArg); ok {
			takers = append(takers, tool.Name)
			assert.Equal(t, ArgHeader, arg.In)
			assert.Equal(t, wrapper.IdempotencyKeyHeader, arg.Wire)
		}
		if tool.Name != "toggle_chat_reaction" {
			assert.NotContains(t, string(tool.InputSchema), `"idempotency_key"`, "%s: not in its schema either", tool.Name)
		}
	}
	assert.Equal(t, []string{"toggle_chat_reaction"}, takers)
}

// TestAtLeastOneBodyMemberSurvivesFlattening: a body's minProperties:1
// would be met by a path argument once flattened, so it becomes "one of
// the body members is present".
func TestAtLeastOneBodyMemberSurvivesFlattening(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("update_widget")
	require.True(t, ok)
	compiled := compile(t, tool.InputSchema)
	assert.Error(t, validate(t, compiled, map[string]any{"space_id": "s", "widget_id": "w"}), "path arguments alone are not a patch")
	assert.Error(t, validate(t, compiled, map[string]any{"space_id": "s", "widget_id": "w", "scope": "space"}), "a query argument is not a body member either")
	assert.NoError(t, validate(t, compiled, map[string]any{"space_id": "s", "widget_id": "w", "limit": 6}))
	assert.NotContains(t, string(tool.InputSchema), `"minProperties":1,"properties"`)
}

// TestARenameRepairsACollision: a body member spelled like a parameter is
// fixed by renaming the member; the tool shows the new name and the call
// sends the wire name.
func TestARenameRepairsACollision(t *testing.T) {
	ex, api := newExecutorFixture(t)
	in := injectChatBodyMember(t, realInputs(t))
	prev, had := overlays["create_chat"]
	overlays["create_chat"] = overlay{RenameBodyMember: map[string]string{"space_id": "body_space_id"}}
	defer func() {
		if had {
			overlays["create_chat"] = prev
		} else {
			delete(overlays, "create_chat")
		}
	}()
	table, err := Derive(in)
	require.NoError(t, err)
	tool, ok := table.Tool("create_chat")
	require.True(t, ok)
	arg, ok := tool.Arg("body_space_id")
	require.True(t, ok)
	assert.Equal(t, Arg{Name: "body_space_id", In: ArgBody, Wire: "space_id"}, arg)

	ex.table = table
	_, err = ex.Run(context.Background(), "create_chat", map[string]any{"space_id": "s", "name": "x", "body_space_id": "inner"})
	require.NoError(t, err)
	assert.Equal(t, "/v2/spaces/s/chats", api.requests[0].Path)
	assert.JSONEq(t, `{"name":"x","space_id":"inner"}`, api.requests[0].Body)

	t.Run("a rename of a member the body lacks is overlay rot", func(t *testing.T) {
		overlays["create_chat"] = overlay{RenameBodyMember: map[string]string{"no_such_member": "x"}}
		_, err := Derive(realInputs(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"no_such_member"`)
	})
}

// injectChatBodyMember adds a space_id member to CreateChatRequest.
func injectChatBodyMember(t *testing.T, in Inputs) Inputs {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(in.OpenAPI, &doc))
	component := doc["components"].(map[string]any)["schemas"].(map[string]any)["CreateChatRequest"].(map[string]any)
	component["properties"].(map[string]any)["space_id"] = map[string]any{"type": "string"}
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	in.OpenAPI = mutated
	return in
}

// TestADanglingTransitiveReferenceFails: a component that references one
// the document lacks fails the derivation instead of serving a $ref no
// host can resolve.
func TestADanglingTransitiveReferenceFails(t *testing.T) {
	in := realInputs(t)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(in.OpenAPI, &doc))
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	schemas["Intermediate"] = map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"$ref": "#/components/schemas/NoSuchComponent"}}}
	schemas["CreateChatRequest"].(map[string]any)["properties"].(map[string]any)["extra"] = map[string]any{"$ref": "#/components/schemas/Intermediate"}
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	in.OpenAPI = mutated

	_, err = Derive(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Intermediate")
	assert.Contains(t, err.Error(), `"NoSuchComponent"`)
}

// TestLazyDerivesOnce: a failed derivation is kept, not retried.
func TestLazyDerivesOnce(t *testing.T) {
	calls := 0
	lazy := NewLazy(func() (*Table, error) {
		calls++
		return nil, fmt.Errorf("broken input")
	})
	_, err1 := lazy.Get()
	_, err2 := lazy.Get()
	require.Error(t, err1)
	assert.Equal(t, err1, err2)
	assert.Equal(t, 1, calls)
}

// TestUploadFileTakesTheJSONForm: with both JSON and multipart bodies, the
// JSON one is derived.
func TestUploadFileTakesTheJSONForm(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("upload_file")
	require.True(t, ok)
	url, ok := tool.Arg("url")
	require.True(t, ok)
	assert.Equal(t, ArgBody, url.In)
	assert.True(t, url.Required)
	_, hasFile := tool.Arg("file")
	assert.False(t, hasFile)
}

// TestAnnotationsBySemantics: read-only POSTs and destructive PATCHes carry
// the hint the method alone would not give.
func TestAnnotationsBySemantics(t *testing.T) {
	table := deriveReal(t)
	cases := map[string]struct{ readOnly, destructive bool }{
		"list_spaces":   {true, false},
		"search_space":  {true, false},
		"search_global": {true, false},
		"validate":      {true, false},
		"delete_object": {false, true},
		"patch_object":  {false, true},
		"update_type":   {false, true},
		"create_object": {false, false},
	}
	for name, want := range cases {
		tool, ok := table.Tool(name)
		require.True(t, ok, name)
		assert.Equal(t, want.readOnly, tool.ReadOnly, "%s readOnly", name)
		assert.Equal(t, want.destructive, tool.Destructive, "%s destructive", name)
	}
}

// compile compiles a tool's input schema with the validator the discovery
// tests use.
func compile(t *testing.T, schema json.RawMessage) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	require.NoError(t, err)
	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("tool.schema.json", doc))
	compiled, err := c.Compile("tool.schema.json")
	require.NoError(t, err)
	return compiled
}

func validate(t *testing.T, compiled *jsonschema.Schema, instance any) error {
	t.Helper()
	raw, err := json.Marshal(instance)
	require.NoError(t, err)
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	require.NoError(t, err)
	return compiled.Validate(value)
}

// TestUpdateTypeBodyModesAreExclusive: the flat body and the ops envelope
// are alternatives; a rename alone passes, a rename beside ops does not.
func TestUpdateTypeBodyModesAreExclusive(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("update_type")
	require.True(t, ok)
	compiled := compile(t, tool.InputSchema)
	base := map[string]any{"space_id": "s", "type": "t"}
	assert.NoError(t, validate(t, compiled, withArgs(base, map[string]any{"name": "Renamed"})), "a flat rename")
	assert.NoError(t, validate(t, compiled, withArgs(base, map[string]any{"ops": []any{map[string]any{"op": "remove_property", "property": "x"}}})), "an envelope")
	assert.Error(t, validate(t, compiled, withArgs(base, map[string]any{"name": "Renamed", "ops": []any{map[string]any{"op": "remove_property", "property": "x"}}})), "flat members beside ops")
}

// TestGoldenToolsList pins the served tools/list. Run with -update after an
// intended change and read the diff: it is the review artifact of this
// table. The measured size is printed so the ceiling can be set from it.
func TestGoldenToolsList(t *testing.T) {
	table := deriveReal(t)
	got, err := table.ListJSON()
	require.NoError(t, err)
	var pretty bytes.Buffer
	require.NoError(t, json.Indent(&pretty, got, "", " "))
	t.Logf("full tier: %d tools, tools/list %d bytes compact", len(table.Tools), len(got))
	if *update {
		require.NoError(t, os.WriteFile(goldenPath, pretty.Bytes(), 0o644))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "no golden yet: run with -update")
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(pretty.Bytes())) {
		assert.Fail(t, "the derived tools/list differs from the golden — review the diff, then run with -update", "")
		_ = os.WriteFile(goldenPath+".actual", pretty.Bytes(), 0o644)
	}
}

// TestToolsListSizeCeiling guards the budget. The ceiling is a regression
// guard set from the first measurement, not an acceptability claim.
func TestToolsListSizeCeiling(t *testing.T) {
	// 62,135 bytes measured for 50 tools with the large schemas served as
	// lookups and the retry key on the reaction toggle only; 69,753 once
	// the edit tools list each op's members, the document bodies type their
	// containers and search publishes its members. The ceiling leaves about
	// a tenth of headroom.
	const ceiling = 76 << 10
	table := deriveReal(t)
	got, err := table.ListJSON()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got), ceiling, "tools/list grew past the ceiling; raise it deliberately")
}

func withArgs(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// allRefs lists every $ref value in a schema.
func allRefs(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var node any
	require.NoError(t, json.Unmarshal(schema, &node))
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				out = append(out, ref)
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(node)
	return out
}

var _ = fmt.Sprintf

// TestNoToolHasADanglingReference: every $ref in every tool resolves in
// that tool's own $defs.
func TestNoToolHasADanglingReference(t *testing.T) {
	table := deriveReal(t)
	for _, tool := range table.Tools {
		var schema struct {
			Defs map[string]json.RawMessage `json:"$defs"`
		}
		require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
		for _, ref := range allRefs(t, tool.InputSchema) {
			_, ok := schema.Defs[strings.TrimPrefix(ref, defsRefPrefix)]
			assert.True(t, ok, "%s: dangling %s", tool.Name, ref)
		}
		compile(t, tool.InputSchema)
	}
}

// pointer finds a lookup in served prose: "<tool> with kind <kind>" or
// "<tool> with its op name".
var (
	kindPointer = regexp.MustCompile(`\b([a-z_]+) with kind ([a-z_]+)\b`)
	opPointer   = regexp.MustCompile(`\b([a-z_]+) with its op name\b`)
)

// openForms returns the parts of a tool's input schema that admit members
// beyond the listed ones: the root of an open body, or the open alternative
// of a body with alternatives.
func openForms(t *testing.T, tool Tool) []map[string]any {
	t.Helper()
	var root map[string]any
	require.NoError(t, json.Unmarshal(tool.InputSchema, &root))
	var out []map[string]any
	if branches, ok := root["anyOf"].([]any); ok {
		for _, b := range branches {
			if m, ok := b.(map[string]any); ok && m["additionalProperties"] != false {
				out = append(out, m)
			}
		}
		return out
	}
	if root["additionalProperties"] != false {
		out = append(out, root)
	}
	return out
}

// TestEveryOpenBodyPointsAtALookupThatResolves: a body the tool does not
// list carries a pointer to get_schema with a kind; the tool is in the
// table and the service serves the kind. This is what makes an open body a
// lookup rather than a guess (the bridge's measured trade: 9 of 10 callers
// fetched the schema first; an opaque body with no pointer drew 6 of 10
// blind invalid writes).
func TestEveryOpenBodyPointsAtALookupThatResolves(t *testing.T) {
	table := deriveReal(t)
	pointed := map[string]string{}
	for _, tool := range table.Tools {
		forms := openForms(t, tool)
		assert.Equal(t, tool.OpenBody, len(forms) > 0, "%s: OpenBody agrees with the schema", tool.Name)
		for _, form := range forms {
			desc, _ := form["description"].(string)
			if desc == "" {
				desc = tool.Description
			}
			m := kindPointer.FindStringSubmatch(desc)
			require.NotNil(t, m, "%s: an open body names its lookup: %q", tool.Name, desc)
			_, ok := table.Tool(m[1])
			assert.True(t, ok, "%s points at %s, which the table must serve", tool.Name, m[1])
			assert.Equal(t, "get_schema", m[1])
			_, err := v2service.ServedKindEntry(m[2])
			assert.NoError(t, err, "%s points at kind %q, which get_schema must serve", tool.Name, m[2])
			pointed[tool.Name] = m[2]
		}
	}
	assert.Equal(t, map[string]string{"create_object": "object", "create_template": "template",
		"create_type": "type_document", "validate": "document"}, pointed)
}

// TestOpMembers: required members first in the schema's order, then the
// optional ones sorted and marked, op left out.
func TestOpMembers(t *testing.T) {
	got, err := OpMembers(json.RawMessage(`{"properties":{"op":{},"view":{},"block":{},"set":{},"columns":{}},"required":["op","view"]}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"view", "block?", "columns?", "set?"}, got)

	_, err = OpMembers(json.RawMessage(`{"properties":{"op":{}},"required":["op","view"]}`))
	assert.Error(t, err, "a required member the schema does not define")
}

// TestOpEnvelopesAreLookups: each ops envelope types an op by name only —
// the enum is exactly the channel's served op set — leaves the members
// open, and points at get_op_schema, which serves every op in the enum.
func TestOpEnvelopesAreLookups(t *testing.T) {
	served, err := v2service.ServedOpSchemas()
	require.NoError(t, err)
	table := deriveReal(t)
	for name, channel := range map[string]string{"patch_object": v2service.OpChannelObject, "update_type": v2service.OpChannelType} {
		t.Run(name, func(t *testing.T) {
			tool, ok := table.Tool(name)
			require.True(t, ok)
			var schema struct {
				Properties struct {
					Ops struct {
						Description string `json:"description"`
						Items       struct {
							AdditionalProperties any      `json:"additionalProperties"`
							Required             []string `json:"required"`
							Properties           struct {
								Op struct {
									Enum []string `json:"enum"`
								} `json:"op"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"ops"`
				} `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
			ops := schema.Properties.Ops
			var want []string
			for op, s := range served {
				for _, c := range s.Channels {
					if c == channel {
						want = append(want, op)
					}
				}
			}
			got := append([]string(nil), ops.Items.Properties.Op.Enum...)
			sort.Strings(got)
			sort.Strings(want)
			assert.Equal(t, want, got, "the op enum is exactly the channel's served ops")
			assert.Equal(t, true, ops.Items.AdditionalProperties, "an op's members are open")
			assert.Equal(t, []string{"op"}, ops.Items.Required)
			m := opPointer.FindStringSubmatch(ops.Description)
			require.NotNil(t, m, "the envelope names its lookup: %q", ops.Description)
			_, ok = table.Tool(m[1])
			assert.True(t, ok, "%s is a tool of the table", m[1])
			getOp, _ := table.Tool("get_op_schema")
			_, takesOp := getOp.Arg("op")
			assert.True(t, takesOp, "get_op_schema takes the op name as op")
			// a model reads the description first: it lists the ops and
			// says their members are a lookup
			for _, op := range ops.Items.Properties.Op.Enum {
				m, err := OpMembers(served[op].Schema)
				require.NoError(t, err)
				assert.Contains(t, tool.Description, op+"{"+strings.Join(m, ", ")+"}")
			}
			assert.Contains(t, tool.Description, "get_op_schema")
		})
	}
	// the bodies with a document form beside a shortcut name its lookup in
	// the description too
	for name, kind := range map[string]string{"create_object": "object", "create_type": "type_document"} {
		tool, ok := table.Tool(name)
		require.True(t, ok)
		assert.Contains(t, tool.Description, "get_schema with kind "+kind, name)
	}
	if channels := map[string]int{}; true {
		for _, s := range served {
			for _, c := range s.Channels {
				channels[c]++
			}
		}
		assert.Equal(t, map[string]int{v2service.OpChannelObject: 15, v2service.OpChannelType: 7}, channels)
	}
}
