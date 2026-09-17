package v2service

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

func TestAPIV2SchemaIsTheFormatSchemaMinusTheExcludedMembers(t *testing.T) {
	t.Run("no excluded member is declared anywhere in the served schema", func(t *testing.T) {
		// given
		var served map[string]any
		require.NoError(t, json.Unmarshal(apiV2DocumentSchema(), &served))
		props, ok := served["properties"].(map[string]any)
		require.True(t, ok)

		// then
		for _, excluded := range apiV2ExcludedMembers {
			assert.NotContainsf(t, props, excluded.member,
				"%s is served to a caller who cannot supply it: %s", excluded.member, excluded.why)
		}
	})

	t.Run("every other root member the format declares survives", func(t *testing.T) {
		// given
		var full, served map[string]any
		require.NoError(t, json.Unmarshal(anyblockjson.SchemaJSON(), &full))
		require.NoError(t, json.Unmarshal(apiV2DocumentSchema(), &served))
		fullProps := full["properties"].(map[string]any)
		servedProps := served["properties"].(map[string]any)

		// when
		want := len(fullProps) - len(apiV2ExcludedMembers)

		// then
		assert.Len(t, servedProps, want, "the delta is the excluded list and nothing else")
		for name := range fullProps {
			if apiV2ExcludedSet[name] {
				continue
			}
			assert.Containsf(t, servedProps, name, "%s was dropped and is not on the excluded list", name)
		}
	})

	t.Run("no gate is left requiring a member the schema no longer declares", func(t *testing.T) {
		// a conditional that still REQUIRES an excluded member would make its
		// kind unsatisfiable against a closed schema.
		// given
		var served map[string]any
		require.NoError(t, json.Unmarshal(apiV2DocumentSchema(), &served))

		// then
		for _, raw := range served["allOf"].([]any) {
			branch, ok := raw.(map[string]any)
			require.True(t, ok)
			for _, arm := range []string{"then", "else"} {
				body, ok := branch[arm].(map[string]any)
				if !ok {
					continue
				}
				if required, ok := body["required"].([]any); ok {
					for _, name := range required {
						assert.False(t, apiV2ExcludedSet[name.(string)],
							"a gate still requires the excluded member %q", name)
					}
				}
				if props, ok := body["properties"].(map[string]any); ok {
					for name := range props {
						assert.Falsef(t, apiV2ExcludedSet[name],
							"a gate still mentions the excluded member %q", name)
					}
				}
			}
		}
	})

	t.Run("the served examples still validate against the served schema", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)

		// then
		for _, kind := range []string{"object", "type", "template"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err)
			assert.NoError(t, validateAgainstSchema(t, entry.Schema, entry.Example), kind)
		}
	})

	t.Run("a document valid under the served schema is valid AnyBlock JSON", func(t *testing.T) {
		// the invariant that makes this a SUBSET rather than a dialect.
		// given
		docs := []string{
			`{"formatVersion":"2.0","type":"page","properties":{"name":"A note"}}`,
			`{"formatVersion":"2.0","type":"page","blocks":[{"type":"paragraph","text":"hi"}]}`,
			`{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Task"},"type_settings":{"api_key":"task"}}`,
		}

		// then
		for _, doc := range docs {
			assert.NoErrorf(t, anyblockjson.Validate([]byte(doc)), "doc: %s", doc)
		}
	})
}

func TestTrimAPIDocumentEnvelopeMatchesTheServedSchema(t *testing.T) {
	// given
	fields := map[string]json.RawMessage{
		"formatVersion":          json.RawMessage(`"2.0"`),
		"type":                   json.RawMessage(`"page"`),
		"properties":             json.RawMessage(`{"name":"A"}`),
		"property_internal_keys": json.RawMessage(`{"Foo":"description"}`),
		"option_ids":             json.RawMessage(`{"Tag":{"Red":"o1"}}`),
		"uninstalled":            json.RawMessage(`true`),
	}

	// when
	trimAPIDocumentEnvelope(fields)

	// then
	assert.Equal(t, []string{"formatVersion", "properties", "type"}, sortedEnvelopeKeys(fields))
}

func sortedEnvelopeKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
