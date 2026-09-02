package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// editSetDoc is a set with the one dataview block sets carry (fixed id
// "dataview"): one "All" view whose custom columns are hidden — the exact
// shape the gap report described on a freshly generated type/set view.
const editSetDoc = `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"},{"property":"dueDate","format":"date"}],` +
	`"views":[{"id":"viewAll1","name":"All",` +
	`"sorts":[{"property":"dueDate"}],` +
	`"columns":[{"property":"name"},{"property":"severity","hidden":true,"width":100},{"property":"dueDate","hidden":true,"width":120}]}]}]}`

// editTwoViewsDoc carries two views, so view targeting is required.
const editTwoViewsDoc = `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
	`"views":[` +
	`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},` +
	`{"id":"viewBoard2","name":"Board","type":"kanban","group_by":"severity","columns":[{"property":"name"},{"property":"severity","hidden":true}]}]}]}`

// editTwoDataviewsDoc is a page with two inline dataviews — block targeting
// is required.
const editTwoDataviewsDoc = `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[` +
	`{"id":"blockPara1","type":"paragraph","text":"intro"},` +
	`{"id":"dvFirst1","type":"dataview","properties":[{"property":"name","format":"text"}],"views":[{"id":"viewA1","name":"A","columns":[{"property":"name"}]}]},` +
	`{"id":"dvSecond2","type":"dataview","properties":[{"property":"name","format":"text"}],"views":[{"id":"viewB1","name":"B","columns":[{"property":"name","hidden":true}]}]}]}`

// editTypeDoc is a kind:"object_type" document with the type's own dataview —
// the reporter's actual target (the default "All" view of a custom type).
const editTypeDoc = `{"formatVersion":"2.0","kind":"object_type","id":"obj1","type_settings":{"api_key":"plant"},"properties":{"name":"Plant"},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
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
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		severity := columnByProperty(t, view, "severity")
		require.NotNil(t, severity)
		_, stillHidden := severity["hidden"]
		assert.False(t, stillHidden, "hidden flipped off (visible = omitted on export)")
		assert.Equal(t, float64(100), severity["width"], "the column's other fields survive the merge")
		dueDate := columnByProperty(t, view, "due_date")
		require.NotNil(t, dueDate)
		assert.Equal(t, true, dueDate["hidden"], "unnamed columns are untouched")
		assert.Equal(t, float64(120), dueDate["width"])
		assert.Equal(t, "All", view["name"], "view fields are untouched")
		sorts, _ := view["sorts"].([]any)
		require.Len(t, sorts, 1, "sorts are untouched")
	})

	t.Run("update_view works on a type document (the reported target)", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTypeDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		severity := columnByProperty(t, view, "severity")
		require.NotNil(t, severity)
		_, stillHidden := severity["hidden"]
		assert.False(t, stillHidden)
	})

	t.Run("update_view needs neither restriction axis — the M1 lesson applied", func(t *testing.T) {
		// sets, collections AND object types all carry Restrictions_Blocks —
		// classifying the view edit as a block op would refuse it on exactly
		// the objects it exists to edit. This test fails if v2OpEditNeeds ever
		// reclassifies update_view.
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
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", false, true)

		require.NoError(t, err, "a blocks-and-details-restricted set must still accept a view edit")
		assert.Equal(t, apicore.EditNeeds{}, got, "update_view must demand neither restriction axis")
	})

	t.Run("the dry run reaches the same verdict on a restricted object (C9)", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editSetDoc)
		read.BlocksRefused = blocksRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", true, true)

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
			patchBody(`{"op":"update_view","columns":{"tags":{"width":90}}}`), "", false, true)

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
			keys = append(keys, p.(map[string]any)["property"].(string))
		}
		assert.Contains(t, keys, "tags", "the properties list gains the key so its format rehydrates")
	})

	t.Run("null removes a column; removing an absent one is a no-op", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"dueDate":null,"neverWasAColumn":null}}`), "", false, true)

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
			patchBody(`{"op":"update_view","columns":{"severty":{"hidden":false}}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "ops[0].columns.severty", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "severity", "the hint suggests the close key")
	})

	t.Run("an unknown column field lists the allowed ones", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"severity":{"visible":true}}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "allowed: hidden, width, align, aggregation")
		assert.Equal(t, "ops[0].columns.severity.visible", apiErr.Issues[0].Path)
	})

	t.Run("set merges view fields and validates enums", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"name":"Open bugs","type":"kanban","group_by":"severity"}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		assert.Equal(t, "Open bugs", view["name"])
		assert.Equal(t, "kanban", view["type"])
		assert.Equal(t, "severity", view["group_by"])
		require.Len(t, columnsOf(t, view), 3, "columns are untouched by a set-only op")
	})

	t.Run("an unknown enum value lists the vocabulary", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"type":"board"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "table, list, gallery, kanban, calendar, graph")
	})

	t.Run("set.columns is steered to the columns channel", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"columns":[{"property":"name"}]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "columns channel")
	})

	t.Run("output-only editor state is rejected by name", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"groups":[]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "output-only")
	})

	t.Run("an unknown view field lists the allowed set", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"nam":"Open"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown view field "nam"`)
		assert.Contains(t, apiErr.Issues[0].Hint, "name")
	})

	t.Run("null clears a view field back to its default", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"viewBoard2","set":{"group_by":null,"type":null}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[1]
		_, hasGroupBy := view["group_by"]
		assert.False(t, hasGroupBy)
		_, hasType := view["type"]
		assert.False(t, hasType, "type cleared = the table default (omitted on export)")
	})

	t.Run("sorts replace whole and are vocabulary-checked", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"sorts":[{"property":"severity","direction":"desc"}]}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"sorts":[{"property":"severity","direction":"down"}]}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"filters":[{"operator":"and","property":"severity","condition":"equal","value":"High"}]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Issues[0].Path, "ops[0].set.filters[0]")
	})

	t.Run("a bad filter condition gets the vocabulary, path-addressed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"severity","condition":"equals","value":"High"}]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.filters[0].condition", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "equal")
	})

	t.Run("a filter naming an existing option stores its id (name is the identity)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t) // "severity" with existing option "High" (opt-high)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"severity","condition":"equal","value":"High"}]}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"severity","condition":"equal","value":"BrandNew"}]}}`), "", true, true)

		require.NoError(t, err)
		require.NotNil(t, result.Created, "the M5 machinery must see update_view's option channel")
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "BrandNew", result.Created.Options[0].Name)
	})

	t.Run("the M5 bound counts update_view's filter options", func(t *testing.T) {
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
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"tags","condition":"in","value":[`+joinStrings(names, ",")+`]}]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "too many new options")
	})

	t.Run("an empty-condition value is stripped, creating nothing (§11)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"severity","condition":"empty","value":"Ghost"}]}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"dueDate","condition":"less","date_preset":"today"}]}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"filter":"severity = \"High\""}}`), "", false, true)

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
			patchBody(`{"op":"update_view","set":{"filter":"name != \"\"","filters":[]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	})

	t.Run("a filter-string parse error lands on the op's field", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filter":"severity ="}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "parse error")
	})

	t.Run("two views need the view field; the error lists them", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, `viewAll1 ("All")`)
		assert.Contains(t, apiErr.Message, `viewBoard2 ("Board")`)
	})

	t.Run("a view resolves by unique id suffix", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"Board2","columns":{"severity":{"hidden":false}}}`), "", false, true)

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
			patchBody(`{"op":"update_view","view":"viewGone9","set":{"name":"X"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, `viewAll1 ("All")`)
	})

	t.Run("an object without a dataview says so", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"name":"X"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "no dataview block")
	})

	t.Run("two dataviews need the block field; an explicit one targets it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"name":"X"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "dvFirst1")
		assert.Contains(t, apiErr.Message, "dvSecond2")
	})

	t.Run("block targeting reaches the named dataview only", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoDataviewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","block":"dvSecond2","columns":{"name":{"hidden":false}}}`), "", false, true)

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
			patchBody(`{"op":"update_view","block":"blockPara1","set":{"name":"X"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "not a dataview")
	})

	t.Run("an empty op is rejected with the schema pointer", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "set and/or columns")
		assert.Contains(t, apiErr.Issues[0].Hint, "schemas/ops/update_view")
	})

	t.Run("a failing update_view leaves the batch atomic", func(t *testing.T) {
		// the op validates against a private copy: an invalid second op must
		// not leave the first op's changes half-applied to the view document
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(
				`{"op":"update_view","columns":{"severity":{"hidden":false}}}`,
				`{"op":"update_view","set":{"type":"board"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Issues[0].Path, "ops[1]")
	})

	t.Run("groups and objectOrders round-trip untouched", func(t *testing.T) {
		// kanban editor state is output-only (§4a) but must SURVIVE a view
		// edit — the whole-block reimport keeps it
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
			`"views":[{"id":"viewBoard1","name":"Board","type":"kanban","group_by":"severity",` +
			`"columns":[{"property":"name"},{"property":"severity","hidden":true}],` +
			`"groups":[{"id":"groupA","background_color":"red"},{"id":"groupB","hidden":true}],` +
			`"object_orders":[{"group_id":"groupA","object_ids":["objX","objY"]}]}]}]}`
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"severity":{"hidden":false}}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		groups, _ := view["groups"].([]any)
		require.Len(t, groups, 2, "kanban group order survives the edit")
		assert.Equal(t, "groupA", groups[0].(map[string]any)["id"])
		orders, _ := view["object_orders"].([]any)
		require.Len(t, orders, 1, "manual object order survives the edit")
	})
}

