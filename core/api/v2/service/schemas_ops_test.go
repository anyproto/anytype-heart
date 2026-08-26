package v2service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
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

			// the example is one op object whose op matches — not a request body
			var example map[string]any
			require.NoError(t, json.Unmarshal(entry.Example, &example), "example of %s must be valid JSON", op)
			assert.Equal(t, op, example["op"])
			assert.NotContains(t, example, "ops", "the example of %s is an op, not a PATCH body", op)
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
	// identically for replace_subtree.
	t.Run("the nested table entries are typed, and their id slot follows the same split", func(t *testing.T) {
		for _, tc := range []struct {
			op       string
			nestedId bool
		}{
			{"insert_blocks", false},
			{"replace_subtree", true},
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
		for _, op := range []string{"insert_blocks", "replace_subtree"} {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err, op)
			assert.NotContains(t, opBlockDefProps(t, entry), "views", op)
		}
	})

	t.Run("replace_subtree keeps the id slot its payload needs", func(t *testing.T) {
		// naming the block it replaces is what makes echoing a read back a
		// no-op instead of a rename (§8.29)
		entry, err := fx.SchemaOp("replace_subtree")

		require.NoError(t, err)
		assert.Contains(t, opBlockDefProps(t, entry), "id",
			"the existing-content block def keeps its id slot")
	})

	// §8.32: the payload block's `type` published no vocabulary — a bare
	// string beside a description naming another fetch, which a decoder cannot
	// make. Asked for a checkbox item, gemma4:e2b answered
	// {"type":"bulleted_list_item","text":"[ ] Follow up"} 10 times out of 10.
	// The enum is derived, never copied: a hand-kept list is the drift class
	// §8.31 was about.
	t.Run("both payload block defs publish the block-type vocabulary", func(t *testing.T) {
		for _, op := range []string{"insert_blocks", "replace_subtree"} {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err, op)

			got := publishedBlockTypes(t, entry)
			assert.Equal(t, anyblockjson.AuthorableBlockTypeNames(), got,
				"%s: the published enum IS the format's authorable vocabulary", op)
			assert.Contains(t, got, "checkbox", "the type the measured failure needed")
		}
	})

	t.Run("the published vocabulary offers no value that cannot become a block", func(t *testing.T) {
		// §7 structural types are in the format's vocabulary but import
		// absorbs or drops them — the §8.30 rule, one level down: a VALUE no
		// caller can succeed with does not appear in the enum
		entry, err := fx.SchemaOp("insert_blocks")
		require.NoError(t, err)
		published := publishedBlockTypes(t, entry)
		require.NotEmpty(t, published)
		for _, typ := range anyblockjson.BlockTypeNames() {
			if anyblockjson.StructuralBlockType(typ) {
				assert.NotContains(t, published, typ, "%s is structural (SPEC §7)", typ)
			}
		}
	})

	// The MIRROR of the block-type assertion above, and the reason it exists:
	// the 1.3 slug cascade re-spelled iconEmoji/iconImage to icon_emoji/
	// icon_image in v2OpBlockCommonProps — block ATTRIBUTE names, which
	// ADDRESSING §7.5a-4 excludes from the respelling by name. Both payload
	// defs are additionalProperties:false, so a grammar-constrained decoder
	// could not author a callout icon at all, while GET /v2/schemas/object
	// went on serving the format's own schema declaring iconEmoji — two served
	// schemas contradicting each other one request apart, and no test failed.
	//
	// The op schemas publish a SUBSET of the block shape, so every property
	// name they publish must exist in the format's own block schema. That
	// turns the exclusion from prose into something the build enforces.
	t.Run("every published block property exists in the format's block schema", func(t *testing.T) {
		for _, op := range []string{"insert_blocks", "replace_subtree"} {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err, op)

			for name := range opBlockDefProps(t, entry) {
				if name == "indent" || name == "id" {
					continue // op-payload addressing, not a block attribute
				}
				assert.True(t, anyblockjson.KnownBlockProperty(name),
					"%s publishes %q, which the format's block schema does not know — "+
						"with additionalProperties:false no document can ever carry it", op, name)
			}
		}
	})

	t.Run("the format's block schema knows the attributes the exclusion names", func(t *testing.T) {
		// the guard above is only worth anything if the format's inventory is
		// really read from the schema. §2b collapsed the flat icon pair into
		// ONE typed member, so the names that must resolve are the typed ones
		// — and the flat pair must NOT, or the op schema could publish a
		// member no document can carry and this guard would not notice.
		assert.True(t, anyblockjson.KnownBlockProperty("icon"))
		assert.True(t, anyblockjson.KnownBlockProperty("icon_size"))
		assert.False(t, anyblockjson.KnownBlockProperty("icon_emoji"))
		assert.False(t, anyblockjson.KnownBlockProperty("icon_image"))
		// §2e likewise: a property block names its property under `property`
		assert.True(t, anyblockjson.KnownBlockProperty("property"))
		assert.False(t, anyblockjson.KnownBlockProperty("key"))
	})

	t.Run("unknown op lists the available ops", func(t *testing.T) {
		_, err := fx.SchemaOp("frobnicate")

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "replace_text")
	})

	t.Run("the index lists the ops", func(t *testing.T) {
		index := fx.SchemaIndex()

		require.Len(t, index.Ops, len(v2OpNames))
		assert.Equal(t, "set_properties", index.Ops[0].Kind)
		assert.Equal(t, "/v2/schemas/ops/set_properties", index.Ops[0].Url)
	})
}

