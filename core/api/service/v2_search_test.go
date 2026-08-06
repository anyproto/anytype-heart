package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
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

	t.Run("a non-zero offset returns the next row and honest has_more", func(t *testing.T) {
		// given
		fx := searchSetup(t)

		// when: page 2 of the two chores (default sort: chore2 then chore1)
		rows, total, hasMore, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Type: "chore"}, 1, 1)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
		assert.Equal(t, []string{"chore1"}, rowIds(rows))
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

// addDateProperty registers a custom date property "verifiedUntil" in the
// test space (resolvable through the store, not the bundle).
func (fx *v2Fixture) addDateProperty(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("rel-verifiedUntil"),
		bundle.RelationKeyRelationKey:    domain.String("verifiedUntil"),
		bundle.RelationKeyName:           domain.String("Verified until"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}})
}

// addCheckboxProperty registers a custom checkbox property "done".
func (fx *v2Fixture) addCheckboxProperty(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("rel-done"),
		bundle.RelationKeyRelationKey:    domain.String("done"),
		bundle.RelationKeyName:           domain.String("Done"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_checkbox)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}})
}

func TestV2SearchStructuredDateValues(t *testing.T) {
	t.Run("an RFC 3339 string on a date property is rejected, never a silent no-match", func(t *testing.T) {
		// given: the exact hazard — a string value survives to the store and
		// compares string-against-int64, matching nothing without a word
		fx := searchSetup(t)
		fx.addDateProperty(t)
		req := apimodel.V2SearchRequest{
			Filters: json.RawMessage(`[{"property":"verifiedUntil","condition":"less","value":"2026-08-01"}]`),
		}

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		// then: the rejection spells out the conversion the caller needs
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filters/0/value", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `property "verifiedUntil" is a date — the structured form takes unix seconds (1785542400), not "2026-08-01"`)
		assert.Contains(t, apiErr.Issues[0].Hint, "compact filter string")
	})

	t.Run("a non-date string on a date property is rejected too", func(t *testing.T) {
		fx := searchSetup(t)
		fx.addDateProperty(t)
		req := apimodel.V2SearchRequest{
			Filters: json.RawMessage(`[{"property":"verifiedUntil","condition":"less","value":"next tuesday"}]`),
		}

		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, `"next tuesday" is not a date`)
	})

	t.Run("a date string inside an in-list is rejected as well", func(t *testing.T) {
		fx := searchSetup(t)
		fx.addDateProperty(t)
		req := apimodel.V2SearchRequest{
			Filters: json.RawMessage(`[{"property":"verifiedUntil","condition":"in","value":["2026-08-01"]}]`),
		}

		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filters/0/value", apiErr.Issues[0].Path)
	})

	t.Run("unix seconds on a date property pass", func(t *testing.T) {
		fx := searchSetup(t)
		fx.addDateProperty(t)
		// greater, because an unset date compares BELOW any set value — with
		// `less` every object without the property would match (the separate
		// unguarded-date warning covers that hazard)
		req := apimodel.V2SearchRequest{
			Filters: json.RawMessage(`[{"property":"verifiedUntil","condition":"greater","value":1785542400}]`),
		}

		_, total, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, req, 0, 25)

		require.NoError(t, err)
		assert.Equal(t, 0, total, "no object carries the property — an honest zero, not an error")
	})
}

func TestV2SearchPlanConvergence(t *testing.T) {
	// the two request forms must compile to the IDENTICAL filter tree — the
	// row-count assertions elsewhere would stay green even if the trees
	// diverged on the value classes that differ (dates, presets, booleans)
	pairs := []struct {
		name       string
		filter     string
		structured string
	}{
		{
			name:       "date converts to the same unix number",
			filter:     `verifiedUntil > "2026-08-01"`,
			structured: `[{"property":"verifiedUntil","condition":"greater","value":1785542400}]`,
		},
		{
			name:       "date preset",
			filter:     `verifiedUntil < currentWeek()`,
			structured: `[{"property":"verifiedUntil","condition":"less","datePreset":"currentWeek"}]`,
		},
		{
			name:       "set literal on a select property",
			filter:     `severity = ("High")`,
			structured: `[{"property":"severity","condition":"exactIn","value":["High"]}]`,
		},
		{
			name:       "boolean",
			filter:     `done = false`,
			structured: `[{"property":"done","condition":"equal","value":false}]`,
		},
		{
			name:       "type pseudo-key resolves to the same type id",
			filter:     `type IN ("chore")`,
			structured: `[{"property":"type","condition":"in","value":["chore"]}]`,
		},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			// given
			fx := searchSetup(t)
			fx.addDateProperty(t)
			fx.addCheckboxProperty(t)

			// when
			planFromString, err := fx.buildSearchPlan(testSpaceId, apimodel.V2SearchRequest{Filter: pair.filter}, true)
			require.NoError(t, err)
			planFromStructured, err := fx.buildSearchPlan(testSpaceId, apimodel.V2SearchRequest{Filters: json.RawMessage(pair.structured)}, true)
			require.NoError(t, err)

			// then
			assert.Equal(t, planFromStructured.filters, planFromString.filters,
				"both request forms must land on one filter tree")
		})
	}
}

