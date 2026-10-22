package state

import (
	"fmt"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Benchmarks the tree-replay cost (the un-snapshotted object open path, see
// sourceimpl.BuildState) of a big flat document produced by three editing
// patterns for "press Enter, then type into the new block":
//
//   - settext:        BlockCreate(empty) + BlockSetText into the same id (old clients)
//   - replace:        BlockCreate(empty) + BlockReplace → fresh id (LWW-fork trick)
//   - createWithText: one BlockCreate already carrying the text (virtual placeholder ideal)
//
// The editing session is simulated through the real ApplyState pipeline
// (withLayouts=true, so div-wrap normalization ops are recorded like in
// production) and the recorded changes are replayed from marshaled bytes the
// same way BuildState does: unmarshal, SetChangeId, ApplyChangeIgnoreErr per
// change, then ApplyStateFastOne + BlocksInit + Normalize + ApplyState.

type editPattern struct {
	name string
	// edit appends the changes for one "Enter + type" iteration and returns
	// the id of the block that ends up holding the text (the next anchor)
	edit func(tb testing.TB, d Doc, rec *changeRecorder, i int, prev string) string
}

type changeRecorder struct {
	raw       [][]byte
	changeIds []string
	bytes     int
}

func (r *changeRecorder) record(tb testing.TB, d Doc) {
	chs := d.(*State).GetChanges()
	data, err := (&pb.Change{Content: chs}).Marshal()
	if err != nil {
		tb.Fatal(err)
	}
	r.raw = append(r.raw, data)
	r.changeIds = append(r.changeIds, fmt.Sprintf("c%d", len(r.changeIds)))
	r.bytes += len(data)
}

func newEmptyTextModel(id string) *model.Block {
	return &model.Block{Id: id, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
}

func newFilledTextModel(id string, i int) *model.Block {
	return &model.Block{Id: id, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: fmt.Sprintf("line %d: some regular typed text content", i)},
	}}
}

func applyEdit(tb testing.TB, d Doc, s *State, rec *changeRecorder) {
	if _, _, err := ApplyState("", s, true); err != nil {
		tb.Fatal(err)
	}
	rec.record(tb, d)
}

func createEmptyBlock(tb testing.TB, d Doc, rec *changeRecorder, id, prev string) {
	s := d.NewState()
	s.Add(simple.New(newEmptyTextModel(id)))
	if err := s.InsertTo(prev, model.Block_Bottom, id); err != nil {
		tb.Fatal(err)
	}
	applyEdit(tb, d, s, rec)
}

var editPatterns = []editPattern{
	{
		name: "settext",
		edit: func(tb testing.TB, d Doc, rec *changeRecorder, i int, prev string) string {
			id := fmt.Sprintf("e%d", i)
			createEmptyBlock(tb, d, rec, id, prev)
			s := d.NewState()
			s.Get(id).Model().GetText().Text = newFilledTextModel(id, i).GetText().Text
			applyEdit(tb, d, s, rec)
			return id
		},
	},
	{
		name: "replace",
		edit: func(tb testing.TB, d Doc, rec *changeRecorder, i int, prev string) string {
			eid := fmt.Sprintf("e%d", i)
			createEmptyBlock(tb, d, rec, eid, prev)
			rid := fmt.Sprintf("r%d", i)
			s := d.NewState()
			s.Add(simple.New(newFilledTextModel(rid, i)))
			if err := s.InsertTo(eid, model.Block_Replace, rid); err != nil {
				tb.Fatal(err)
			}
			applyEdit(tb, d, s, rec)
			return rid
		},
	},
	{
		name: "createWithText",
		edit: func(tb testing.TB, d Doc, rec *changeRecorder, i int, prev string) string {
			id := fmt.Sprintf("t%d", i)
			s := d.NewState()
			s.Add(simple.New(newFilledTextModel(id, i)))
			if err := s.InsertTo(prev, model.Block_Bottom, id); err != nil {
				tb.Fatal(err)
			}
			applyEdit(tb, d, s, rec)
			return id
		},
	},
}

func simulateEditingSession(tb testing.TB, p editPattern, n int) *changeRecorder {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	})
	rec := &changeRecorder{}
	prev := ""
	for i := 0; i < n; i++ {
		prev = p.edit(tb, d, rec, i, prev)
	}
	return rec
}

// replayTree mirrors sourceimpl.BuildState + treeSource.buildState
func replayTree(tb testing.TB, rec *changeRecorder, withParentIndex bool) *State {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*State)
	if withParentIndex {
		d.EnableParentIndex()
	}
	for i, raw := range rec.raw {
		var ch pb.Change
		if err := ch.Unmarshal(raw); err != nil {
			tb.Fatal(err)
		}
		d.SetChangeId(rec.changeIds[i])
		d.ApplyChangeIgnoreErr(ch.Content...)
	}
	d.DisableParentIndex()
	if _, _, err := ApplyStateFastOne("", d); err != nil {
		tb.Fatal(err)
	}
	d.BlocksInit(d)
	if err := d.Normalize(false); err != nil {
		tb.Fatal(err)
	}
	if _, _, err := ApplyState("", d, false); err != nil {
		tb.Fatal(err)
	}
	return d
}

func BenchmarkTreeBuild(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		for _, p := range editPatterns {
			for _, indexed := range []bool{false, true} {
				name := fmt.Sprintf("%s/blocks=%d", p.name, n)
				if indexed {
					name += "/indexed"
				}
				b.Run(name, func(b *testing.B) {
					rec := simulateEditingSession(b, p, n)
					st := replayTree(b, rec, indexed)
					var blocks int
					_ = st.Iterate(func(simple.Block) bool { blocks++; return true })
					b.Logf("changes=%d treeBytes=%d finalBlocks=%d", len(rec.raw), rec.bytes, blocks)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						replayTree(b, rec, indexed)
					}
				})
			}
		}
	}
}
