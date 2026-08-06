package v2service

import (
	"encoding/json"
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

func TestDiffEditDocs(t *testing.T) {
	t.Run("insertion does not mark following siblings moved", func(t *testing.T) {
		// given
		before := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"b","type":"paragraph","text":"b"}]}`)
		after := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"n","type":"paragraph","text":"n"},{"id":"b","type":"paragraph","text":"b"}]}`)
		want := v2model.V2DiffStats{BlocksAdded: 1}

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
		assert.Equal(t, v2model.V2DiffStats{BlocksMoved: 2}, got)
	})

	t.Run("reparenting is moved, not changed", func(t *testing.T) {
		before := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"id":"b","type":"paragraph","text":"b"}]}`)
		after := []byte(`{"blocks":[{"id":"a","type":"paragraph","text":"a"},{"indent":1,"id":"b","type":"paragraph","text":"b"}]}`)

		got, err := diffEditDocs(before, after)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksMoved: 1}, got)
	})

	t.Run("property add, change and removal each count once", func(t *testing.T) {
		before := []byte(`{"properties":{"name":"Doc","done":false}}`)
		after := []byte(`{"properties":{"name":"Doc2","status":["x"]}}`)

		got, err := diffEditDocs(before, after)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{PropertiesChanged: 3}, got)
	})
}
