package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// addChoreObjects registers two chore objects (and one plain page) in the
// test space: chore1 has severity High and an older modification date,
// chore2 has none and a newer one.
func (fx *v2Fixture) addChoreObjects(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("chore1"),
			bundle.RelationKeyName:             domain.String("Fix the sink"),
			bundle.RelationKeyType:             domain.String("type-chore"),
			domain.RelationKey("severity"):     domain.StringList([]string{"opt-high"}),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
			// created by the caller — the §6.2 current-user placeholder target
			bundle.RelationKeyCreator: domain.String(domain.NewParticipantId(testSpaceId, testAccountId)),
		},
		{
			bundle.RelationKeyId:               domain.String("chore2"),
			bundle.RelationKeyName:             domain.String("Write the report"),
			bundle.RelationKeyType:             domain.String("type-chore"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(2000),
			bundle.RelationKeyCreator:          domain.String("_participant_space1_someoneElse"),
		},
		{
			bundle.RelationKeyId:               domain.String("page1"),
			bundle.RelationKeyName:             domain.String("A page"),
			bundle.RelationKeyType:             domain.String("type-page"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(3000),
		},
		{
			bundle.RelationKeyId:             domain.String("type-page"),
			bundle.RelationKeyName:           domain.String("Page"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
	})
}

func searchSetup(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addSelectProperty(t) // "severity" + option "High"
	fx.addTaskType(t)       // type "chore" recommending "severity"
	fx.addChoreObjects(t)
	return fx
}

func rowIds(rows []apimodel.V2ObjectRow) []string {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.Id
	}
	return ids
}

