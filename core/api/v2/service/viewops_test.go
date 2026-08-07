package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
)

// editSetDoc is a set with the one dataview block sets carry (fixed id
// "dataview"): one "All" view whose custom columns are hidden — the exact
// shape the gap report described on a freshly generated type/set view.
const editSetDoc = `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"key":"name","format":"text"},{"key":"severity","format":"select"},{"key":"dueDate","format":"date"}],` +
	`"views":[{"id":"viewAll1","name":"All",` +
	`"sorts":[{"property":"dueDate"}],` +
	`"columns":[{"property":"name"},{"property":"severity","hidden":true,"width":100},{"property":"dueDate","hidden":true,"width":120}]}]}]}`

// editTwoViewsDoc carries two views, so view targeting is required.
const editTwoViewsDoc = `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"key":"name","format":"text"},{"key":"severity","format":"select"}],` +
	`"views":[` +
	`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},` +
	`{"id":"viewBoard2","name":"Board","type":"kanban","groupBy":"severity","columns":[{"property":"name"},{"property":"severity","hidden":true}]}]}]}`

// editTwoDataviewsDoc is a page with two inline dataviews — block targeting
// is required.
const editTwoDataviewsDoc = `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[` +
	`{"id":"blockPara1","type":"paragraph","text":"intro"},` +
	`{"id":"dvFirst1","type":"dataview","properties":[{"key":"name","format":"text"}],"views":[{"id":"viewA1","name":"A","columns":[{"property":"name"}]}]},` +
	`{"id":"dvSecond2","type":"dataview","properties":[{"key":"name","format":"text"}],"views":[{"id":"viewB1","name":"B","columns":[{"property":"name","hidden":true}]}]}]}`

// editTypeDoc is a kind:"objectType" document with the type's own dataview —
// the reporter's actual target (the default "All" view of a custom type).
const editTypeDoc = `{"version":1,"kind":"objectType","id":"obj1","key":"plant","properties":{"name":"Plant"},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"key":"name","format":"text"},{"key":"severity","format":"select"}],` +
	`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"},{"property":"severity","hidden":true}]}]}]}`

// dataviewOf digs the addressed dataview block out of a captured state doc.
func dataviewOf(t *testing.T, st *state.State, blockId string) map[string]any {
	t.Helper()
	for _, b := range docBlocks(stateDoc(t, st)) {
		if b["id"] == blockId {
			return b
		}
	}
	t.Fatalf("dataview block %q not found in captured state", blockId)
	return nil
}

func viewsOf(t *testing.T, dv map[string]any) []map[string]any {
	t.Helper()
	raw, _ := dv["views"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(map[string]any))
	}
	return out
}

func columnsOf(t *testing.T, view map[string]any) []map[string]any {
	t.Helper()
	raw, _ := view["columns"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		out = append(out, c.(map[string]any))
	}
	return out
}

func columnByProperty(t *testing.T, view map[string]any, key string) map[string]any {
	t.Helper()
	for _, col := range columnsOf(t, view) {
		if col["property"] == key {
			return col
		}
	}
	return nil
}