func TestV2SearchEffectiveSorts(t *testing.T) {
	t.Run("full-text without sorts is pure relevance", func(t *testing.T) {
		sorts := defaultSearchSorts("report", nil)
		require.Len(t, sorts, 1)
		assert.Equal(t, bundle.RelationKey_final_score, sorts[0].RelationKey)
	})

	t.Run("full-text with explicit sorts appends the score tiebreak LAST", func(t *testing.T) {
		// the append is the load-bearing trick: with a score sort present the
		// engine does not PREPEND its own, so the user's sort stays primary
		user := []database.SortRequest{{RelationKey: bundle.RelationKeyLastModifiedDate, Type: model.BlockContentDataviewSort_Asc}}
		sorts := defaultSearchSorts("report", user)
		require.Len(t, sorts, 2)
		assert.Equal(t, bundle.RelationKeyLastModifiedDate, sorts[0].RelationKey, "the explicit sort stays primary")
		assert.Equal(t, bundle.RelationKey_final_score, sorts[1].RelationKey)
	})

	t.Run("no query and no sorts falls back to newest-modified first", func(t *testing.T) {
		sorts := defaultSearchSorts("", nil)
		require.Len(t, sorts, 1)
		assert.Equal(t, bundle.RelationKeyLastModifiedDate, sorts[0].RelationKey)
		assert.True(t, sorts[0].IncludeTime)
	})

	t.Run("a date sort left includeTime-less defaults to second granularity", func(t *testing.T) {
		// without this the granularity depended on whether the full-text
		// tiebreak was appended (isSingleDateSort sees 2 sorts → day-truncated)
		fx := searchSetup(t)
		plan, err := fx.buildSearchPlan(testSpaceId, apimodel.V2SearchRequest{
			Query: "report",
			Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc"}]`),
		}, true)
		require.NoError(t, err)
		require.NotEmpty(t, plan.sorts)
		assert.True(t, plan.sorts[0].IncludeTime)
	})

	t.Run("an explicit includeTime false is honored", func(t *testing.T) {
		fx := searchSetup(t)
		plan, err := fx.buildSearchPlan(testSpaceId, apimodel.V2SearchRequest{
			Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc","includeTime":false}]`),
		}, true)
		require.NoError(t, err)
		require.NotEmpty(t, plan.sorts)
		assert.False(t, plan.sorts[0].IncludeTime)
	})
}

func TestV2SearchFullText(t *testing.T) {
	indexDoc := func(t *testing.T, fx *v2Fixture, id, title string) {
		t.Helper()
		require.NoError(t, fx.objectStore.FullText.Index(ftsearch.SearchDoc{
			Id:      id,
			SpaceId: testSpaceId,
			Title:   title,
		}))
	}

	t.Run("query narrows to the indexed matches with an honest total", func(t *testing.T) {
		// given: only chore2 is full-text indexed
		fx := searchSetup(t)
		indexDoc(t, fx, "chore2/r/name", "Write the report")

		// when
		rows, total, hasMore, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Query: "report"}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
		assert.Equal(t, []string{"chore2"}, rowIds(rows))
	})

	t.Run("an explicit sort stays primary under full-text", func(t *testing.T) {
		// given: chore2 matches much more strongly, so pure relevance would
		// put it first — the explicit ascending date sort must win
		fx := searchSetup(t)
		indexDoc(t, fx, "chore1/r/name", "Fix the sink report")
		indexDoc(t, fx, "chore2/r/name", "Report report report")

		// when
		rows, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{
				Query: "report",
				Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc"}]`),
			}, 0, 25)

		// then: chore1 (1000) before chore2 (2000)
		require.NoError(t, err)
		assert.Equal(t, []string{"chore1", "chore2"}, rowIds(rows))
	})

	t.Run("an offset past the matches is an empty page, has_more false", func(t *testing.T) {
		fx := searchSetup(t)
		indexDoc(t, fx, "chore2/r/name", "Write the report")

		rows, total, hasMore, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Query: "report"}, 5, 25)

		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})

	t.Run("deep offsets escalate the store's candidate budget past the 100-doc floor", func(t *testing.T) {
		// given: 120 matches — more than ftCandidatesMin (100). With Limit 0
		// the engine froze at the 100-doc floor: total capped at ~100 and the
		// page at offset 100 came back empty with has_more false, silently
		// ending the agent's enumeration at a seventh of the matches.
		fx := searchSetup(t)
		const matches = 120
		objects := make([]objectstore.TestObject, 0, matches)
		for i := 0; i < matches; i++ {
			objects = append(objects, objectstore.TestObject{
				bundle.RelationKeyId:               domain.String(fmt.Sprintf("ft%03d", i)),
				bundle.RelationKeyName:             domain.String(fmt.Sprintf("Quarterly report %d", i)),
				bundle.RelationKeyType:             domain.String("type-chore"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(int64(10000 + i)),
			})
		}
		fx.objectStore.AddObjects(t, testSpaceId, objects)
		for i := 0; i < matches; i++ {
			indexDoc(t, fx, fmt.Sprintf("ft%03d/r/name", i), fmt.Sprintf("Quarterly report %d", i))
		}

		// when: the page past the floor
		rows, total, hasMore, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Query: "report"}, 100, 10)

		// then: the store escalated its budget and served the page
		require.NoError(t, err)
		require.Len(t, rows, 10, "the page past the 100-doc floor must not be empty")
		assert.True(t, hasMore)
		assert.GreaterOrEqual(t, total, 111, "total is at least the clipped fetch")

		// and the enumeration terminates honestly at the true end
		rows, total, hasMore, _, err = fx.SearchObjects(context.Background(), testSpaceId,
			apimodel.V2SearchRequest{Query: "report"}, 110, 10)
		require.NoError(t, err)
		assert.Len(t, rows, 10)
		assert.Equal(t, matches, total, "an exhausted result reports the exact count")
		assert.False(t, hasMore)
	})
}

