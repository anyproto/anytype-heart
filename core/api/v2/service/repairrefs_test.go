package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// Round-two eval F6/F9/F10/F15/F17: the errors callers actually hit carry
// the reference their repair needs, spelled in the hint exactly as the
// reference renders, so every surface can find and replace it.

func issueAt(t *testing.T, err error, path string) v2model.Issue {
	t.Helper()
	apiErr := v2Err(t, err)
	for _, issue := range apiErr.Issues {
		if issue.Path == path {
			return issue
		}
	}
	t.Fatalf("no issue at %s in %v", path, apiErr.Issues)
	return v2model.Issue{}
}

func TestV2RepairReferences(t *testing.T) {
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)
		return fx
	}

	t.Run("a missing formatVersion names the one legal value and the schema", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"chore","properties":{"name":"X"},"blocks":[{"type":"paragraph","text":"hi"}]}`), true, true)

		issue := issueAt(t, err, "/formatVersion")
		assert.Equal(t, "formatVersion is required", issue.Message)
		assert.Contains(t, issue.Hint, `include "formatVersion":"2.0"`)
		assert.Contains(t, issue.Hint, v2model.RefGetSchema("object").String())
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("object")}, issue.SeeAlso)
	})

	t.Run("a formatVersion of the wrong shape names the literal too", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":2,"type":"chore","properties":{"name":"X"}}`), true, true)

		issue := issueAt(t, err, "/formatVersion")
		assert.Contains(t, issue.Hint, `include "formatVersion":"2.0"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("object")}, issue.SeeAlso)
	})

	t.Run("a newer formatVersion keeps its own verdict", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"9.0","type":"chore","properties":{"name":"X"}}`), true, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeVersionUnsupported, apiErr.Code)
	})

	t.Run("a definition's key and type members are both named, at the flat body's own paths", func(t *testing.T) {
		// the format refused `key` and pruned `type`; the caller dropped the
		// member and every field became text
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Plant","property_definitions":[{"key":"loc","type":"select"}]}`), true, true)

		key := issueAt(t, err, "/property_definitions/0/key")
		assert.Contains(t, key.Message, `spelled "property"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type")}, key.SeeAlso)
		format := issueAt(t, err, "/property_definitions/0/type")
		assert.Contains(t, format.Message, `spelled "format"`)
		assert.Contains(t, format.Message, "select")
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type")}, format.SeeAlso)
		assert.Len(t, v2Err(t, err).Issues, 2, "no 'missing property' trio beside the two repairs")
	})

	t.Run("the same on the document form, under type_settings", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Plant"},"type_settings":{"property_definitions":[{"name":"Loc","type":"select"}]}}`), true, true)

		format := issueAt(t, err, "/type_settings/property_definitions/0/type")
		assert.Contains(t, format.Message, `spelled "format"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type_document")}, format.SeeAlso,
			"the document form is documented by type_document; the flat kind type has neither type_settings nor formatVersion")
		assert.Len(t, v2Err(t, err).Issues, 1)
	})

	t.Run("an unsupported formatVersion outranks the definition repairs", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"9.0","kind":"object_type","properties":{"name":"Plant"},"type_settings":{"property_definitions":[{"name":"Loc","type":"select"}]}}`), true, true)

		assert.Equal(t, v2model.CodeVersionUnsupported, v2Err(t, err).Code)
	})

	t.Run("a type document's formatVersion repair points at type_document", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":2,"kind":"object_type","properties":{"name":"Plant"}}`), true, true)

		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type_document")}, issueAt(t, err, "/formatVersion").SeeAlso)
	})

	t.Run("PATCH types names a definition's type member too, instead of minting text", func(t *testing.T) {
		fx := newTypeOpsFixture(t)

		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			[]byte(`{"property_definitions":[{"name":"New location","type":"select"}]}`), true, false)

		format := issueAt(t, err, "/property_definitions/0/type")
		assert.Contains(t, format.Message, `spelled "format"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type")}, format.SeeAlso)
	})

	t.Run("a definition naming nothing gets one verdict, not three", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Plant","property_definitions":[{"format":"select"}]}`), true, true)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1, "%v", apiErr.Issues)
		assert.Equal(t, "/property_definitions/0", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `give "name"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("type")}, apiErr.Issues[0].SeeAlso)
	})

	t.Run("a flat type body's other refusals are addressed at the body, not the document built from it", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Plant","layout":"spiral"}`), true, true)

		apiErr := v2Err(t, err)
		for _, issue := range apiErr.Issues {
			assert.NotContains(t, issue.Path, "/type_settings", "%v", apiErr.Issues)
		}
		issueAt(t, err, "/layout")

		// the checks this function runs itself, after the format: same address
		_, err = fx.CreateType(context.Background(), testSpaceId, []byte(`{"name":"Plant","api_key":"bad-key"}`), true, true)
		issueAt(t, err, "/api_key")
		_, err = fx.CreateType(context.Background(), testSpaceId, []byte(`{"name":"Chore"}`), true, true)
		issueAt(t, err, "/name")
	})

	t.Run("the validate endpoint attaches the same repairs, pointed at the document kind", func(t *testing.T) {
		// validate applies the format's full schema, so its repair names the
		// unnarrowed document kind for every document, not the create kinds
		// (a type document with blocks is valid here and refused on create)
		fx := setup(t)

		resp := fx.ValidateDocument([]byte(`{"blocks":[{"type":"paragraph","text":"hi"}]}`))

		require.Len(t, resp.Issues, 1)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("document")}, resp.Issues[0].SeeAlso)

		typed := fx.ValidateDocument([]byte(`{"kind":"object_type","properties":{"name":"Plant"}}`))
		require.NotEmpty(t, typed.Issues)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("document")}, typed.Issues[0].SeeAlso)
	})

	t.Run("an unknown property's guess carries the list reference the list-all branch carries", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","severty":"High"}}`), true, true)

		issue := issueAt(t, err, "/properties/severty")
		assert.Contains(t, issue.Hint, "did you mean severity? — if not, list all with "+v2model.RefListProperties(testSpaceId).String())
		assert.Equal(t, []v2model.Ref{v2model.RefListProperties(testSpaceId), v2model.RefCreateProperty(testSpaceId)}, issue.SeeAlso)
	})

	t.Run("an unknown PATCH body key points at the op that carries it", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", []byte(`{"properties":{"name":"X"}}`), "", true, true)

		issue := issueAt(t, err, "/properties")
		assert.Contains(t, issue.Message, "carries only ops")
		assert.NotContains(t, issue.Hint, "If-Match")
		assert.Contains(t, issue.Hint, "set_properties op")
		assert.Equal(t, []v2model.Ref{v2model.RefGetOpSchema("set_properties")}, issue.SeeAlso)
	})

	t.Run("the If-Match hint fires only for a precondition written into the body", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", []byte(`{"ops":[],"If-Match":"h"}`), "", true, true)
		assert.Equal(t, "the If-Match precondition is a header, not a body field", issueAt(t, err, "/If-Match").Hint)

		_, err = fx.PatchObject(context.Background(), testSpaceId, "obj1", []byte(`{"ops":[],"colour":"red"}`), "", true, true)
		stray := issueAt(t, err, "/colour")
		assert.NotContains(t, stray.Hint, "If-Match")
		assert.Equal(t, []v2model.Ref{v2model.NewRef(v2model.OpListSchemas)}, stray.SeeAlso, "the op index is the schema index, not an unbound single-op lookup")
	})

	t.Run("a type ops body with a stray member names the op index", func(t *testing.T) {
		fx := newTypeOpsFixture(t)

		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			[]byte(`{"ops":[{"op":"remove_property","property":"sun_needs"}],"name":"Plant"}`), true, false)

		issue := issueAt(t, err, "/name")
		assert.Equal(t, []v2model.Ref{v2model.NewRef(v2model.OpListSchemas)}, issue.SeeAlso)
	})

	t.Run("a filter grammar error points at the grammar", func(t *testing.T) {
		fx := setup(t)

		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Type: "chore", Filter: `severity = = "High"`}, 0, 25)

		issue := issueAt(t, err, "/filter")
		assert.Contains(t, issue.Message, "parse error at offset 11")
		assert.Contains(t, issue.Hint, "values are double-quoted strings")
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("filters")}, issue.SeeAlso)
	})

	t.Run("an unknown filter key's guess carries the property list", func(t *testing.T) {
		fx := setup(t)

		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Type: "chore", Filter: `severty = "High"`}, 0, 25)

		issue := issueAt(t, err, "/filter")
		assert.Contains(t, issue.Hint, "did you mean severity? — if not, list keys with "+v2model.RefListProperties(testSpaceId).String())
		assert.Equal(t, []v2model.Ref{v2model.RefListProperties(testSpaceId)}, issue.SeeAlso)
	})

	t.Run("a structured filter without a condition points at the node shape", func(t *testing.T) {
		fx := setup(t)

		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Type: "chore", Filters: json.RawMessage(`[{"property":"severity","cond":"equal"}]`)}, 0, 25)

		issue := issueAt(t, err, "/filters/0/condition")
		assert.Contains(t, issue.Hint, "not_equal", "the conditions are spelled as the schema spells them")
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("filters")}, issue.SeeAlso)
	})

	t.Run("a query view refusal is addressed at /views and points at the view shape", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(context.Background(), testSpaceId,
			v2model.CreateQueryRequest{Name: "Q", Type: "chore", Views: json.RawMessage(`[{"name":"V","groupBy":"severity"}]`)}, true, true)

		issue := issueAt(t, err, "/views/0/groupBy")
		assert.Contains(t, issue.Message, `"groupBy" is not allowed`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("query"), v2model.RefGetOpSchema("insert_view")}, issue.SeeAlso)
	})

	t.Run("a fault in the default view is addressed at the top-level channel that built it", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(context.Background(), testSpaceId,
			v2model.CreateQueryRequest{Name: "Q", Type: "chore", Sorts: json.RawMessage(`[{"property":"severity","direction":"sideways"}]`)}, true, true)

		issue := issueAt(t, err, "/sorts/0/direction")
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("query")}, issue.SeeAlso)
		for _, iss := range v2Err(t, err).Issues {
			assert.NotContains(t, iss.Path, "/views", "the caller sent no views")
		}
	})

	t.Run("an unknown view key's guess names the type as the caller spelled it", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(context.Background(), testSpaceId,
			v2model.CreateQueryRequest{Name: "Q", Type: "chore", Sorts: json.RawMessage(`[{"property":"severty"}]`)}, true, true)

		issue := issueAt(t, err, "/sorts/0/property")
		assert.Contains(t, issue.Hint, "did you mean severity? — if not, ")
		assert.Equal(t, []v2model.Ref{v2model.RefGetType(testSpaceId, "chore")}, issue.SeeAlso)
	})

	t.Run("the query kind's views are typed, with the fields insert_view takes", func(t *testing.T) {
		fx := setup(t)

		schema, err := fx.SchemaKind("query")

		require.NoError(t, err)
		raw, _ := json.Marshal(schema)
		assert.Contains(t, string(raw), `"group_by"`)
		assert.Contains(t, string(raw), `"columns"`)
		assert.NotContains(t, string(raw), "full view objects", "the views member is a typed list now, not prose over anyValue")
		var entry struct {
			Schema struct {
				Properties struct {
					Views struct {
						Items map[string]any `json:"items"`
					} `json:"views"`
				} `json:"properties"`
			} `json:"schema"`
		}
		require.NoError(t, json.Unmarshal(raw, &entry))
		items, _ := json.Marshal(entry.Schema.Properties.Views.Items)
		assert.NotContains(t, string(items), "null", "a created view takes no null: that is the merge channel's clear-to-default")
		assert.NotContains(t, string(items), "/v2/", "no raw route rides into the query schema")
		assert.NotContains(t, string(items), `"filter"`, "the compact string is the query's top-level filter, not a view member")
	})
}