// TestOpVocabularyIsSnakeCase pins C2 on the op set — the most agent-visible
// strings in the API, since a model types one on every edit. Both halves
// matter: the served set must be snake_case, and the pre-rename camelCase
// spelling must be REFUSED rather than quietly still accepted, so a caller
// written against the old vocabulary fails loudly at the first op instead of
// half-working. There is no GBNF to keep in step here (the wrapper's grammars
// constrain tool ARGUMENTS, never op names), so this is the pin that stands
// in for the grammar-acceptance guard the tool surface has.
func TestOpVocabularyIsSnakeCase(t *testing.T) {
	t.Run("every served op name is snake_case", func(t *testing.T) {
		for _, op := range v2OpNames {
			assert.Regexp(t, `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`, op)
		}
	})

	t.Run("the pre-rename camelCase spelling is refused", func(t *testing.T) {
		ctx := context.Background()
		for _, op := range v2OpNames {
			camel := snakeToCamelForTest(op)
			require.NotEqual(t, op, camel, "every op name must have an underscore to have been renamed")

			t.Run(camel, func(t *testing.T) {
				// given
				fx := newV2Fixture(t)
				fx.expectMutate(editRead(t, editBaseDoc))

				// when
				_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
					patchBody(`{"op":"`+camel+`"}`), "", false, true)

				// then
				apiErr := v2Err(t, err)
				assert.Contains(t, apiErr.Message, `unknown op "`+camel+`"`)
			})
		}
	})
}

// snakeToCamelForTest spells a snake_case op the way v2 spelled it before the
// C2 rename ("set_properties" → "setProperties") — the string the server must
// now refuse.
func snakeToCamelForTest(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// TestServedOpExampleValidatesAgainstItsOwnSchema is the pin the wrapper
// manifest has had all along (TestExamplesAcceptedByOwnGBNF: every served
// example must be in the language of the served grammar) and this route did
// not: the two halves of GET /v2/schemas/ops/{op} were at DIFFERENT levels —
// a `schema` describing one op (additionalProperties:false, `op` required
// with a const) beside an `example` that was a whole {"ops":[…]} request
// body, so the example the route showed was rejected by the schema it showed
// it with. Table-driven over every op, so a new op cannot land with an
// example its own schema refuses.
func TestServedOpExampleValidatesAgainstItsOwnSchema(t *testing.T) {
	fx := newV2Fixture(t)

	for _, op := range v2OpNames {
		t.Run(op, func(t *testing.T) {
			// given
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err)

			// when
			err = validateAgainstSchema(t, entry.Schema, entry.Example)

			// then
			assert.NoError(t, err, "the served example must be an instance of the served schema")
		})
	}
}

