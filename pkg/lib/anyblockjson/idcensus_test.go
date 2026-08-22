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

	"github.com/anyproto/anytype-heart/core/domain"
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

			// The census is what the round trip REBUILDS, which is what the
			// document spells PLUS the cell ids a table implies. A cell
			// carries no id in the flat form, but import re-derives
			// `rowId-colId` from row and column ids that are spelled, so the
			// same ids come back — and they have to be reserved, or a column
			// compacts to a label its own cells share as a suffix in the live
			// object (see emittedLocalIds). Every OTHER unspelled block —
			// container, structural, content-less, unreachable — is genuinely
			// gone and must stay out, which is what the two shapes below pin.
			want := servedLocalIds(t, data)
			for _, id := range derivedCellIdsOf(tc.snap, testOptions()) {
				want[id] = true
			}
			assert.Equal(t, want, censusOf(tc.snap, testOptions()))
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

// derivedCellIdsOf lists the `rowId-colId` ids a snapshot's tables imply. It
// walks the snapshot itself rather than calling the exporter's own helper, so
// it is an INDEPENDENT statement of what the census owes — the same reason
// servedLocalIds parses the output bytes instead of asking the exporter.
//
// Shape (§6.1): a table's children are two layout wrappers, TableColumns and
// TableRows; the column ids and row ids are their children, and every
// (row, column) pair implies a cell.
func derivedCellIdsOf(snap *model.SmartBlockSnapshotBase, _ Options) []string {
	byId := map[string]*model.Block{}
	for _, b := range snap.GetBlocks() {
		if b != nil && b.Id != "" {
			byId[b.Id] = b
		}
	}
	var out []string
	for _, b := range snap.GetBlocks() {
		if b == nil {
			continue
		}
		if _, isTable := b.Content.(*model.BlockContentOfTable); !isTable {
			continue
		}
		var cols, rows []string
		for _, wrapperId := range b.ChildrenIds {
			w := byId[wrapperId]
			l, ok := w.GetContent().(*model.BlockContentOfLayout)
			if !ok {
				continue
			}
			switch l.Layout.GetStyle() {
			case model.BlockContentLayout_TableColumns:
				cols = append(cols, w.ChildrenIds...)
			case model.BlockContentLayout_TableRows:
				rows = append(rows, w.ChildrenIds...)
			}
		}
		for _, r := range rows {
			for _, c := range cols {
				out = append(out, r+"-"+c)
			}
		}
	}
	return out
}

// countingOptions counts how many times export asks it to name an option,
// which is how a test can SEE the census probe: the probe is a second full
// block emit, so a wired resolver is asked twice. The rich fixture's dataview
// carries select filter values, so this hook is genuinely reached.
type countingOptions struct {
	n     *int
	inner OptionResolver
}

func (c countingOptions) OptionName(key domain.RelationKey, id string) (string, bool) {
	*c.n++
	return c.inner.OptionName(key, id)
}
func (c countingOptions) OptionId(key domain.RelationKey, name string) (string, bool) {
	return c.inner.OptionId(key, name)
}

// The census probe (emittedLocalIds) is a SECOND full block emit, and the most
// expensive thing a compact export does. OmitIds writes no id at all, so a
// label plan has nothing to label — running the probe for that combination
// costs a whole extra emit for output that carries no ids.
//
// The bytes are byte-identical either way, which is exactly why the waste went
// unnoticed. So this counts RESOLVER CALLS through the public Marshal instead:
// the probe asks the resolver a second time, and that is observable. A test
// that re-implemented the gate would pass no matter what the export does —
// this one fails when the gate is removed.
func TestExport_NoCensusProbeWhenNoIdIsWritten(t *testing.T) {
	snap := richSnapshot()

	callsFor := func(opts Options) int {
		n := 0
		opts.ResolveFormat = testFormatResolver
		opts.ResolveOptions = countingOptions{n: &n, inner: testResolver}
		_, err := Marshal(model.SmartBlockType_Page, snap, opts)
		require.NoError(t, err)
		return n
	}

	plain := callsFor(Options{OmitIds: true})
	compact := callsFor(Options{OmitIds: true, CompactIds: true})
	require.NotZero(t, plain, "the fixture must reach the resolver, or this proves nothing")
	assert.Equal(t, plain, compact,
		"OmitIds writes no id, so compaction must not run the census probe")

	// the control: WITHOUT OmitIds, compaction does run the probe, so the
	// assertion above cannot pass by never probing at all
	withIds := callsFor(Options{})
	withIdsCompact := callsFor(Options{CompactIds: true})
	assert.Greater(t, withIdsCompact, withIds,
		"a compact read still pays for its census (the probe is a second emit)")

	// and the bytes are unchanged by the gate
	a, err := Marshal(model.SmartBlockType_Page, snap, Options{OmitIds: true})
	require.NoError(t, err)
	b, err := Marshal(model.SmartBlockType_Page, snap, Options{OmitIds: true, CompactIds: true})
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b),
		"OmitIds decides the ids; compaction adds nothing to decide")
}
