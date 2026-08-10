package v2service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestSchemaOp(t *testing.T) {
	fx := newV2Fixture(t)

	t.Run("every op serves a strict schema and a single-op example", func(t *testing.T) {
		for _, op := range v2OpNames {
			// when
			entry, err := fx.SchemaOp(op)

			// then
			require.NoError(t, err, op)
			assert.Equal(t, op, entry.Kind)
			assert.Equal(t, v2OpsEndpoint, entry.Endpoint)

			var schema map[string]any
			require.NoError(t, json.Unmarshal(entry.Schema, &schema), "schema of %s must be valid JSON", op)
			assert.Equal(t, false, schema["additionalProperties"], "%s schema is C13-strict", op)

			// the example is a full single-op PATCH body whose op matches
			var example struct {
				Ops []struct {
					Op string `json:"op"`
				} `json:"ops"`
			}
			require.NoError(t, json.Unmarshal(entry.Example, &example), "example of %s must be valid JSON", op)
			require.Len(t, example.Ops, 1, "example of %s is single-op", op)
			assert.Equal(t, op, example.Ops[0].Op)
		}
	})

	t.Run("the op set and the schema map agree", func(t *testing.T) {
		assert.Len(t, v2OpSchemas, len(v2OpNames))
	})

	// §8.30: a field no value of which can succeed is not advertised. In a
	// NEW-content payload the op only ever CREATES, so an id slot there is an
	// error whatever the caller writes — and a constrained decoder emits the
	// fields it is shown, so a runtime guard alone cannot stop it.
	//
	// Table-driven over v2NewContentOps, the ONE set the runtime reads too
	// (decodePayloadRun): an op added to the runtime half and missed in the
	// schema half is what re-creates §8.30's bug, so neither half gets its
	// own list to fall out of date.
	t.Run("the new-content op set decides which schemas publish an id", func(t *testing.T) {
		for _, op := range v2OpNames {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err, op)

			owners := schemaPropertyOwners(t, entry.Schema, "id")
			if v2NewContentOps[op] {
				assert.Empty(t, owners, "%s only ever creates: no part of its schema may advertise an id", op)
				assert.NotContains(t, opBlockDefProps(t, entry), "id", "%s block def", op)
				continue
			}
			assert.NotEmpty(t, owners, "%s addresses existing content: its payload-block def keeps the id slot", op)
		}
	})

	t.Run("every new-content op is a real op", func(t *testing.T) {
		for op := range v2NewContentOps {
			assert.Contains(t, v2OpNames, op)
		}
	})

	// §8.31: the emptiness assertion above is only worth anything if the
	// nested slots are TYPED. They were not — columns/rows were bare
	// {"type":"array"} with no items — so "no nested id is published" held
	// because there was no nested schema at all, and would have passed
	// identically for replaceSubtree.
	t.Run("the nested table entries are typed, and their id slot follows the same split", func(t *testing.T) {
		for _, tc := range []struct {
			op       string
			nestedId bool
		}{
			{"insertBlocks", false},
			{"replaceSubtree", true},
		} {
			entry, err := fx.SchemaOp(tc.op)
			require.NoError(t, err, tc.op)
			for _, field := range []string{"columns", "rows"} {
				items := blockDefArrayItems(t, entry, field)
				require.NotNil(t, items, "%s: %s must publish an items def — an untyped array constrains nothing", tc.op, field)
				assert.Equal(t, false, items["additionalProperties"],
					"%s: %s entries are C13-strict, which is what makes an absent id unemittable", tc.op, field)
				props, _ := items["properties"].(map[string]any)
				require.NotEmpty(t, props, "%s: %s entries publish properties", tc.op, field)
				_, hasId := props["id"]
				assert.Equal(t, tc.nestedId, hasId, "%s: %s[].id", tc.op, field)
			}
		}
	})

	t.Run("no payload-block def publishes views", func(t *testing.T) {
		// the block def's own claim: neither shape has a `views` property, so
		// with additionalProperties:false no decoder can name a dataview view
		// through this channel at all
		for _, op := range []string{"insertBlocks", "replaceSubtree"} {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err, op)
			assert.NotContains(t, opBlockDefProps(t, entry), "views", op)
		}
	})

	t.Run("replaceSubtree keeps the id slot its payload needs", func(t *testing.T) {
		// naming the block it replaces is what makes echoing a read back a
		// no-op instead of a rename (§8.29)
		entry, err := fx.SchemaOp("replaceSubtree")

		require.NoError(t, err)
		assert.Contains(t, opBlockDefProps(t, entry), "id",
			"the existing-content block def keeps its id slot")
	})

	t.Run("unknown op lists the available ops", func(t *testing.T) {
		_, err := fx.SchemaOp("frobnicate")

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "replaceText")
	})

	t.Run("the index lists the ops", func(t *testing.T) {
		index := fx.SchemaIndex()

		require.Len(t, index.Ops, len(v2OpNames))
		assert.Equal(t, "setProperties", index.Ops[0].Kind)
		assert.Equal(t, "/v2/schemas/ops/setProperties", index.Ops[0].Url)
	})
}