func TestUpdateViewOp(t *testing.T) {
	ctx := context.Background()

	t.Run("flipping one column's hidden touches nothing else", func(t *testing.T) {
		// given: the reported gap — a view whose custom columns are all hidden
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		// when: no block, no view — both default (one dataview, one view)
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		severity := columnByProperty(t, view, "severity")
		require.NotNil(t, severity)
		_, stillHidden := severity["hidden"]
		assert.False(t, stillHidden, "hidden flipped off (visible = omitted on export)")
		assert.Equal(t, float64(100), severity["width"], "the column's other fields survive the merge")
		dueDate := columnByProperty(t, view, "dueDate")
		require.NotNil(t, dueDate)
		assert.Equal(t, true, dueDate["hidden"], "unnamed columns are untouched")
		assert.Equal(t, float64(120), dueDate["width"])
		assert.Equal(t, "All", view["name"], "view fields are untouched")
		sorts, _ := view["sorts"].([]any)
		require.Len(t, sorts, 1, "sorts are untouched")
	})

	t.Run("updateView works on a type document (the reported target)", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTypeDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		severity := columnByProperty(t, view, "severity")
		require.NotNil(t, severity)
		_, stillHidden := severity["hidden"]
		assert.False(t, stillHidden)
	})

	t.Run("updateView needs neither restriction axis — the M1 lesson applied", func(t *testing.T) {
		// sets, collections AND object types all carry Restrictions_Blocks —
		// classifying the view edit as a block op would refuse it on exactly
		// the objects it exists to edit. This test fails if v2OpEditNeeds ever
		// reclassifies updateView.
		fx := newV2Fixture(t)
		read := editRead(t, editSetDoc)
		read.BlocksRefused = blocksRefusedProduction()
		read.DetailsRefused = blocksRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()

		var got apicore.EditNeeds
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				got = needs
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
					return nil, err
				}
				return []string{"headB"}, nil
			})

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", false)

		require.NoError(t, err, "a blocks-and-details-restricted set must still accept a view edit")
		assert.Equal(t, apicore.EditNeeds{}, got, "updateView must demand neither restriction axis")
	})

	t.Run("the dry run reaches the same verdict on a restricted object (C9)", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editSetDoc)
		read.BlocksRefused = blocksRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", true)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
	})

	t.Run("a new column appends and joins the dataview properties list", func(t *testing.T) {
		// given: "severity" exists in the space but "priority" is a fresh
		// space property not yet in this dataview
		fx := newV2Fixture(t)
		fx.addTagProperty(t) // registers "tags" (multiSelect) in the space
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"tags":{"width":90}}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		dv := dataviewOf(t, *captured, "dataview")
		view := viewsOf(t, dv)[0]
		tags := columnByProperty(t, view, "tags")
		require.NotNil(t, tags, "a key without a column appends one")
		assert.Equal(t, float64(90), tags["width"])
		props, _ := dv["properties"].([]any)
		var keys []string
		for _, p := range props {
			keys = append(keys, p.(map[string]any)["key"].(string))
		}
		assert.Contains(t, keys, "tags", "the properties list gains the key so its format rehydrates")
	})

	t.Run("null removes a column; removing an absent one is a no-op", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"dueDate":null,"neverWasAColumn":null}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		assert.Nil(t, columnByProperty(t, view, "dueDate"), "the named column is removed")
		require.Len(t, columnsOf(t, view), 2)
	})

	t.Run("an unknown column property gets the did-you-mean", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severty":{"hidden":false}}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "ops[0].columns.severty", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "severity", "the hint suggests the close key")
	})

	t.Run("an unknown column field lists the allowed ones", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"visible":true}}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "allowed: hidden, width, align, aggregation")
		assert.Equal(t, "ops[0].columns.severity.visible", apiErr.Issues[0].Path)
	})

	t.Run("set merges view fields and validates enums", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"name":"Open bugs","type":"kanban","groupBy":"severity"}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		assert.Equal(t, "Open bugs", view["name"])
		assert.Equal(t, "kanban", view["type"])
		assert.Equal(t, "severity", view["groupBy"])
		require.Len(t, columnsOf(t, view), 3, "columns are untouched by a set-only op")
	})

	t.Run("an unknown enum value lists the vocabulary", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"type":"board"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "table, list, gallery, kanban, calendar, graph")
	})

	t.Run("set.columns is steered to the columns channel", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"columns":[{"property":"name"}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "columns channel")
	})

	t.Run("output-only editor state is rejected by name", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"groups":[]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "output-only")
	})

	t.Run("an unknown view field lists the allowed set", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"nam":"Open"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown view field "nam"`)
		assert.Contains(t, apiErr.Issues[0].Hint, "name")
	})

	t.Run("null clears a view field back to its default", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","view":"viewBoard2","set":{"groupBy":null,"type":null}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[1]
		_, hasGroupBy := view["groupBy"]
		assert.False(t, hasGroupBy)
		_, hasType := view["type"]
		assert.False(t, hasType, "type cleared = the table default (omitted on export)")
	})

	t.Run("sorts replace whole and are vocabulary-checked", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"sorts":[{"property":"severity","direction":"desc"}]}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		sorts, _ := view["sorts"].([]any)
		require.Len(t, sorts, 1)
		sort := sorts[0].(map[string]any)
		assert.Equal(t, "severity", sort["property"])
		assert.Equal(t, "desc", sort["direction"])
	})

	t.Run("a bad sort direction is path-addressed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"sorts":[{"property":"severity","direction":"down"}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.sorts[0].direction", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "asc, desc, custom")
	})

	t.Run("structured filters replace whole; the M3 shape gate runs", func(t *testing.T) {
		// a node carrying both arms used to reach the store as MATCH
		// EVERYTHING — on a persisted view that is not a bad query but a view
		// that quietly shows the whole space, for good
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"operator":"and","property":"severity","condition":"equal","value":"High"}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Issues[0].Path, "ops[0].set.filters[0]")
	})

	t.Run("a bad filter condition gets the vocabulary, path-addressed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"severity","condition":"equals","value":"High"}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.filters[0].condition", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "equal")
	})

	t.Run("a filter naming an existing option stores its id (name is the identity)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t) // "severity" with existing option "High" (opt-high)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"severity","condition":"equal","value":"High"}]}}`), "", false)

		require.NoError(t, err)
		assert.Nil(t, result.Created, "an existing option name creates nothing")
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		// the captured doc is exported without a store resolver, so the
		// stored option ID shows verbatim — proving the name resolved
		assert.Equal(t, "opt-high", filters[0].(map[string]any)["value"])
	})

	t.Run("a filter naming a new option reports the would-be create on dry run", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editSetDoc), nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"severity","condition":"equal","value":"BrandNew"}]}}`), "", true)

		require.NoError(t, err)
		require.NotNil(t, result.Created, "the M5 machinery must see updateView's option channel")
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "BrandNew", result.Created.Options[0].Name)
	})

	t.Run("the M5 bound counts updateView's filter options", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editSetDoc), nil)
		// no mutator expectation and no create expectation: the refusal must
		// happen before anything is created or locked

		names := make([]string, 0, v2MaxCreatedOptionsPerPatch+1)
		for i := 0; i <= v2MaxCreatedOptionsPerPatch; i++ {
			names = append(names, `"NewTag`+string(rune('A'+i%26))+string(rune('0'+i/26))+`"`)
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"tags","condition":"in","value":[`+joinStrings(names, ",")+`]}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "too many new options")
	})

	t.Run("an empty-condition value is stripped, creating nothing (§11)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"severity","condition":"empty","value":"Ghost"}]}}`), "", false)

		require.NoError(t, err)
		assert.Nil(t, result.Created, "a value on an empty-condition leaf must not mint an option")
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		_, hasValue := filters[0].(map[string]any)["value"]
		assert.False(t, hasValue, "the canonical form drops the value")
	})

	t.Run("an unguarded date comparison warns (C11), it does not reject", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filters":[{"property":"dueDate","condition":"less","datePreset":"today"}]}}`), "", false)

		require.NoError(t, err)
		require.NotEmpty(t, result.Warnings, "the §6.2 empty-date trap rides the warnings channel")
		assert.Contains(t, result.Warnings[0].Path, "ops[0].set.filters")
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1, "the filter is stored despite the warning")
	})

	t.Run("the compact filter string parses into stored filters", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filter":"severity = \"High\""}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		leaf := filters[0].(map[string]any)
		assert.Equal(t, "severity", leaf["property"])
		assert.Equal(t, "opt-high", leaf["value"], "the option name resolved to its id")
	})

	t.Run("filter and filters together are ambiguous", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filter":"name != \"\"","filters":[]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	})

	t.Run("a filter-string parse error lands on the op's field", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"filter":"severity ="}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "parse error")
	})

	t.Run("two views need the view field; the error lists them", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, `viewAll1 ("All")`)
		assert.Contains(t, apiErr.Message, `viewBoard2 ("Board")`)
	})

	t.Run("a view resolves by unique id suffix", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","view":"Board2","columns":{"severity":{"hidden":false}}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[1]
		severity := columnByProperty(t, view, "severity")
		_, stillHidden := severity["hidden"]
		assert.False(t, stillHidden)
	})

	t.Run("an unknown view lists the available ones", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","view":"viewGone9","set":{"name":"X"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, `viewAll1 ("All")`)
	})

	t.Run("an object without a dataview says so", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"name":"X"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "no dataview block")
	})

	t.Run("two dataviews need the block field; an explicit one targets it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","set":{"name":"X"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "dvFirst1")
		assert.Contains(t, apiErr.Message, "dvSecond2")
	})

	t.Run("block targeting reaches the named dataview only", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoDataviewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","block":"dvSecond2","columns":{"name":{"hidden":false}}}`), "", false)

		require.NoError(t, err)
		second := viewsOf(t, dataviewOf(t, *captured, "dvSecond2"))[0]
		nameCol := columnByProperty(t, second, "name")
		_, stillHidden := nameCol["hidden"]
		assert.False(t, stillHidden)
		first := viewsOf(t, dataviewOf(t, *captured, "dvFirst1"))[0]
		require.Len(t, columnsOf(t, first), 1, "the other dataview is untouched")
	})

	t.Run("a non-dataview block is rejected by type", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","block":"blockPara1","set":{"name":"X"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "not a dataview")
	})

	t.Run("an empty op is rejected with the schema pointer", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "set and/or columns")
		assert.Contains(t, apiErr.Issues[0].Hint, "schemas/ops/updateView")
	})

	t.Run("a failing updateView leaves the batch atomic", func(t *testing.T) {
		// the op validates against a private copy: an invalid second op must
		// not leave the first op's changes half-applied to the view document
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(
				`{"op":"updateView","columns":{"severity":{"hidden":false}}}`,
				`{"op":"updateView","set":{"type":"board"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Issues[0].Path, "ops[1]")
	})

	t.Run("groups and objectOrders round-trip untouched", func(t *testing.T) {
		// kanban editor state is output-only (§4a) but must SURVIVE a view
		// edit — the whole-block reimport keeps it
		doc := `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"key":"name","format":"text"},{"key":"severity","format":"select"}],` +
			`"views":[{"id":"viewBoard1","name":"Board","type":"kanban","groupBy":"severity",` +
			`"columns":[{"property":"name"},{"property":"severity","hidden":true}],` +
			`"groups":[{"id":"groupA","backgroundColor":"red"},{"id":"groupB","hidden":true}],` +
			`"objectOrders":[{"groupId":"groupA","objectIds":["objX","objY"]}]}]}]}`
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","columns":{"severity":{"hidden":false}}}`), "", false)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		groups, _ := view["groups"].([]any)
		require.Len(t, groups, 2, "kanban group order survives the edit")
		assert.Equal(t, "groupA", groups[0].(map[string]any)["id"])
		orders, _ := view["objectOrders"].([]any)
		require.Len(t, orders, 1, "manual object order survives the edit")
	})
}

// TestUpdateViewSchema pins the served op schema: it must exist, parse, and
// carry the example that repairs the reported gap in one line.
func TestUpdateViewSchema(t *testing.T) {
	fx := newV2Fixture(t)

	entry, err := fx.SchemaOp("updateView")

	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(entry.Schema, &schema), "the served schema must be valid JSON")
	assert.Contains(t, string(entry.Example), `"hidden":false`)
	var example map[string]any
	require.NoError(t, json.Unmarshal(entry.Example, &example))
}
