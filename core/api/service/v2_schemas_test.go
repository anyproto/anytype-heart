package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

func TestV2Schemas(t *testing.T) {
	fx := newV2FixtureBare(t)

	t.Run("index lists every kind with its endpoint and url", func(t *testing.T) {
		// when
		index := fx.SchemaIndex()

		// then
		require.NotEmpty(t, index.Kinds)
		kinds := map[string]bool{}
		for _, entry := range index.Kinds {
			kinds[entry.Kind] = true
			assert.NotEmpty(t, entry.Endpoint, entry.Kind)
			assert.Equal(t, "/v2/schemas/"+entry.Kind, entry.Url)
		}
		for _, want := range []string{"object", "shortcut", "type", "template", "property", "set", "collection", "file", "filters"} {
			assert.True(t, kinds[want], "missing kind %s", want)
		}
	})

	t.Run("every kind serves parseable schema and example (C12/C13)", func(t *testing.T) {
		for _, entry := range fx.SchemaIndex().Kinds {
			got, err := fx.SchemaKind(entry.Kind)
			require.NoError(t, err, entry.Kind)
			var schema, example any
			require.NoError(t, json.Unmarshal(got.Schema, &schema), "schema of %s must be valid JSON", entry.Kind)
			require.NoError(t, json.Unmarshal(got.Example, &example), "example of %s must be valid JSON", entry.Kind)
		}
	})

	t.Run("AnyBlock examples pass the format's own validation", func(t *testing.T) {
		// the object, type and template examples are full AnyBlock documents;
		// serving an example the format rejects would poison every agent
		for _, kind := range []string{"object", "type", "template"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err)
			assert.NoError(t, anyblockjson.Validate(entry.Example), "example of kind %s", kind)
		}
	})

	t.Run("object kind serves the embedded format schema", func(t *testing.T) {
		entry, err := fx.SchemaKind("object")
		require.NoError(t, err)
		assert.JSONEq(t, string(anyblockjson.SchemaJSON()), string(entry.Schema))
	})

	t.Run("unknown kind is a 404 naming the available kinds", func(t *testing.T) {
		_, err := fx.SchemaKind("wat")
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "object")
	})
}
