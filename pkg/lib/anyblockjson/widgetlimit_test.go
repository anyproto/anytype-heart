package anyblockjson

// widgetlimit_test.go — the widget `limit` bound has ONE home.
//
// The index's flat widget caps `limit` at 100 — a deliberate product bound
// on sidebar listings (the corpus maximum is 50), enforced again by the
// widget-object lift, which keeps the whole document for anything above it.
// The widget BLOCK deliberately takes the whole int32 range instead: the
// block is the fidelity fallback for exactly the widget the index refuses,
// so the cap must not bind there or the fallback document would fail its own
// schema. Every sibling widget field (`layout`, `card_style`, `icon_size`,
// `description`) states its vocabulary once in the object schema and the
// index references it; `limit` inlined its own copy in the index, which is
// the drift channel this file closes: the cap now lives in
// `$defs/widgetListingLimit`, the index references it, the Go lift reads the
// same number through maxIndexWidgetLimit, and this test ties all four
// statements together so no one of them can move alone.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodedSchema parses one embedded schema for structural assertions.
func decodedSchema(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

// widgetBlockBranch finds the object schema's widget-block conditional and
// returns its `then.properties`, failing loudly when the dispatch reshuffles.
func widgetBlockBranch(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	branches, ok := schemaAt(t, schema, "$defs", "blockCore", "allOf").([]any)
	require.True(t, ok, "the block dispatch is an allOf of if/then branches")
	for _, b := range branches {
		branch, ok := b.(map[string]any)
		if !ok {
			continue
		}
		ifClause, ok := branch["if"].(map[string]any)
		if !ok {
			continue
		}
		if c, _ := schemaAt(t, ifClause, "properties", "type").(map[string]any); c != nil &&
			c["const"] == "widget" {
			props, ok := schemaAt(t, branch, "then", "properties").(map[string]any)
			require.True(t, ok)
			return props
		}
	}
	require.Fail(t, "no widget branch in the block dispatch")
	return nil
}

func TestWidgetLimit_OneStatementOfTheCap(t *testing.T) {
	object := decodedSchema(t, SchemaJSON())
	index := decodedSchema(t, indexSchemaJSON)
	authoringIndex := decodedSchema(t, authoringIndexSchemaJSON)

	t.Run("the shared def carries the cap the Go lift enforces", func(t *testing.T) {
		def, ok := schemaAt(t, object, "$defs", "widgetListingLimit").(map[string]any)
		require.True(t, ok, "the cap's one home is $defs/widgetListingLimit")
		assert.Equal(t, "integer", def["type"])
		assert.Equal(t, float64(0), def["minimum"])
		assert.Equal(t, float64(maxIndexWidgetLimit), def["maximum"],
			"the schema's cap and widgetObjectWidgets' bound are one number")
	})

	t.Run("the index references the shared def like its siblings", func(t *testing.T) {
		limit, ok := schemaAt(t, index, "$defs", "widget", "properties", "limit").(map[string]any)
		require.True(t, ok)
		assert.Equal(t, SchemaURL+"#/$defs/widgetListingLimit", limit["$ref"],
			"limit states the cap as one $ref into the object schema — a copy is the drift channel")
		assert.NotContains(t, limit, "maximum", "no inline copy beside the $ref")
	})

	t.Run("the authoring index inlines the same number, pinned here", func(t *testing.T) {
		// the authoring subset is self-contained by design (it inlines the
		// layout enum too), so its copy is allowed — and chained to the same
		// constant so it cannot drift alone
		limit, ok := schemaAt(t, authoringIndex, "$defs", "widget", "properties", "limit").(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(0), limit["minimum"])
		assert.Equal(t, float64(maxIndexWidgetLimit), limit["maximum"])
	})

	t.Run("the widget BLOCK keeps the whole int32 range — the fidelity fallback", func(t *testing.T) {
		limit, ok := widgetBlockBranch(t, object)["limit"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(0), limit["minimum"])
		assert.Equal(t, float64(2147483647), limit["maximum"],
			"a widget the index refuses (limit above the cap) travels as a full document, "+
				"and that document must validate — capping the block breaks the designed fallback")
	})

	t.Run("the two doors behave as the two statements say", func(t *testing.T) {
		// the index: 100 in, 101 out
		_, err := UnmarshalIndex([]byte(`{"version":2,"widgets":[{"target":"page-a","limit":100}]}`))
		require.NoError(t, err)
		_, err = UnmarshalIndex([]byte(`{"version":2,"widgets":[{"target":"page-a","limit":101}]}`))
		require.Error(t, err)
		assert.Contains(t, issuePaths(t, err), "/widgets/0/limit")

		// the block: 101 is a legal document — the fallback the lift's own
		// refusal (pinned in widgetobject_test.go) depends on
		assert.NoError(t, Validate([]byte(
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"widget","limit":101}]}`)))
	})
}