// TestUpdateViewSchema pins the served op schema: it must exist, parse, and
// carry the example that repairs the reported gap in one line.
func TestUpdateViewSchema(t *testing.T) {
	fx := newV2Fixture(t)

	entry, err := fx.SchemaOp("update_view")

	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(entry.Schema, &schema), "the served schema must be valid JSON")
	assert.Contains(t, string(entry.Example), `"hidden":false`)
	var example map[string]any
	require.NoError(t, json.Unmarshal(entry.Example, &example))
}

// TestViewFamilyOps covers insert_view / move_view / delete_view (§8.18).
func TestViewFamilyOps(t *testing.T) {
	ctx := context.Background()

	t.Run("bare insert_view appends a usable view: every property visible, latest first", func(t *testing.T) {
		// given: the create-defaults decision — NOT the native CreateView
		// default, which hides every column but name (the GO-5969 disease)
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Recent"}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.CreatedViews, 1)
		newId := result.CreatedViews["ops[0]"]
		assert.Len(t, newId, 24, "the view id is server-minted, editor-shaped")
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 2)
		view := views[1]
		assert.Equal(t, newId, view["id"])
		assert.Equal(t, "Recent", view["name"])
		cols := columnsOf(t, view)
		require.Len(t, cols, 3, "one column per listed property (name, severity, dueDate)")
		for _, col := range cols {
			_, hidden := col["hidden"]
			assert.False(t, hidden, "column %v must be visible — the bare default is a view someone can look at", col["property"])
		}
		sorts, _ := view["sorts"].([]any)
		require.Len(t, sorts, 1)
		assert.Equal(t, "last_modified_date", sorts[0].(map[string]any)["property"])
		assert.Equal(t, "desc", sorts[0].(map[string]any)["direction"])
	})

	t.Run("insert_view with set and columns is update_view aimed at a fresh view", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Board","set":{"type":"kanban","group_by":"severity"},"columns":{"dueDate":{"hidden":true}}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[1]
		assert.Equal(t, "kanban", view["type"])
		assert.Equal(t, "severity", view["group_by"])
		dueDate := columnByProperty(t, view, "due_date")
		require.NotNil(t, dueDate)
		assert.Equal(t, true, dueDate["hidden"], "the columns channel merges onto the defaults")
	})

	t.Run("copy_from duplicates everything but identity, then set overrides", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Board copy","copy_from":"viewBoard2","set":{"card_size":"large"}}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 3)
		copied := views[2]
		assert.Equal(t, "Board copy", copied["name"])
		assert.Equal(t, "kanban", copied["type"], "the source view's type is copied")
		assert.Equal(t, "severity", copied["group_by"], "the source's grouping is copied")
		assert.Equal(t, "large", copied["card_size"], "set overrides on top of the copy")
		require.Len(t, columnsOf(t, copied), 2, "the source's columns are copied")
		assert.NotEqual(t, "viewBoard2", copied["id"], "the copy gets a fresh id")
		assert.Equal(t, result.CreatedViews["ops[0]"], copied["id"])
	})

	t.Run("insert_view targets a position; first is the default tab", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Lead","position":"first"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 3)
		assert.Equal(t, "Lead", views[0]["name"], "position first leads the list — the client's default tab")
		assert.Equal(t, "All", views[1]["name"])
	})

	t.Run("insert_view after a view ref lands between", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Middle","after":"viewAll1"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		assert.Equal(t, "Middle", views[1]["name"])
	})

	t.Run("insert_view needs a name", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].name", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "schemas/ops/insert_view")
	})

	t.Run("insert_view with an unknown copy_from lists the views", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"X","copy_from":"viewGone9"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, `viewAll1 ("All")`)
	})

	t.Run("insert_view with two targeting fields is ambiguous", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"X","after":"viewAll1","position":"first"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "at most one of after, before, position")
	})

	t.Run("insert_view validates its channels like update_view", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"X","set":{"type":"board"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "table, list, gallery, kanban, calendar, graph")
	})

	t.Run("move_view reorders without resending the list", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"move_view","view":"viewBoard2","position":"first"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 2)
		assert.Equal(t, "viewBoard2", views[0]["id"], "Board is now the default tab")
		assert.Equal(t, "viewAll1", views[1]["id"])
		require.Len(t, columnsOf(t, views[0]), 2, "the moved view's content is untouched")
	})

	t.Run("move_view after a later view adjusts the splice correctly", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"move_view","view":"viewAll1","after":"viewBoard2"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		assert.Equal(t, "viewBoard2", views[0]["id"])
		assert.Equal(t, "viewAll1", views[1]["id"])
	})

	t.Run("move_view requires the view", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"move_view","position":"first"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].view", apiErr.Issues[0].Path)
	})

	t.Run("delete_view removes the view and its editor state", func(t *testing.T) {
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
			`"views":[` +
			`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},` +
			`{"id":"viewBoard2","name":"Board","type":"kanban","group_by":"severity","columns":[{"property":"name"}],` +
			`"groups":[{"id":"groupA","background_color":"red"}]}]}]}`
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, doc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"delete_view","view":"viewBoard2"}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 1)
		assert.Equal(t, "viewAll1", views[0]["id"])
	})

	t.Run("deleting the last view is refused — the guard, not a corrupt object", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"delete_view","view":"viewAll1"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Message, "cannot delete the last view")
		assert.Contains(t, apiErr.Issues[0].Hint, "insert_view")
	})

	t.Run("insert-then-delete in one batch makes the last-view guard count the insert", func(t *testing.T) {
		// the batch is atomic and sequential: after insert_view there are two
		// views, so deleting the formerly-only one is legal — replacing a
		// type's default view is exactly this two-op batch
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(
				`{"op":"insert_view","name":"Better","copy_from":"viewAll1","columns":{"severity":{"hidden":false}}}`,
				`{"op":"delete_view","view":"viewAll1"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 1)
		assert.Equal(t, "Better", views[0]["name"])
		assert.Equal(t, result.CreatedViews["ops[0]"], views[0]["id"])
	})

	t.Run("the whole view family needs neither restriction axis", func(t *testing.T) {
		// the M1 pin for the family: a set refusing BOTH axes must still take
		// a create+move+delete batch. Fails if any of the three is ever
		// reclassified in v2OpEditNeeds.
		fx := newV2Fixture(t)
		read := editRead(t, editTwoViewsDoc)
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
			patchBody(
				`{"op":"insert_view","name":"Extra"}`,
				`{"op":"move_view","view":"viewBoard2","position":"first"}`,
				`{"op":"delete_view","view":"viewAll1"}`), "", false, true)

		require.NoError(t, err, "a fully restricted set must still accept the whole view family")
		assert.Equal(t, apicore.EditNeeds{}, got)
	})

	t.Run("the M5 bound counts insert_view's filter options", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editSetDoc), nil)

		names := make([]string, 0, v2MaxCreatedOptionsPerPatch+1)
		for i := 0; i <= v2MaxCreatedOptionsPerPatch; i++ {
			names = append(names, `"NewTag`+string(rune('A'+i%26))+string(rune('0'+i/26))+`"`)
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Tagged","set":{"filters":[{"property":"tags","condition":"in","value":[`+joinStrings(names, ",")+`]}]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "too many new options")
	})
}

// TestViewOpReviewFixes covers the §8.19 review findings: the no-create
// commit with live-proto restore (A), the dataview-aware render bound (B),
// the M7 registration coupling (C), the schema fixes (D/E) and the minors.
func TestViewOpReviewFixes(t *testing.T) {
	ctx := context.Background()

	// editDanglingDoc: view B's filter holds a value that resolves to no
	// option — the state a deleted tag leaves behind.
	const editDanglingDoc = `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
		`"views":[` +
		`{"id":"viewAll1","name":"All","columns":[{"property":"name"},{"property":"severity","hidden":true}]},` +
		`{"id":"viewOld2","name":"Old","columns":[{"property":"name"}],` +
		`"filters":[{"property":"severity","condition":"equal","value":"bafyDanglingOpt1"}]}]}]}`

	t.Run("A: a dangling option reference survives an op on another view verbatim", func(t *testing.T) {
		// before the fix, the commit re-imported EVERY view through the
		// creating resolver: the dangling value exported as its raw id and
		// round-tripped into a brand-new option named after it — created
		// under the object lock, past both halves of M5, by an op that never
		// touched the view
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectMutate(editRead(t, editDanglingDoc), "headB")
		// no ObjectCreateRelationOption expectation: any create RPC fails the test

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"move_view","view":"viewOld2","position":"first"}`), "", false, true)

		require.NoError(t, err)
		assert.Nil(t, result.Created, "an op that authors no values must create nothing")
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Equal(t, "viewOld2", views[0]["id"], "the move itself happened")
		filters, _ := views[0]["filters"].([]any)
		require.Len(t, filters, 1)
		assert.Equal(t, "bafyDanglingOpt1", filters[0].(map[string]any)["value"],
			"the dangling value is untouched — not rebound, not minted into an option")
	})

	t.Run("A: an untouched view keeps its exact option id when a twin shares the name", func(t *testing.T) {
		// two options legally share a name; export writes the NAME, so a
		// re-import re-picks by store listing order — the untouched view must
		// instead be restored from the live proto, keeping ITS option
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-severity"),
				bundle.RelationKeyRelationKey:    domain.String("severity"),
				bundle.RelationKeyName:           domain.String("Severity"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("optTwinA"),
				bundle.RelationKeyRelationKey:    domain.String("severity"),
				bundle.RelationKeyName:           domain.String("High"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				bundle.RelationKeyId:             domain.String("optTwinB"),
				bundle.RelationKeyRelationKey:    domain.String("severity"),
				bundle.RelationKeyName:           domain.String("High"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			},
		})
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
			`"views":[` +
			`{"id":"viewAll1","name":"All","columns":[{"property":"name"},{"property":"severity","hidden":true}]},` +
			`{"id":"viewPinned2","name":"Pinned","columns":[{"property":"name"}],` +
			`"filters":[{"property":"severity","condition":"equal","value":"optTwinB"}]}]}]}`
		captured := fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"viewAll1","columns":{"severity":{"hidden":false}}}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		filters, _ := views[1]["filters"].([]any)
		require.Len(t, filters, 1)
		assert.Equal(t, "optTwinB", filters[0].(map[string]any)["value"],
			"the untouched view's filter must keep ITS twin, not be repointed by listing order")
	})

	t.Run("A: a doc/space format disagreement passes through instead of minting under the lock", func(t *testing.T) {
		// the dataview's own properties list says select, the space says
		// longtext: the prewarm (space-informed) skips the value, so the
		// commit (dv-list-informed) must NOT create — it passes the name
		// through verbatim
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-legacy"),
				bundle.RelationKeyRelationKey:    domain.String("legacy"),
				bundle.RelationKeyName:           domain.String("Legacy"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
		})
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"legacy","format":"select"}],` +
			`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"},{"property":"legacy"}]}]}]}`
		captured := fx.expectMutate(editRead(t, doc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"legacy","condition":"equal","value":"Ghost"}]}}`), "", false, true)

		require.NoError(t, err)
		assert.Nil(t, result.Created, "the narrow prewarm/import disagreement must not mint under the lock")
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		assert.Equal(t, "Ghost", filters[0].(map[string]any)["value"], "the unresolvable value passes through verbatim")
	})

	t.Run("A: copy_from preserves the source's exact option ids", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t) // opt-high exists
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
			`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}],` +
			`"filters":[{"property":"severity","condition":"equal","value":"bafyDanglingOpt1"}]}]}]}`
		captured := fx.expectMutate(editRead(t, doc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Copy","copy_from":"viewAll1"}`), "", false, true)

		require.NoError(t, err)
		assert.Nil(t, result.Created)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 2)
		filters, _ := views[1]["filters"].([]any)
		require.Len(t, filters, 1)
		assert.Equal(t, "bafyDanglingOpt1", filters[0].(map[string]any)["value"],
			"the copy's filters restore from the source proto — no name round-trip")
	})

	t.Run("B: the render bound sees dataview weight, not one block", func(t *testing.T) {
		// a fully legal 512×insert_view batch on a wide set held the lock for
		// tens of seconds while scoring 0.05% of the budget — the dataview is
		// one block whose marshal cost is O(views × columns)
		fx := newV2Fixture(t)
		var props, cols strings.Builder
		for i := 0; i < 50; i++ {
			if i > 0 {
				props.WriteString(",")
				cols.WriteString(",")
			}
			fmt.Fprintf(&props, `{"property":"name%02d","format":"text"}`, i)
			fmt.Fprintf(&cols, `{"property":"name%02d"}`, i)
		}
		var views strings.Builder
		for v := 0; v < 10; v++ {
			if v > 0 {
				views.WriteString(",")
			}
			fmt.Fprintf(&views, `{"id":"view%02d","name":"V%d","columns":[%s]}`, v, v, cols.String())
		}
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Wide","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview","properties":[` + props.String() + `],"views":[` + views.String() + `]}]}`
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, doc), nil)
		var mutated *state.State
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				read := editRead(t, doc)
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
					return nil, err
				}
				mutated = st
				return []string{"headB"}, nil
			}).Maybe()

		ops := make([]string, 0, v2MaxOpsPerPatch)
		for i := 0; i < v2MaxOpsPerPatch; i++ {
			ops = append(ops, fmt.Sprintf(`{"op":"insert_view","name":"Extra %d"}`, i))
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(ops...), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "too much re-rendering work")
		assert.Nil(t, mutated, "the batch must be refused before any op applies")
	})

	t.Run("C: view ops rebuild the document view per op and the M7 map says so", func(t *testing.T) {
		// the marshal-count pin (the TestApplierRenderCounts pattern) coupled
		// to the v2OpRebuildsView registration: an op measured to re-marshal
		// per op must be counted by the render-work bound
		fx := newV2Fixture(t)
		edit, err := editFromRead("obj1", editRead(t, editTwoViewsDoc))
		require.NoError(t, err)
		resolvers := fx.newCreatingResolvers(ctx, testSpaceId, false, true)
		applier := newV2StateApplier(fx.Service, testSpaceId, "obj1", edit.SbType, edit.State, resolvers, errKeys{})
		_, err = applier.begin()
		require.NoError(t, err)

		op := json.RawMessage(`{"op":"move_view","view":"viewBoard2","position":"first"}`)
		for i := 0; i < 2; i++ {
			require.NoError(t, applier.apply(i, op))
		}
		_, err = applier.currentDoc()
		require.NoError(t, err)

		assert.Equal(t, 3, applier.marshalCount, "2 view ops = begin + one rebuild + the final render")
		for _, opName := range []string{"update_view", "insert_view", "move_view", "delete_view"} {
			assert.True(t, v2OpRebuildsView[opName],
				"%s re-marshals per op (measured above) and must be counted by the M7 bound", opName)
		}
	})

	t.Run("E: insert_view rejects set.name — including the null that defeated the requirement", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		for _, body := range []string{
			`{"op":"insert_view","name":"Real","set":{"name":"Sneaky"}}`,
			`{"op":"insert_view","name":"Real","set":{"name":null}}`,
		} {
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(body), "", false, true)
			apiErr := v2Err(t, err)
			assert.Equal(t, "ops[0].set.name", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Message, "top-level field")
		}
	})

	t.Run("F: an indented inline dataview is editable", func(t *testing.T) {
		// the shipped inline-dataview shape: nested under a parent block —
		// the op must strip the view-doc indent before the fragment re-import
		fx := newV2Fixture(t)
		doc := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[` +
			`{"id":"blockParent1","type":"toggle","text":"data"},` +
			`{"indent":1,"id":"dvInline1","type":"dataview","properties":[{"property":"name","format":"text"}],` +
			`"views":[{"id":"viewA1","name":"A","columns":[{"property":"name","hidden":true}]}]}]}`
		captured := fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"name":{"hidden":false}}}`), "", false, true)

		require.NoError(t, err)
		blocks := docBlocks(stateDoc(t, *captured))
		require.Len(t, blocks, 2)
		assert.Equal(t, float64(1), blocks[1]["indent"], "the dataview stays nested")
		view := viewsOf(t, blocks[1])[0]
		nameCol := columnByProperty(t, view, "name")
		_, hidden := nameCol["hidden"]
		assert.False(t, hidden)
	})

	t.Run("minor: removing a column from a column-less view is a clean no-op", func(t *testing.T) {
		fx := newV2Fixture(t)
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview","properties":[{"property":"name","format":"text"}],` +
			`"views":[{"id":"viewA1","name":"A"}]}]}`
		fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"name":null}}`), "", false, true)

		require.NoError(t, err, "a removal no-op must not write columns:null into the block")
	})

	t.Run("minor: the advertised customOrder and filters bounds are enforced", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc))

		entries := make([]string, maxV2CustomOrderValues+1)
		for i := range entries {
			entries[i] = fmt.Sprintf(`"v%d"`, i)
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"sorts":[{"property":"severity","custom_order":[`+joinStrings(entries, ",")+`]}]}}`), "", false, true)
		apiErr := v2Err(t, err)
		assert.Equal(t, "ops[0].set.sorts[0].custom_order", apiErr.Issues[0].Path)

		nodes := make([]string, maxV2ViewFilterNodes+1)
		for i := range nodes {
			nodes[i] = `{"property":"severity","condition":"not_empty"}`
		}
		_, err = fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[`+joinStrings(nodes, ",")+`]}}`), "", false, true)
		apiErr = v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "top-level filter nodes")
	})

	t.Run("minor: the filter string sees the whole dataview's keys, like the structured form", func(t *testing.T) {
		// severity is in the properties list and the OTHER view's columns but
		// not a column of the addressed view, and not in the space — the two
		// filter forms must accept the same keys
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"viewAll1","set":{"filter":"severity IS NOT EMPTY"}}`), "", false, true)

		require.NoError(t, err, "the string form must accept keys the structured form accepts")
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		assert.Equal(t, "severity", filters[0].(map[string]any)["property"])
	})

	t.Run("minor: a target-less move_view is refused, not a silent default-tab change", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoViewsDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"move_view","view":"viewAll1"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "needs a destination")
		assert.Contains(t, apiErr.Issues[0].Hint, "default tab")
	})

	t.Run("minor: two bare inserts in one batch build identical views", func(t *testing.T) {
		// the bare default must come from pre-op membership: the first
		// insert's default sort must not grow the properties list and hand
		// the second insert an extra column
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"One"}`, `{"op":"insert_view","name":"Two"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 3)
		assert.Len(t, columnsOf(t, views[1]), 3)
		assert.Len(t, columnsOf(t, views[2]), 3, "the second bare insert must not inherit a column the first one's sort minted")
	})

	t.Run("minor: copy_from carries the source's kanban editor state", func(t *testing.T) {
		fx := newV2Fixture(t)
		doc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview",` +
			`"properties":[{"property":"name","format":"text"},{"property":"severity","format":"select"}],` +
			`"views":[{"id":"viewBoard1","name":"Board","type":"kanban","group_by":"severity",` +
			`"columns":[{"property":"name"}],` +
			`"groups":[{"id":"groupA","background_color":"red"},{"id":"groupB","hidden":true}],` +
			`"object_orders":[{"group_id":"groupA","object_ids":["objX"]}]}]}]}`
		captured := fx.expectMutate(editRead(t, doc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_view","name":"Board 2","copy_from":"viewBoard1"}`), "", false, true)

		require.NoError(t, err)
		views := viewsOf(t, dataviewOf(t, *captured, "dataview"))
		require.Len(t, views, 2)
		groups, _ := views[1]["groups"].([]any)
		require.Len(t, groups, 2, "group order and colors ride the copy")
		orders, _ := views[1]["object_orders"].([]any)
		require.Len(t, orders, 1, "manual object order rides the copy")
	})
}

// TestViewSchemaDrift pins the served schemas to the implementation: the
// §6.2 enum vocabulary comes from viewvocab.go, the sorts item accepts the
// id every read emits, and insert_view's set has no name slot.
func TestViewSchemaDrift(t *testing.T) {
	fx := newV2Fixture(t)

	dig := func(t *testing.T, m map[string]any, path ...string) map[string]any {
		t.Helper()
		for _, key := range path {
			next, ok := m[key].(map[string]any)
			require.True(t, ok, "schema path %v missing at %q", path, key)
			m = next
		}
		return m
	}
	enumOf := func(t *testing.T, prop map[string]any) []string {
		t.Helper()
		raw, _ := prop["enum"].([]any)
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	entry, err := fx.SchemaOp("update_view")
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(entry.Schema, &schema))
	setProps := dig(t, schema, "properties", "set", "properties")

	t.Run("view enums match the exported vocabulary", func(t *testing.T) {
		assert.ElementsMatch(t, anyblockjson.ViewTypeNames(), enumOf(t, dig(t, setProps, "type")))
		assert.ElementsMatch(t, anyblockjson.ViewCardSizeNames(), enumOf(t, dig(t, setProps, "card_size")))
		assert.ElementsMatch(t, anyblockjson.ViewListSizeNames(), enumOf(t, dig(t, setProps, "list_size")))
		colProps := dig(t, schema, "properties", "columns", "additionalProperties", "properties")
		assert.ElementsMatch(t, anyblockjson.ColumnAlignNames(), enumOf(t, dig(t, colProps, "align")))
		assert.ElementsMatch(t, anyblockjson.ColumnAggregationNames(), enumOf(t, dig(t, colProps, "aggregation")))
	})

	t.Run("the sorts item accepts the id every read emits", func(t *testing.T) {
		sortProps := dig(t, setProps, "sorts", "items", "properties")
		_, hasId := sortProps["id"]
		assert.True(t, hasId, "read→edit→write of a sort must not be schema-refused")
	})

	t.Run("the filters bound is advertised", func(t *testing.T) {
		filters := dig(t, setProps, "filters")
		assert.Equal(t, float64(maxV2ViewFilterNodes), filters["maxItems"])
	})

	t.Run("insert_view's set has no name slot", func(t *testing.T) {
		entry, err := fx.SchemaOp("insert_view")
		require.NoError(t, err)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))
		insertSetProps := dig(t, schema, "properties", "set", "properties")
		_, hasName := insertSetProps["name"]
		assert.False(t, hasName, "insert_view's name is the op's required top-level field")
	})
}

