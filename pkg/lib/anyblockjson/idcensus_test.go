package anyblockjson

// idcensus_test.go pins buildLabelPlan's LOCAL census (§9a): the population
// is what the export SERVES, not what the snapshot holds. objectcensus_test.go
// pins the other half — the object ids the same plan reserves.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// censusOf runs the label plan's population probe over a snapshot.
func censusOf(sn *model.SmartBlockSnapshotBase, opts Options) map[string]bool {
	e := &exporter{opts: opts, snapshot: sn, blocks: map[string]*model.Block{}, visited: map[string]bool{}}
	e.indexBlocks()
	return e.emittedLocalIds()
}

// servedLocalIds reads back every doc-local id a rendered document spells:
// block ids, table row/column ids, dataview view ids. It parses the OUTPUT,
// so it is an independent statement of "what the document says" — the census
// probe walks the snapshot, this walks the bytes.
func servedLocalIds(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	var doc struct {
		Blocks []struct {
			Id      string `json:"id"`
			Columns []struct {
				Id string `json:"id"`
			} `json:"columns"`
			Rows []struct {
				Id string `json:"id"`
			} `json:"rows"`
			Views []struct {
				Id string `json:"id"`
			} `json:"views"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	out := map[string]bool{}
	add := func(id string) {
		if id != "" {
			out[id] = true
		}
	}
	for _, b := range doc.Blocks {
		add(b.Id)
		for _, c := range b.Columns {
			add(c.Id)
		}
		for _, r := range b.Rows {
			add(r.Id)
		}
		for _, v := range b.Views {
			add(v.Id)
		}
	}
	return out
}

// TestExport_CensusPopulationIsWhatExportEmits is the agreement check the
// probe rests on: the census population must be exactly the ids the served
// document spells. A recording site missed (table rows, dataview views) or a
// drop rule the probe disagrees with shows up here as a set difference.
//
// The snapshots below all carry clean ids, so the stored id and the id
// written for it are the same string — which is what lets the output be read
// back as the population.
func TestExport_CensusPopulationIsWhatExportEmits(t *testing.T) {
	for _, tc := range []struct {
		name string
		snap *model.SmartBlockSnapshotBase
	}{
		{"rich snapshot", richSnapshot()},
		{"content-less and structural blocks", &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"title", "ghost", "para", "orphan"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				textBlock("title", model.BlockContentText_Title, "Title"),
				{Id: "ghost"}, // content-less, childless: dropped (§7)
				textBlock("para", model.BlockContentText_Paragraph, "kept"),
				{Id: "orphan", ChildrenIds: []string{"unreachable"}}, // content-less with children
				textBlock("unreachable", model.BlockContentText_Paragraph, "under the content-less block"),
			},
			Details: fields(map[string]*types.Value{"id": str("root"), "name": str("Title")}),
		}},
		{"children of a leaf block are not emitted", &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"bm"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "bm", ChildrenIds: []string{"belowLeaf"},
					Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{Url: "https://anytype.io"}}},
				textBlock("belowLeaf", model.BlockContentText_Paragraph, "never served"),
			},
			Details: fields(map[string]*types.Value{"id": str("root")}),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, tc.snap, testOptions())
			require.NoError(t, err)
			require.NoError(t, Validate(data))

			assert.Equal(t, servedLocalIds(t, data), censusOf(tc.snap, testOptions()))
		})
	}
}

// TestExport_CompactIdsSurviveARoundTrip is guarantee 3 (§11) on the API's
// default read shape: a block the document does not spell must not reserve a
// suffix slot, or the read before a round trip and the read after it disagree
// about whether a served block compacts.
//
// The two ids below share the 5-char tail `183ba`, which is the collision a
// real production template carries between a `Layout_Div` and a paragraph.
func TestExport_CompactIdsSurviveARoundTrip(t *testing.T) {
	const (
		servedId = "aaaaaaaaaaaaaaaaaaa183ba" // 24 lowercase hex: relabels
		ghostId  = "bbbbbbbbbbbbbbbbbbb183ba" // same tail, never served
	)
	for _, tc := range []struct {
		name  string
		ghost *model.Block
		extra []*model.Block
	}{
		// the production shape: a Layout_Div wrapper and a paragraph whose
		// minted ids end in the same five characters
		{"transparent container", divBlock(ghostId, "inner"),
			[]*model.Block{textBlock("inner", model.BlockContentText_Paragraph, "inside the container")}},
		{"structural block", textBlock(ghostId, model.BlockContentText_Title, "Title"), nil},
		{"content-less leaf", &model.Block{Id: ghostId}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := &model.SmartBlockSnapshotBase{
				Blocks: append([]*model.Block{
					{Id: "root", ChildrenIds: []string{ghostId, servedId},
						Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
					tc.ghost,
					textBlock(servedId, model.BlockContentText_Paragraph, "served"),
				}, tc.extra...),
				Details: fields(map[string]*types.Value{"id": str("root"), "name": str("Title")}),
			}
			opts := testOptions()
			opts.CompactIds = true

			first, err := Marshal(model.SmartBlockType_Page, snap, opts)
			require.NoError(t, err)
			require.NoError(t, Validate(first))
			// the positive half: the ghost does not hold the slot, so the
			// served block relabels. Without this the test would pass on a
			// document that simply never compacts anything.
			assert.Contains(t, string(first), `"id": "183ba"`)

			_, reimported, err := Unmarshal(first, opts)
			require.NoError(t, err)
			second, err := Marshal(model.SmartBlockType_Page, reimported, opts)
			require.NoError(t, err)

			assert.Equal(t, string(first), string(second),
				"Export(S) and Export(Import(Export(S))) must agree (§11 guarantee 3)")
		})
	}
}