func TestV2SearchObjects(t *testing.T) {
	t.Run("structured filters narrow by option name (read-only resolution)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"severity","condition":"in","value":["High"]}]`),
		}

		// when
		rows, total, hasMore, warnings, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
		assert.Empty(t, warnings)
		require.Len(t, rows, 1)
		assert.Equal(t, "chore1", rows[0].Id)
		assert.Equal(t, "Fix the sink", rows[0].Name)
		assert.Equal(t, "chore", rows[0].Type, "rows carry the type KEY (C2), never the type object")
		assert.Empty(t, rows[0].SpaceId, "space-scoped rows carry no spaceId")
	})

	t.Run("the compact filter string lands on the same tree (one execution path)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Type: "chore", Filter: `severity IN ("High")`}

		// when
		rows, total, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, "chore1", rows[0].Id)
	})

	t.Run("filter and filters together are ambiguous_input (C6)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{
			Filter:  `severity IS EMPTY`,
			Filters: json.RawMessage(`[]`),
		}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "provide filter or filters, not both")
	})

	t.Run("unknown structured filter key gets a path-addressed did-you-mean (rule 1)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"sevirity","condition":"notEmpty"}]`),
		}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filters/0/property", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "sevirity"`)
		assert.Equal(t, "did you mean severity?", apiErr.Issues[0].Hint)
	})

	t.Run("unknown filter-string key gets an offset-addressed did-you-mean", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Type: "chore", Filter: `done = false AND sevirity IS EMPTY`}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then — "done" is not a chore key either, so it errors FIRST, at 0
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `parse error at offset 0 near "done"`)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "done"`)
	})

	t.Run("unknown option name never silently no-matches (rule 3)", func(t *testing.T) {
		// given
		fx := searchSetup(t)

		t.Run("structured form", func(t *testing.T) {
			req := apimodel.V2SearchRequest{
				Type:    "chore",
				Filters: json.RawMessage(`[{"property":"severity","condition":"in","value":["Hgih"]}]`),
			}

			_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

			apiErr := v2Err(t, err)
			require.Len(t, apiErr.Issues, 1)
			assert.Equal(t, "/filters/0/value", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Message, `property "severity" has no option named "Hgih"`)
			assert.Contains(t, apiErr.Issues[0].Message, "a query never creates options")
			assert.Equal(t, "did you mean High?", apiErr.Issues[0].Hint)
		})

		t.Run("string form", func(t *testing.T) {
			req := apimodel.V2SearchRequest{Type: "chore", Filter: `severity = "Hgih"`}

			_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

			apiErr := v2Err(t, err)
			require.Len(t, apiErr.Issues, 1)
			assert.Equal(t, "/filter", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Message, "parse error at offset 11")
			assert.Contains(t, apiErr.Issues[0].Message, `no option named "Hgih"`)
			assert.Equal(t, "did you mean High?", apiErr.Issues[0].Hint)
		})
	})

	t.Run("sort by any property key (v1's closed enum is gone)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{
			Type:  "chore",
			Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc"}]`),
		}

		// when
		rows, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"chore1", "chore2"}, rowIds(rows))
	})

	t.Run("default sort is newest-modified first", func(t *testing.T) {
		// given
		fx := searchSetup(t)

		// when
		rows, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Type: "chore"}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"chore2", "chore1"}, rowIds(rows))
	})

	t.Run("unknown sort key gets a path-addressed did-you-mean", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{
			Type:  "chore",
			Sorts: json.RawMessage(`[{"property":"dueDates"}]`),
		}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/sorts/0/property", apiErr.Issues[0].Path)
	})

	t.Run("type is a filterable pseudo-key (rule 6)", func(t *testing.T) {
		// given: no top-level type — the filter channel carries it
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Filter: `type IN ("chore")`}

		// when
		rows, total, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then: both chores, not the page
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.ElementsMatch(t, []string{"chore1", "chore2"}, rowIds(rows))
	})

	t.Run("unknown type key in the type pseudo-filter gets did-you-mean", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Filter: `type IN ("chores")`}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown type key "chores"`)
		assert.Contains(t, apiErr.Issues[0].Hint, "chore")
	})

	t.Run("the unguarded-date-comparison hazard warns on the response (rule 5)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Type: "chore", Filter: `lastModifiedDate < today()`}

		// when
		_, _, _, warnings, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then — the SPEC §6.2 import warning text rides the C6 channel
		require.NoError(t, err)
		require.Len(t, warnings, 1)
		assert.Equal(t, "/filter", warnings[0].Path)
		assert.Contains(t, warnings[0].Message, "also matches objects with no lastModifiedDate")
		assert.Contains(t, warnings[0].Message, "notEmpty")
	})

	t.Run("unknown fields keys are rejected (rule 1 covers field keys)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Type: "chore", Fields: []string{"sevirity"}}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/fields/0", apiErr.Issues[0].Path)
		assert.Equal(t, "did you mean severity?", apiErr.Issues[0].Hint)
	})

	t.Run("fields expand rows with property values (C5)", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		req := apimodel.V2SearchRequest{Type: "chore", Fields: []string{"severity"}}

		// when
		rows, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then — option ids render as NAMES (C2)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		byId := map[string]apimodel.V2ObjectRow{}
		for _, row := range rows {
			byId[row.Id] = row
		}
		assert.Equal(t, []any{"High"}, byId["chore1"].Properties["severity"])
	})

	t.Run("pagination is honest (C10)", func(t *testing.T) {
		// given
		fx := searchSetup(t)

		// when
		rows, total, hasMore, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Type: "chore"}, 0, 1)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, total, "total is the store count, not len(fetched)")
		assert.True(t, hasMore)
		require.Len(t, rows, 1)
	})

	t.Run("unknown top-level type gets the R9 did-you-mean", func(t *testing.T) {
		// given
		fx := searchSetup(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Type: "chores"}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "chore")
	})
}

func TestV2GlobalSearchObjects(t *testing.T) {
	const otherSpaceId = "space2"

	setup := func(t *testing.T) *v2Fixture {
		fx := searchSetup(t)
		fx.registerSpace(t, otherSpaceId)
		fx.objectStore.AddObjects(t, otherSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String("note1"),
				bundle.RelationKeyName:             domain.String("A note elsewhere"),
				bundle.RelationKeyType:             domain.String("type-note"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(1500),
			},
			{
				bundle.RelationKeyId:             domain.String("type-note"),
				bundle.RelationKeyName:           domain.String("Note"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-note"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})
		return fx
	}

	t.Run("merges spaces by the requested sort with honest totals (rule 4)", func(t *testing.T) {
		// given
		fx := setup(t)

		// when: no type, no filters — default newest-modified-first merge
		rows, total, hasMore, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{}, 0, 25)

		// then: page1(3000) > chore2(2000) > note1(1500) > chore1(1000)
		require.NoError(t, err)
		assert.Equal(t, 4, total, "total is the sum of per-space store counts")
		assert.False(t, hasMore)
		assert.Equal(t, []string{"page1", "chore2", "note1", "chore1"}, rowIds(rows))
		for _, row := range rows {
			assert.NotEmpty(t, row.SpaceId, "global rows carry their space id")
		}
	})

	t.Run("a type resolving in only some spaces queries those and warns (rule 4)", func(t *testing.T) {
		// given: type "chore" exists only in space1
		fx := setup(t)

		// when
		rows, total, _, warnings, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{Type: "chore"}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.ElementsMatch(t, []string{"chore1", "chore2"}, rowIds(rows))
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, `space "space2" was skipped`)
		assert.Contains(t, warnings[0].Message, `unknown type key "chore"`)
	})

	t.Run("a reference resolving nowhere is the error, not an empty result", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, _, _, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{Type: "ghost"}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
	})

	t.Run("has_more compares the requested page against the honest total", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		rows, total, hasMore, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{}, 0, 2)

		// then
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.True(t, hasMore)
		assert.Equal(t, []string{"page1", "chore2"}, rowIds(rows))
	})

	t.Run("offset pages continue the merged order", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		rows, _, hasMore, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{}, 2, 2)

		// then
		require.NoError(t, err)
		assert.False(t, hasMore)
		assert.Equal(t, []string{"note1", "chore1"}, rowIds(rows))
	})
}