// TestViewOpKeySpellings closes the §8.22/§8.23 deferral: view-op `set` and
// `columns` channels used to accept stored keys ONLY, and once the documents
// they merge into spell slugs (§7.5a) that becomes actively wrong — a
// stored-key column address stops matching the column it names. Revert
// canonicalViewKey and both subtests fail.
func TestViewOpKeySpellings(t *testing.T) {
	ctx := context.Background()

	t.Run("the stored-key spelling still addresses the column the document spells as a slug", func(t *testing.T) {
		// given — the working document spells due_date; the caller says
		// dueDate, which every other v2 channel accepts
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"dueDate":{"hidden":false}}}`), "", false, true)

		// then
		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		require.Len(t, columnsOf(t, view), 3, "it merged onto the existing column, it did not append a twin")
		dueDate := columnByProperty(t, view, "due_date")
		require.NotNil(t, dueDate)
		_, stillHidden := dueDate["hidden"]
		assert.False(t, stillHidden)
	})

	t.Run("a folded spelling lands as the document's, not as itself", func(t *testing.T) {
		// given — the fold layer (§7.5a-3) is server-side everywhere else;
		// unfolded, `DueDate` would be written verbatim into the stored
		// groupBy, where nothing can ever match it
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"group_by":"DueDate","sorts":[{"property":"due-date","direction":"desc"}]}}`), "", false, true)

		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		assert.Equal(t, "due_date", view["group_by"])
		sorts, _ := view["sorts"].([]any)
		require.Len(t, sorts, 1)
		assert.Equal(t, "due_date", sorts[0].(map[string]any)["property"])
	})
}