// TestExampleValidatorIsHonest: the validator above must be able to reject,
// or the pin proves nothing. Both historical shapes fail it — the wrapped
// request body, and an op object missing the `op` discriminator (the field
// gemma4:e4b omitted on 9 of 60 calls when shown the wrapped example).
func TestExampleValidatorIsHonest(t *testing.T) {
	fx := newV2Fixture(t)
	entry, err := fx.SchemaOp("insert_blocks")
	require.NoError(t, err)

	assert.Error(t, validateAgainstSchema(t, entry.Schema,
		json.RawMessage(`{"ops":[`+string(entry.Example)+`]}`)), "the old wrapped shape must fail")
	assert.Error(t, validateAgainstSchema(t, entry.Schema,
		json.RawMessage(`{"after":"b3","markdown":"- [ ] todo"}`)), "a missing op discriminator must fail")
}

// TestMatchLocatorIsPublishedExactlyWhereItWorks ties the two halves of the
// `match` locator together the way §8.30 ties the payload id slot: the ops
// whose SCHEMA publishes `match` and the ops whose DECODER accepts it must
// be the same set. Schema-without-runtime advertises a field no value of
// which can succeed (a constrained decoder emits what it is shown);
// runtime-without-schema hides a channel from the only consumers that read
// the schema. The list is derived from the served schemas, so neither half
// carries a literal that can fall out of date — and the probe goes through
// PatchObject, so what is tested is the real strict decoder.
func TestMatchLocatorIsPublishedExactlyWhereItWorks(t *testing.T) {
	ctx := context.Background()
	schemas := newV2Fixture(t)

	for _, op := range v2OpNames {
		t.Run(op, func(t *testing.T) {
			entry, err := schemas.SchemaOp(op)
			require.NoError(t, err)
			published := len(schemaPropertyOwners(t, entry.Schema, "match")) > 0

			fx := newV2Fixture(t)
			fx.expectMutate(editRead(t, editBaseDoc))
			_, err = fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(fmt.Sprintf(`{"op":%q,"match":"no block says this"}`, op)), "", false, true)

			// the probe carries nothing but a locator that matches nothing, so
			// every op refuses SOMEHOW — the question is only whether it
			// refuses the FIELD
			require.Error(t, err, op)
			apiErr := v2Err(t, err)
			rejectedTheField := false
			for _, issue := range apiErr.Issues {
				if strings.Contains(issue.Message, `unknown field "match"`) {
					rejectedTheField = true
				}
			}
			assert.Equal(t, published, !rejectedTheField,
				"%s: the served schema and the decoder must agree about `match`", op)
		})
	}
}

// TestLocatorOpsDropTheRequiredId is the schema half of 2.1b: an op that
// accepts a locator cannot go on REQUIRING an id, or a decoder constrained
// to the schema can never write the locator form at all — which is the whole
// point of the field.
func TestLocatorOpsDropTheRequiredId(t *testing.T) {
	fx := newV2Fixture(t)

	for _, op := range []string{"update_block", "delete_block"} {
		t.Run(op, func(t *testing.T) {
			entry, err := fx.SchemaOp(op)
			require.NoError(t, err)

			var schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(entry.Schema, &schema))

			assert.NotContains(t, schema.Required, "id", "id is one of two addressing channels now")
			assert.Contains(t, schema.Properties, "id", "and still published")
			assert.Contains(t, schema.Properties, "match")
			assert.Equal(t, v2OpMatchPropDef, `"match":`+string(schema.Properties["match"]),
				"one published def serves every locator op — a second spelling is the §8.31 drift class")
		})
	}
}

// validateAgainstSchema compiles a served JSON Schema and validates one
// served example against it.
func validateAgainstSchema(t *testing.T, schema, instance json.RawMessage) error {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	require.NoError(t, err)
	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("op.schema.json", doc))
	compiled, err := c.Compile("op.schema.json")
	require.NoError(t, err)
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	require.NoError(t, err)
	return compiled.Validate(value)
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

// publishedBlockTypes returns the block-type enum an op's payload-block def
// publishes.
func publishedBlockTypes(t *testing.T, entry v2model.SchemaEntry) []string {
	t.Helper()
	typeProp, _ := opBlockDefProps(t, entry)["type"].(map[string]any)
	require.NotNil(t, typeProp, "the block def types `type`")
	values, _ := typeProp["enum"].([]any)
	require.NotEmpty(t, values, "`type` publishes its vocabulary")
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.(string))
	}
	return out
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
