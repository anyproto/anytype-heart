package anyblockjson

// objectcensus_test.go pins buildLabelPlan's OBJECT census (export.go, §9a),
// position by position.
//
// Only one population of ids relabels — doc-local block/row/column/view ids —
// but the plan walks a second one to build the `fullIds` avoid-set: every
// OBJECT id the document references, each of which is now spelled verbatim
// somewhere in the output. mintedSuffixLabels' own census counts local ids
// only, so that avoid-set is the sole thing standing between a minted block
// and a label that already names something else in the same document
// (TestExport_CompactLabelCannotTakeAServedId states the rule; this file
// states that every arm of the walk is live).
//
// It is a coverage file, and it exists because of a specific accident that
// nearly happened: 42396b448 deleted a `compactObjectId` call from all
// thirteen of these positions at once, and afterwards the whole census was
// held by ONE of them (a text mark). A surgery that dropped an arm would have
// been silently green. Each subtest below plants the SAME 5-hex-character
// object id at exactly one position, next to a minted block whose 5-character
// suffix is that string, and asserts the block keeps its full id — which it
// can only do while that position feeds the avoid-set.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// censusObject is compactIdMinLen wide and hex-lower, which is exactly
	// the shape of a minted label: an object id that a block could steal.
	censusObject = "abcde"
	// censusBlock is a 24-hex minted block id (isMintedLocalId) whose
	// last-5 suffix is censusObject, so it is the relabel candidate that
	// wants that very label.
	censusBlock = "0000000000000000000abcde"
)

// censusSnapshot wires one block under a root, with an explicit envelope id
// so the object id under test is the only interesting string in the document.
func censusSnapshot(block *model.Block, details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	all := map[string]*types.Value{"id": str("obj1"), "name": str("Ticket")}
	for k, v := range details {
		all[k] = v
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{block.Id},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			block,
		},
		Details: fields(all),
	}
}

// censusBlockId exports under CompactBlockLabels and returns the id written
// for the document's single block. It also insists the object id is really
// spelled in the output: a fixture whose object id never reaches the document
// would pass the label assertion for the wrong reason (nothing to collide
// with), which is precisely the trap this file is built to avoid.
func censusBlockId(t *testing.T, snap *model.SmartBlockSnapshotBase) string {
	t.Helper()
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{CompactBlockLabels: true})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "%s", data)
	assert.Contains(t, string(data), censusObject,
		"the fixture only bites while the object id is spelled in the document:\n%s", data)

	var doc struct {
		Blocks []struct {
			Id string `json:"id"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Blocks, 1, "%s", data)
	return doc.Blocks[0].Id
}

// censusTextBlock is a text block carrying one mark of the given type.
func censusTextBlock(markType model.BlockContentTextMarkType, param string) *model.Block {
	return &model.Block{Id: censusBlock, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: "x", Marks: &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 0, To: 1},
				Type:  markType, Param: param}}}}}}
}

// censusDataviewBlock wraps a dataview whose one property is object-formatted,
// so filter values and custom orders resolve down the object branch.
func censusDataviewBlock(dv *model.BlockContentDataview) *model.Block {
	dv.RelationLinks = []*model.RelationLink{{Key: "assignee", Format: model.RelationFormat_object}}
	return &model.Block{Id: censusBlock, Content: &model.BlockContentOfDataview{Dataview: dv}}
}

// TestExport_TheObjectCensusCoversEveryPosition walks every arm of
// buildLabelPlan's object walk. Neutering any single arm — dropping its
// addObject call — makes exactly its subtest fail, because the id it no
// longer reserves is then free for the minted block to take as a label.
func TestExport_TheObjectCensusCoversEveryPosition(t *testing.T) {
	cases := map[string]*model.SmartBlockSnapshotBase{
		// --- text marks ---
		"a mention mark's target": censusSnapshot(
			censusTextBlock(model.BlockContentTextMark_Mention, censusObject), nil),
		"an object mark's target": censusSnapshot(
			censusTextBlock(model.BlockContentTextMark_Object, censusObject), nil),
		"an object URL behind a link mark": censusSnapshot(
			censusTextBlock(model.BlockContentTextMark_Link, objectLinkDest(censusObject)), nil),
		"a callout's icon image": censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "x", Style: model.BlockContentText_Callout, IconImage: censusObject}}}, nil),

		// --- file, bookmark, link blocks ---
		"a file block's target": censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				TargetObjectId: censusObject, Type: model.BlockContentFile_Image}}}, nil),
		"a file block's legacy hash": censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				Hash: censusObject, Type: model.BlockContentFile_Image}}}, nil),
		"a bookmark's target": censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				Url: "https://anytype.io", TargetObjectId: censusObject}}}, nil),
		"a link block's target": censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: censusObject}}}, nil),

		// --- dataview ---
		"a dataview's target": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{TargetObjectId: censusObject}), nil),
		"a view's default template": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{
				{Id: "v1", Name: "All", DefaultTemplateId: censusObject}}}), nil),
		"a view's default type": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{
				{Id: "v1", Name: "All", DefaultObjectTypeId: censusObject}}}), nil),
		"an object-valued filter": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{
				{Id: "v1", Name: "All", Filters: []*model.BlockContentDataviewFilter{{
					Id: "f1", RelationKey: "assignee", Format: model.RelationFormat_object,
					Condition: model.BlockContentDataviewFilter_In,
					Value:     strList(censusObject)}}}}}), nil),
		"an object-valued sort's custom order": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{
				{Id: "v1", Name: "All", Sorts: []*model.BlockContentDataviewSort{{
					Id: "s1", RelationKey: "assignee", Format: model.RelationFormat_object,
					Type:        model.BlockContentDataviewSort_Custom,
					CustomOrder: []*types.Value{str(censusObject)}}}}}}), nil),
		"a view's object order": censusSnapshot(censusDataviewBlock(
			&model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "v1", Name: "All"}},
				ObjectOrders: []*model.BlockContentDataviewObjectOrder{{
					ViewId: "v1", GroupId: "g1", ObjectIds: []string{censusObject}}}}), nil),

		// --- the envelope ---
		"an object-valued property": censusSnapshot(
			&model.Block{Id: censusBlock, Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "x"}}},
			map[string]*types.Value{"assignee": strList(censusObject)}),
	}
	for name, snap := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, censusBlock, censusBlockId(t, snap),
				"the object id at this position must be reserved, so the minted block cannot label itself with it")
		})
	}

	// The collection census is the one position that is not a block or a
	// property: the items list lives on Collections, and export lifts it into
	// the envelope's `items`.
	t.Run("a collection item", func(t *testing.T) {
		snap := censusSnapshot(&model.Block{Id: censusBlock,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "x"}}}, nil)
		snap.Collections = fields(map[string]*types.Value{storeKeyItems: strList(censusObject)})
		assert.Equal(t, censusBlock, censusBlockId(t, snap),
			"a collection item is an object reference like any other")
	})
}
