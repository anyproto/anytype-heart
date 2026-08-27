package snapshotdiff

// widgetobject_test.go pins the comparator's side of the §2c widget-object
// omission: a widget document travels as index.widgets plus the auto-widget
// ledger, and what a bundle carries instead is WidgetsSnapshot's rebuild.
// The one skip that trip needs — the object's own timestamps and its empty
// name absent on the way back — is scoped to snapshots the omission
// predicate itself admits, so the ordinary document round trip keeps its
// full sensitivity. Taught in the same commit that taught the export
// wiring, because a comparator that learns a normalization late reports it
// as loss across a whole corpus (the drift that once produced 1,344 false
// failures in one sweep).

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const widgetTestTarget = "bafyreidft7aqr2fgy6g57hme4rmcdkynf24cd2jfhlyr3duxjevj6vewsu"

// storedWidgetObject is a widget object the way a real export holds one: the
// wrapper-and-link pair, the constant hidden-dashboard details, the ledger,
// the timestamps, and the empty name 15 of 77 corpus documents carry.
func storedWidgetObject() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "root", ChildrenIds: []string{"w1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "w1", ChildrenIds: []string{"l1"},
				Content: &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
					Layout: model.BlockContentWidget_Tree, Limit: 6}}},
			{Id: "l1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: widgetTestTarget}}},
		},
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id":                 {Kind: &types.Value_StringValue{StringValue: "root"}},
			"isHidden":           {Kind: &types.Value_BoolValue{BoolValue: true}},
			"layout":             {Kind: &types.Value_NumberValue{NumberValue: float64(model.ObjectType_dashboard)}},
			"resolvedLayout":     {Kind: &types.Value_NumberValue{NumberValue: float64(model.ObjectType_dashboard)}},
			"createdDate":        {Kind: &types.Value_NumberValue{NumberValue: 0}},
			"lastModifiedDate":   {Kind: &types.Value_NumberValue{NumberValue: 1700000000}},
			"name":               {Kind: &types.Value_StringValue{StringValue: ""}},
			"autoWidgetTargets":  {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{{Kind: &types.Value_StringValue{StringValue: "bin"}}}}}},
			"autoWidgetDisabled": {Kind: &types.Value_BoolValue{BoolValue: true}},
		}},
		ObjectTypes: []string{"ot-dashboard"},
	}
}

// Across the omission trip the rebuild carries everything the index states —
// the pair, the constants, the ledger — and only the object's own timestamps
// and its empty name come back absent. That is normalization, not loss:
// a restored sidebar is created when it is restored.
//
// How this can fail: remove the WidgetObjectResidualKey skip from Compare's
// orig-key loop and createdDate/lastModifiedDate/name all report as
// changed-to-absent — on 66 of 77 corpus spaces at once.
func TestCompare_OmittedWidgetObjectAgainstItsRebuild(t *testing.T) {
	orig := storedWidgetObject()
	require.True(t, anyblockjson.OmittedWidgetObject(model.SmartBlockType_Widget, orig),
		"the fixture must be one the omission admits, or this test pins nothing")

	var idx anyblockjson.Index
	anyblockjson.IndexFromWidgetObject(&idx, orig)
	rebuilt, err := anyblockjson.WidgetsSnapshot(&idx)
	require.NoError(t, err)
	require.NotNil(t, rebuilt)

	assert.Empty(t, Compare(orig, rebuilt, model.SmartBlockType_Widget, anyblockjson.Options{}))
}

// The skip is scoped by the omission predicate and by the residual predicate
// both, so the comparator's sensitivity survives everywhere else.
func TestCompare_WidgetSkipStaysScoped(t *testing.T) {
	t.Run("a lost timestamp on an ordinary document still reports", func(t *testing.T) {
		orig := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
			"createdDate": {Kind: &types.Value_NumberValue{NumberValue: 1700000000}},
		}}}
		got := &model.SmartBlockSnapshotBase{}
		diffs := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		require.Len(t, diffs, 1)
		assert.Contains(t, diffs[0], "createdDate")
	})

	t.Run("a widget object the omission refuses keeps full sensitivity", func(t *testing.T) {
		// a NON-empty name makes the object one the bundle keeps as a
		// document, so nothing about it is normalization
		orig := storedWidgetObject()
		orig.Details.Fields["name"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "My sidebar"}}
		require.False(t, anyblockjson.OmittedWidgetObject(model.SmartBlockType_Widget, orig))
		diffs := Compare(orig, &model.SmartBlockSnapshotBase{ObjectTypes: orig.ObjectTypes},
			model.SmartBlockType_Widget, anyblockjson.Options{})
		assert.NotEmpty(t, diffs, "every detail it loses must report, timestamps included")
	})

	t.Run("a lost ledger still reports even on an omitted object", func(t *testing.T) {
		// the ledger is LIFTED state, not residue: the rebuild writes it
		// back, so a rebuild that loses it has drifted from the lift
		orig := storedWidgetObject()
		rebuilt := storedWidgetObject()
		delete(rebuilt.Details.Fields, "autoWidgetTargets")
		diffs := Compare(orig, rebuilt, model.SmartBlockType_Widget, anyblockjson.Options{})
		require.Len(t, diffs, 1)
		assert.Contains(t, diffs[0], "autoWidgetTargets")
	})
}
