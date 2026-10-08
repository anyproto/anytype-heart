package full

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

var update = flag.Bool("update", false, "rewrite the golden tools list")

const goldenPath = "testdata/tools_list.golden.json"

// realInputs loads the embedded document from disk and the op schemas from
// the service — the same inputs package api hands Derive.
func realInputs(t *testing.T) Inputs {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "v2", "openapi.json"))
	require.NoError(t, err)
	served, err := v2service.ServedOpSchemas()
	require.NoError(t, err)
	ops := make(map[string]OpSchema, len(served))
	for op, s := range served {
		ops[op] = OpSchema{Schema: s.Schema, Example: s.Example, Channels: s.Channels}
	}
	return Inputs{OpenAPI: doc, Ops: ops}
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
	in := realInputs(t)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(in.OpenAPI, &doc))
	component := doc["components"].(map[string]any)["schemas"].(map[string]any)["CreateChatRequest"].(map[string]any)
	component["properties"].(map[string]any)["space_id"] = map[string]any{"type": "string"}
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	in.OpenAPI = mutated

	_, err = Derive(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create_chat")
	assert.Contains(t, err.Error(), `"space_id"`)
	assert.Contains(t, err.Error(), "RenameArg")
}

// TestArgumentsPlaceEveryParameter: path parameters are required
// arguments, headers are spelled snake_case, body members route to the
// body, and the schema lists exactly the arguments.
func TestArgumentsPlaceEveryParameter(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("add_chat_message")
	require.True(t, ok)
	want := map[string]Arg{
		"space_id":        {Name: "space_id", In: ArgPath, Wire: "space_id", Required: true},
		"chat_id":         {Name: "chat_id", In: ArgPath, Wire: "chat_id", Required: true},
		"dry_run":         {Name: "dry_run", In: ArgQuery, Wire: "dry_run"},
		"idempotency_key": {Name: "idempotency_key", In: ArgHeader, Wire: "Idempotency-Key"},
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

// TestDocumentBodiesStayOpen: a body that is an AnyBlock document has no
// listed members and admits any; the description names where the shape is.
func TestDocumentBodiesStayOpen(t *testing.T) {
	table := deriveReal(t)
	for _, name := range []string{"create_template", "validate", "create_object", "create_type"} {
		tool, ok := table.Tool(name)
		require.True(t, ok, name)
		assert.True(t, tool.OpenBody, "%s takes a document form", name)
		var root struct {
			AdditionalProperties *bool `json:"additionalProperties"`
		}
		require.NoError(t, json.Unmarshal(tool.InputSchema, &root))
		assert.Nil(t, root.AdditionalProperties, "%s root must not refuse document members", name)
	}
	tool, _ := table.Tool("create_template")
	assert.Contains(t, tool.Description, "Body:")
	assert.Contains(t, tool.Description, "get_schema")
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

// envelopeTools maps each ops-bearing tool to its channel and the path
// arguments a call needs.
var envelopeTools = map[string]struct {
	channel string
	args    map[string]any
}{
	"patch_object": {channel: v2service.OpChannelObject, args: map[string]any{"space_id": "s", "object_id": "o"}},
	"update_type":  {channel: v2service.OpChannelType, args: map[string]any{"space_id": "s", "type": "t"}},
}

// TestTypedOpsEnvelope: each channel's envelope has exactly the channel's
// ops as branches, every served example validates AFTER embedding (refs
// rewritten, definitions deduplicated), and the shapes the server refuses
// are refused by the schema too.
func TestTypedOpsEnvelope(t *testing.T) {
	in := realInputs(t)
	table, err := Derive(in)
	require.NoError(t, err)
	for name, tc := range envelopeTools {
		t.Run(name, func(t *testing.T) {
			tool, ok := table.Tool(name)
			require.True(t, ok)
			compiled := compile(t, tool.InputSchema)

			var channelOps []string
			for op, s := range in.Ops {
				for _, c := range s.Channels {
					if c == tc.channel {
						channelOps = append(channelOps, op)
					}
				}
			}
			sort.Strings(channelOps)
			require.NotEmpty(t, channelOps)

			var branches []string
			for _, m := range opBranchRefs(t, tool.InputSchema) {
				branches = append(branches, strings.TrimPrefix(m, opBranchPrefix))
			}
			sort.Strings(branches)
			assert.Equal(t, channelOps, branches, "the envelope's branches are exactly the channel's ops")

			for _, op := range channelOps {
				var example map[string]any
				require.NoError(t, json.Unmarshal(in.Ops[op].Example, &example))
				call := withArgs(tc.args, map[string]any{"ops": []any{example}})
				assert.NoError(t, validate(t, compiled, call), "served example of %s must validate in the assembled tool schema", op)

				unknownField := withArgs(tc.args, map[string]any{"ops": []any{withArgs(example, map[string]any{"no_such_member": 1})}})
				assert.Error(t, validate(t, compiled, unknownField), "%s: an extra member must be refused", op)

				noOp := map[string]any{}
				for k, v := range example {
					if k != "op" {
						noOp[k] = v
					}
				}
				assert.Error(t, validate(t, compiled, withArgs(tc.args, map[string]any{"ops": []any{noOp}})), "%s: a missing op discriminator must be refused", op)
			}
			assert.Error(t, validate(t, compiled, withArgs(tc.args, map[string]any{"ops": []any{map[string]any{"op": "no_such_op"}}})), "an unknown op must be refused")
			assert.Error(t, validate(t, compiled, withArgs(tc.args, map[string]any{"ops": []any{}})), "an empty envelope must be refused")
		})
	}
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

// TestOpDefinitionsAreDeduplicated: a definition every op shares keeps its
// plain name once; one that differs between ops is namespaced per shape;
// nothing dangles.
func TestOpDefinitionsAreDeduplicated(t *testing.T) {
	table := deriveReal(t)
	tool, ok := table.Tool("patch_object")
	require.True(t, ok)
	var schema struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	_, plain := schema.Defs["anyValue"]
	assert.True(t, plain, "anyValue is identical on every op and keeps its plain name")
	var blockVariants []string
	for name := range schema.Defs {
		if name == "block" || strings.HasPrefix(name, "block"+opDefSeparator) {
			blockVariants = append(blockVariants, name)
		}
	}
	sort.Strings(blockVariants)
	assert.Equal(t, []string{"block__insert_blocks", "block__replace_subtree"}, blockVariants,
		"block differs between new-content and existing-content ops, and every op of one shape shares one definition")
	_, blockRefPlain := schema.Defs["blockRef"]
	assert.True(t, blockRefPlain, "blockRef is identical on every op and keeps its plain name")
	for _, ref := range allRefs(t, tool.InputSchema) {
		_, known := schema.Defs[strings.TrimPrefix(ref, defsRefPrefix)]
		assert.True(t, known, "dangling reference %s", ref)
	}
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
	// 110,938 bytes measured on 2026-10-08 for 50 tools; the ceiling leaves
	// a tenth of headroom.
	const ceiling = 120 << 10
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

// opBranchRefs returns the $defs names the envelope's oneOf references.
func opBranchRefs(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var out []string
	for _, ref := range allRefs(t, schema) {
		name := strings.TrimPrefix(ref, defsRefPrefix)
		if strings.HasPrefix(name, opBranchPrefix) {
			out = append(out, name)
		}
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
