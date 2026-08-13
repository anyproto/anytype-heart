package v2model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorShape(t *testing.T) {
	t.Run("serializes the C6 envelope", func(t *testing.T) {
		// given
		err := AmbiguousInput("outline and block are mutually exclusive",
			Issue{Path: "outline", Message: "conflicts with block", Hint: "drop one"})
		want := `{"status":400,"code":"ambiguous_input","message":"outline and block are mutually exclusive","issues":[{"path":"outline","message":"conflicts with block","hint":"drop one"}]}`

		// when
		data, marshalErr := json.Marshal(err)

		// then
		require.NoError(t, marshalErr)
		assert.JSONEq(t, want, string(data))
	})

	t.Run("issues are omitted when empty", func(t *testing.T) {
		data, err := json.Marshal(NotFound("object gone"))
		require.NoError(t, err)
		assert.NotContains(t, string(data), "issues")
	})

	t.Run("etag mismatch carries the current etag", func(t *testing.T) {
		err := EtagMismatch("abcd1234")
		assert.Equal(t, 409, err.Status)
		assert.Equal(t, CodeEtagMismatch, err.Code)
		assert.Contains(t, err.Message, `"abcd1234"`)
	})

	t.Run("version unsupported names both versions verbatim", func(t *testing.T) {
		err := VersionUnsupported(2, 1)
		assert.Equal(t, 400, err.Status)
		assert.Equal(t, CodeVersionUnsupported, err.Code)
		assert.Contains(t, err.Message, "produced by a newer version")
		assert.Contains(t, err.Message, "document version 2")
		assert.Contains(t, err.Message, "supported version 1")
	})
}

func TestNewListResponse(t *testing.T) {
	t.Run("truncated lists carry a steering message", func(t *testing.T) {
		// given / when
		resp := NewListResponse([]TypeRow{{Key: "task"}}, 312, 0, 1, true, "narrow with prefix=")

		// then
		assert.True(t, resp.HasMore)
		assert.Contains(t, resp.Message, "312 matches")
		assert.Contains(t, resp.Message, "narrow with prefix=")
	})

	t.Run("complete lists have no message and data serializes as []", func(t *testing.T) {
		// given / when
		resp := NewListResponse[TypeRow](nil, 0, 0, 25, false, "unused")

		// then
		assert.Empty(t, resp.Message)
		data, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"data":[]`)
	})
}

// jsonFieldNames extracts the json tag names of a struct type, in field
// order — the wire vocabulary a doc twin must reproduce exactly.
func jsonFieldNames(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	var names []string
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		require.NotEmpty(t, tag, "field %s must carry a json tag", typ.Field(i).Name)
		names = append(names, strings.Split(tag, ",")[0])
	}
	return names
}

func TestSearchRequestDocMirrorsSearchRequest(t *testing.T) {
	// SearchRequestDoc exists ONLY for the OpenAPI document (swag cannot
	// resolve json.RawMessage and panics on swaggertype:"array,…"), so the
	// one way it can go wrong is drifting from the type the handler actually
	// decodes. Field-for-field, same json names, same order.
	want := jsonFieldNames(t, reflect.TypeOf(SearchRequest{}))

	got := jsonFieldNames(t, reflect.TypeOf(SearchRequestDoc{}))

	assert.Equal(t, want, got, "the doc twin drifted from SearchRequest — the published document would lie about the search body")
}

// TestIsOutputOnlyProperty pins the §4a predicate in BOTH vocabularies. The
// stored list is camelCase and every caller now speaks slugs (ADDRESSING
// §7.5a) — the wrapper's describe passes SERVED keys straight in — so the
// bundled fallback is what keeps `created_date` output-only. Revert it and
// the two surfaces that share this predicate start disagreeing, silently:
// a setProperties naming created_date would be accepted, and describe would
// advertise it as settable.
func TestIsOutputOnlyProperty(t *testing.T) {
	t.Run("both spellings of an output-only key answer the same", func(t *testing.T) {
		for stored, slug := range map[string]string{
			"createdDate":      "created_date",
			"lastModifiedDate": "last_modified_date",
			"coverId":          "cover_id",
			"coverType":        "cover_type",
			"isArchived":       "is_archived",
			"resolvedLayout":   "resolved_layout",
		} {
			assert.True(t, IsOutputOnlyProperty(stored), stored)
			assert.True(t, IsOutputOnlyProperty(slug), slug)
		}
		// creator spells the same in both vocabularies — which is why a
		// fixture using it could not tell them apart
		assert.True(t, IsOutputOnlyProperty("creator"))
	})

	t.Run("authorable keys stay authorable in both spellings", func(t *testing.T) {
		for _, key := range []string{"name", "description", "dueDate", "due_date", "isFavorite", "is_favorite", "manual_property"} {
			assert.False(t, IsOutputOnlyProperty(key), key)
		}
	})

	t.Run("the advertised listing is the wire spelling", func(t *testing.T) {
		keys := OutputOnlyPropertyKeys()
		assert.Contains(t, keys, "created_date")
		assert.NotContains(t, keys, "createdDate")
		for _, key := range keys {
			assert.True(t, IsOutputOnlyProperty(key), "everything advertised must answer the predicate: %s", key)
		}
	})
}