// opBlockDefProps returns the property names an op schema's payload-block
// def publishes.
func opBlockDefProps(t *testing.T, entry v2model.SchemaEntry) map[string]any {
	t.Helper()
	var schema struct {
		Defs struct {
			Block struct {
				Properties map[string]any `json:"properties"`
			} `json:"block"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(entry.Schema, &schema))
	return schema.Defs.Block.Properties
}

// blockDefArrayItems returns the `items` def an op's payload-block def
// publishes for one array-valued property (columns, rows), or nil.
func blockDefArrayItems(t *testing.T, entry v2model.SchemaEntry, field string) map[string]any {
	t.Helper()
	prop, _ := opBlockDefProps(t, entry)[field].(map[string]any)
	items, _ := prop["items"].(map[string]any)
	return items
}

// schemaPropertyOwners walks a JSON Schema and reports every "properties"
// map in it that publishes the named field — unreferenced $defs included, so
// the assertion holds however the op wires its refs.
func schemaPropertyOwners(t *testing.T, raw json.RawMessage, field string) []string {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	var found []string
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch n := node.(type) {
		case map[string]any:
			if props, ok := n["properties"].(map[string]any); ok {
				if _, has := props[field]; has {
					found = append(found, path+".properties")
				}
			}
			for k, v := range n {
				walk(v, path+"."+k)
			}
		case []any:
			for i, v := range n {
				walk(v, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(doc, "")
	return found
}

func TestDiffEditDocs(t *testing.T) {
	t.Run("insertion does not mark following siblings moved", func(t *testing.T) {
		// given
		before := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"b","type":"paragraph","text":"b"}]}`)
		after := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"n","type":"paragraph","text":"n"},{"id":"b","type":"paragraph","text":"b"}]}`)
		want := v2model.DiffStats{BlocksAdded: 1}

		// when
		got, err := diffEditDocs(before, after)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("reorder among common siblings is moved", func(t *testing.T) {
		before := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"b","type":"paragraph","text":"b"}]}`)
		after := []byte(`{"blocks":[{"id":"b","type":"paragraph","text":"b"},{"id":"a","type":"paragraph","text":"a"}]}`)

		got, err := diffEditDocs(before, after)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksMoved: 2}, got)
	})

	t.Run("reparenting is moved, not changed", func(t *testing.T) {
		before := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"b","type":"paragraph","text":"b"}]}`)
		after := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"indent":1,"id":"b","type":"paragraph","text":"b"}]}`)

		got, err := diffEditDocs(before, after)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksMoved: 1}, got)
	})

	t.Run("property add, change and removal each count once", func(t *testing.T) {
		before := []byte(`{"properties":{"name":"Doc","done":false}}`)
		after := []byte(`{"properties":{"name":"Doc2","status":["x"]}}`)

		got, err := diffEditDocs(before, after)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{PropertiesChanged: 3}, got)
	})
}