func TestV2Phase4EnsureSpaceGuard(t *testing.T) {
	// the C2 class: every space-scoped Phase-4 entry point must 404 an
	// unknown space id BEFORE any store or reader access (the reader mock
	// carries no expectation — a call would fail the test)
	fx := newV2Fixture(t)
	const ghost = "ghostspace"
	ctx := context.Background()
	calls := []struct {
		name string
		run  func() error
	}{
		{"SearchObjects", func() error {
			_, _, _, _, err := fx.SearchObjects(ctx, ghost, apimodel.V2SearchRequest{}, 0, 25)
			return err
		}},
		{"GetSetObjects", func() error {
			_, _, _, _, err := fx.GetSetObjects(ctx, ghost, "set1", "", nil, 0, 25)
			return err
		}},
		{"GetSetViews", func() error {
			_, _, _, err := fx.GetSetViews(ctx, ghost, "set1", 0, 25)
			return err
		}},
		{"GetCollectionObjects", func() error {
			_, _, _, _, err := fx.GetCollectionObjects(ctx, ghost, "col1", "", nil, 0, 25)
			return err
		}},
		{"GetCollectionViews", func() error {
			_, _, _, err := fx.GetCollectionViews(ctx, ghost, "col1", 0, 25)
			return err
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			apiErr := v2Err(t, call.run())
			assert.Equal(t, apimodel.V2CodeNotFound, apiErr.Code)
			assert.Contains(t, apiErr.Message, `space "ghostspace" not found`)
		})
	}
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

	t.Run("a display-only fields key unknown in one space never narrows the scope", func(t *testing.T) {
		// given: "severity" exists only in space1 — a column request must not
		// remove space2's rows from results or total
		fx := setup(t)

		// when
		rows, total, _, warnings, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{Fields: []string{"severity"}}, 0, 25)

		// then: all four rows, and a warning instead of a dropped space
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Equal(t, []string{"page1", "chore2", "note1", "chore1"}, rowIds(rows))
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, `field "severity" is not a property of space "space2"`)
		assert.Contains(t, warnings[0].Message, "omitted from those rows")
	})

	t.Run("the global offset is bounded — deep paging steers to the space search", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, _, _, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{}, 2001, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		assert.Contains(t, apiErr.Message, "global search pages at most 2000 rows deep")
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Hint, "POST /v2/spaces/{spaceId}/search")
	})

	t.Run("identical per-space warnings dedupe to one", func(t *testing.T) {
		// given: the unguarded-date warning fires identically in both spaces
		fx := setup(t)

		// when
		_, _, _, warnings, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{Filter: `lastModifiedDate < today()`}, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, warnings, 1, "the same (path, message) warning from N spaces collapses to one")
		assert.Contains(t, warnings[0].Message, "also matches objects with no lastModifiedDate")
	})

	t.Run("a space being removed is not searched (and gets no index minted)", func(t *testing.T) {
		// given: a spaceView in removing state whose object would sort first
		fx := setup(t)
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("spaceView_gone"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String("spaceGone"),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceRemoving)),
		}})
		fx.objectStore.AddObjects(t, "spaceGone", []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("gonner"),
			bundle.RelationKeyName:             domain.String("A note in a removed space"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(9999),
		}})

		// when
		rows, total, _, _, err := fx.GlobalSearchObjects(context.Background(),
			apimodel.V2SearchRequest{}, 0, 25)

		// then: the removed space contributes nothing
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.NotContains(t, rowIds(rows), "gonner")
	})
}
