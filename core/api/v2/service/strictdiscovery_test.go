package v2service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictDiscoveryKeepsAnAuthorWrittenValueSchema(t *testing.T) {
	// The hardening pass bounds what is unbounded. It must not WIDEN what an
	// author already narrowed: discovery would then advertise values the
	// reader refuses, which is how the format's bounded-string legend came to
	// be published as any-JSON.
	t.Run("a narrow additionalProperties survives; only the count is added", func(t *testing.T) {
		// given
		in := []byte(`{"type":"object","additionalProperties":{"type":"string","maxLength":128}}`)

		// when
		out, err := strictDiscoverySchema(in)
		require.NoError(t, err)

		// then
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got))
		additional, ok := got["additionalProperties"].(map[string]any)
		require.True(t, ok, "the author's value schema was replaced: %s", out)
		assert.Equal(t, "string", additional["type"], "the declared type must survive")
		assert.EqualValues(t, 128, additional["maxLength"])
		assert.EqualValues(t, strictSchemaDefaultMaxProperties, got["maxProperties"],
			"the unbounded half — the count — is what gets a bound")
	})

	t.Run("an author's tighter count bound is not loosened", func(t *testing.T) {
		// given
		in := []byte(`{"type":"object","additionalProperties":{"type":"string"},"maxProperties":4}`)

		// when
		out, err := strictDiscoverySchema(in)
		require.NoError(t, err)

		// then
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got))
		assert.EqualValues(t, 4, got["maxProperties"])
	})

	t.Run("an open map with no value schema still gets the any-value", func(t *testing.T) {
		// given
		in := []byte(`{"type":"object"}`)

		// when
		out, err := strictDiscoverySchema(in)
		require.NoError(t, err)

		// then
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got))
		additional, ok := got["additionalProperties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, anyValueRef, additional["$ref"], "and it is hoisted, not inlined")
		assert.EqualValues(t, strictSchemaDefaultMaxProperties, got["maxProperties"])
	})
}

func TestStrictDiscoveryHoistsTheAnyValueBlob(t *testing.T) {
	fx := newV2FixtureBare(t)

	// the document kinds: their schemas are the format's, which carries the
	// open value slots this pass bounds. `type` serves the flat body instead,
	// and that one is closed all the way down — it needs no hoisted value at
	// all, which the subtest below pins.
	t.Run("the served schemas carry one definition and reference it", func(t *testing.T) {
		for _, kind := range []string{"object", "type_document", "template"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err)

			var doc map[string]any
			require.NoError(t, json.Unmarshal(entry.Schema, &doc))
			defs, ok := doc["$defs"].(map[string]any)
			require.Truef(t, ok, "%s has no $defs", kind)
			require.Containsf(t, defs, anyValueDef, "%s does not define the hoisted value", kind)

			body := string(entry.Schema)
			assert.Greaterf(t, strings.Count(body, anyValueRef), 0, "%s references nothing", kind)
		}
	})

	t.Run("the flat type schema needs no hoisted value", func(t *testing.T) {
		// given: every slot of the flat body is declared, so nothing is open
		entry, err := fx.SchemaKind("type")
		require.NoError(t, err)

		// then
		assert.NotContains(t, string(entry.Schema), anyValueRef,
			"the flat body should be closed all the way down")
	})

	t.Run("the definition does not reference itself", func(t *testing.T) {
		// a self-referential anyValue would be cyclic, which is exactly what
		// a constrained decoder cannot consume and what this pass exists to
		// prevent.
		entry, err := fx.SchemaKind("object")
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(entry.Schema, &doc))
		def := doc["$defs"].(map[string]any)[anyValueDef]
		encoded, err := json.Marshal(def)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), anyValueRef,
			"$defs/%s references itself — the schema is now cyclic", anyValueDef)
	})

	t.Run("no inlined copy of the blob is left anywhere", func(t *testing.T) {
		entry, err := fx.SchemaKind("object")
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(entry.Schema, &doc))
		delete(doc, "$defs")
		blob, err := json.Marshal(boundedDiscoveryValue(3))
		require.NoError(t, err)
		rest, err := json.Marshal(doc)
		require.NoError(t, err)
		assert.NotContains(t, string(rest), string(blob), "an inlined copy survived the hoist")
	})
}
