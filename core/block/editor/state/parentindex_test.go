package state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func piTextModel(id string) *model.Block {
	return &model.Block{Id: id, Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "t-" + id}}}
}

func piCreate(targetId string, pos model.BlockPosition, blocks ...*model.Block) *pb.ChangeContent {
	return &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{BlockCreate: &pb.ChangeBlockCreate{
		TargetId: targetId, Position: pos, Blocks: blocks,
	}}}
}

func piMove(targetId string, pos model.BlockPosition, ids ...string) *pb.ChangeContent {
	return &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockMove{BlockMove: &pb.ChangeBlockMove{
		TargetId: targetId, Position: pos, Ids: ids,
	}}}
}

func piRemove(ids ...string) *pb.ChangeContent {
	return &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockRemove{BlockRemove: &pb.ChangeBlockRemove{Ids: ids}}}
}

// adversarialReplay exercises every replay path that mutates structure,
// including the cases the index must survive: duplicate parents (a created
// block also declared as another block's child), orphaned subtrees (parent
// removed, children swept only at the final apply), re-creation of an existing
// id with a different children list (wholesale swap → stale index entries),
// and side-moves (wrapToRow writes a child slot in place, bypassing helpers).
func adversarialReplay() [][]*pb.ChangeContent {
	return [][]*pb.ChangeContent{
		// flat creates
		{piCreate("root", model.Block_Inner, piTextModel("c1"))},
		{piCreate("c1", model.Block_Bottom, piTextModel("c2"))},
		{piCreate("c2", model.Block_Bottom, piTextModel("c3"))},
		{piCreate("c3", model.Block_Bottom, piTextModel("c4"))},
		{piCreate("c4", model.Block_Bottom, piTextModel("c5"))},
		// duplicate parent: p1 declares c2 as child while c2 stays under root
		{piCreate("c5", model.Block_Bottom, &model.Block{
			Id:          "p1",
			ChildrenIds: []string{"c2"},
			Content:     &model.BlockContentOfText{Text: &model.BlockContentText{Text: "p1"}},
		})},
		// move into the duplicate-parented container
		{piMove("p1", model.Block_Inner, "c3")},
		// remove a middle block
		{piRemove("c4")},
		// re-create an existing id with a different children list (wholesale swap)
		{piCreate("root", model.Block_Inner, &model.Block{
			Id:          "c5",
			ChildrenIds: []string{"c1"},
			Content:     &model.BlockContentOfText{Text: &model.BlockContentText{Text: "c5-recreated"}},
		})},
		// side move → moveFromSide/wrapToRow (in-place child slot write)
		{piCreate("root", model.Block_Inner, piTextModel("s1"))},
		{piCreate("s1", model.Block_Bottom, piTextModel("s2"))},
		{piMove("s1", model.Block_Left, "s2")},
		// side-move a container that already has children (real-account shape,
		// found by the shadow sweep): wrapToRow re-parents it into a fresh
		// column — must not poison the subtree's chain with ambiguity, and
		// later ops below its children must stay consistent
		{piCreate("root", model.Block_Inner, piTextModel("w1"))},
		{piCreate("w1", model.Block_Inner, piTextModel("w2"))},
		{piCreate("root", model.Block_Inner, piTextModel("w3"))},
		{piMove("w1", model.Block_Right, "w3")},
		{piCreate("w2", model.Block_Bottom, piTextModel("w4"))},
		{piRemove("w4")},
		{piCreate("w2", model.Block_Bottom, piTextModel("w5"))},
		// remove a subtree root: children become orphans until the final sweep
		{piRemove("p1")},
		// move something after the orphaning
		{piMove("c5", model.Block_Bottom, "s2")},
		// remove and re-add under a different parent
		{piMove("c1", model.Block_Inner, "c2")},
		// detached-but-alive subtree: a move whose target no longer exists
		// unlinks the subtree but the insert fails (swallowed), leaving it
		// alive in the map yet unreachable. Ops targeting blocks inside it
		// must follow traversal semantics (dropped), not map-aliveness.
		{piCreate("root", model.Block_Inner, piTextModel("dB"))},
		{piCreate("root", model.Block_Inner, piTextModel("dP"))},
		{piCreate("root", model.Block_Inner, piTextModel("dX"))},
		{piMove("dP", model.Block_Inner, "dB")},
		{piMove("missing-target", model.Block_Bottom, "dP")},
		{piMove("dB", model.Block_Bottom, "dX")},
		{piMove("root", model.Block_Inner, "dP")},
	}
}

func collectIds(changeSets [][]*pb.ChangeContent) []string {
	seen := map[string]struct{}{}
	var ids []string
	add := func(id string) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	for _, cs := range changeSets {
		for _, ch := range cs {
			if c := ch.GetBlockCreate(); c != nil {
				add(c.TargetId)
				for _, b := range c.Blocks {
					add(b.Id)
					for _, cid := range b.ChildrenIds {
						add(cid)
					}
				}
			}
			if m := ch.GetBlockMove(); m != nil {
				add(m.TargetId)
				for _, id := range m.Ids {
					add(id)
				}
			}
			if r := ch.GetBlockRemove(); r != nil {
				for _, id := range r.Ids {
					add(id)
				}
			}
		}
	}
	return ids
}

// TestParentIndex_ReplayEquivalence replays the same adversarial change
// sequence with and without the index in lockstep, comparing PickParentOf for
// every known id after every change, and the full final structure.
func TestParentIndex_ReplayEquivalence(t *testing.T) {
	changeSets := adversarialReplay()
	ids := collectIds(changeSets)

	newDoc := func() *State {
		return NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).(*State)
	}
	plain := newDoc()
	indexed := newDoc()
	indexed.EnableParentIndex()

	parentOf := func(s *State, id string) string {
		if p := s.PickParentOf(id); p != nil {
			return p.Model().Id
		}
		return ""
	}

	// each replica must get its own deep copy: changeBlockCreate wraps the
	// change's *model.Block by reference, so sharing change objects between
	// replicas would alias every created block's ChildrenIds and structurally
	// mask divergence
	cloneChangeSet := func(cs []*pb.ChangeContent) []*pb.ChangeContent {
		data, err := (&pb.Change{Content: cs}).Marshal()
		require.NoError(t, err)
		var ch pb.Change
		require.NoError(t, ch.Unmarshal(data))
		return ch.Content
	}

	for i, cs := range changeSets {
		plain.ApplyChangeIgnoreErr(cloneChangeSet(cs)...)
		indexed.ApplyChangeIgnoreErr(cloneChangeSet(cs)...)
		for _, id := range ids {
			require.Equal(t, parentOf(plain, id), parentOf(indexed, id),
				"parent of %q diverged after change #%d", id, i)
		}
	}

	finish := func(s *State) *State {
		s.DisableParentIndex()
		_, _, err := ApplyStateFastOne("", s)
		require.NoError(t, err)
		require.NoError(t, s.Normalize(false))
		_, _, err = ApplyState("", s, false)
		require.NoError(t, err)
		return s
	}
	assert.Equal(t, finish(plain).String(), finish(indexed).String())
}

func TestParentIndex_LayeredStateIsNotIndexed(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*State)
	child := d.NewState()
	child.EnableParentIndex()
	assert.Nil(t, child.parentIdx, "index must never activate on a layered state")

	d.EnableParentIndex()
	require.NotNil(t, d.parentIdx)
	assert.Nil(t, d.NewState().parentIdx, "index must not propagate to child states")
	assert.Nil(t, d.Copy().parentIdx, "index must not propagate to copies")
}

func TestParentIndex_AmbiguousAndStaleFallBack(t *testing.T) {
	// given: c under root, then also declared child of p (duplicate)
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"c"}}),
		"c":    simple.New(piTextModel("c")),
	}).(*State)
	d.EnableParentIndex()

	d.ApplyChangeIgnoreErr(piCreate("c", model.Block_Bottom, &model.Block{
		Id:          "p",
		ChildrenIds: []string{"c"},
		Content:     &model.BlockContentOfText{Text: &model.BlockContentText{}},
	}))

	// then: ambiguous id resolves via traversal (root wins — first in order)
	require.Contains(t, d.parentIdx.ambiguous, "c")
	require.NotNil(t, d.PickParentOf("c"))
	assert.Equal(t, "root", d.PickParentOf("c").Model().Id)

	// and: a stale entry (parent deleted behind the index's back) self-heals
	d2 := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"x"}}),
		"x":    simple.New(&model.Block{Id: "x", ChildrenIds: []string{"y"}}),
		"y":    simple.New(piTextModel("y")),
	}).(*State)
	d2.EnableParentIndex()
	d2.ApplyChangeIgnoreErr(piRemove("x")) // y orphaned; its entry points at deleted x
	assert.Nil(t, d2.PickParentOf("y"), "orphan must resolve to no parent, same as traversal")
	_, found := d2.lookupParent("y")
	assert.False(t, found)
}

func TestParentIndex_UnlinkAllMixedHitsAndMisses(t *testing.T) {
	blocks := map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b", "c"}}),
		"a":    simple.New(piTextModel("a")),
		"b":    simple.New(piTextModel("b")),
		"c":    simple.New(piTextModel("c")),
	}
	d := NewDoc("root", blocks).(*State)
	d.EnableParentIndex()

	// "ghost" has no parent anywhere → index miss → scan path
	d.UnlinkAll([]string{"a", "ghost", "c"})
	assert.Equal(t, []string{"b"}, d.Pick("root").Model().ChildrenIds)
}

func TestParentIndex_ShadowMode(t *testing.T) {
	prev := parentIndexDebug
	parentIndexDebug = true
	defer func() { parentIndexDebug = prev }()

	t.Run("clean replay reports zero mismatches", func(t *testing.T) {
		// given
		d := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).(*State)
		d.EnableParentIndex()

		// when: the full adversarial sequence runs under shadow verification
		for _, cs := range adversarialReplay() {
			d.ApplyChangeIgnoreErr(cs...)
		}

		// then
		assert.Positive(t, d.parentIdx.debugLookups)
		assert.Zero(t, d.parentIdx.debugMismatches)
	})

	t.Run("unflagged duplicate parents are detected and traversal answer wins", func(t *testing.T) {
		// given: "a" listed under both root and b, but not flagged ambiguous
		d := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b"}}),
			"a":    simple.New(piTextModel("a")),
			"b":    simple.New(&model.Block{Id: "b", ChildrenIds: []string{"a"}, Content: piTextModel("b").Content}),
		}).(*State)
		d.EnableParentIndex()
		// the bootstrap flags "a" ambiguous — simulate the machinery missing it
		delete(d.parentIdx.ambiguous, "a")
		d.parentIdx.parents["a"] = "b"

		// when
		p := d.PickParentOf("a")

		// then: traversal answer returned, mismatch counted
		require.NotNil(t, p)
		assert.Equal(t, "root", p.Model().Id)
		assert.Equal(t, 1, d.parentIdx.debugMismatches)
	})

	t.Run("unverifiable chain counts as fallback, not mismatch", func(t *testing.T) {
		// given: an ambiguous ancestor between the child and the root
		d := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"mid"}}),
			"mid":  simple.New(&model.Block{Id: "mid", ChildrenIds: []string{"leaf"}, Content: piTextModel("mid").Content}),
			"leaf": simple.New(piTextModel("leaf")),
		}).(*State)
		d.EnableParentIndex()
		d.parentIdx.ambiguous["mid"] = struct{}{}

		// when
		p := d.PickParentOf("leaf")

		// then
		require.NotNil(t, p)
		assert.Equal(t, "mid", p.Model().Id)
		assert.Zero(t, d.parentIdx.debugMismatches)
		assert.Equal(t, 1, d.parentIdx.debugFallbacks)
	})

	t.Run("index claiming no parent is detected", func(t *testing.T) {
		// given
		d := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a"}}),
			"a":    simple.New(piTextModel("a")),
		}).(*State)
		d.EnableParentIndex()
		// simulate a missed assert
		delete(d.parentIdx.parents, "a")

		// when
		p := d.PickParentOf("a")

		// then
		require.NotNil(t, p)
		assert.Equal(t, "root", p.Model().Id)
		assert.Equal(t, 1, d.parentIdx.debugMismatches)
	})
}

func TestParentIndex_UnlinkForReplayUnderAmbiguousAncestor(t *testing.T) {
	// given: root → mid → leaf, with mid flagged ambiguous — the leaf's chain
	// is unverifiable, but the leaf IS reachable
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"mid"}}),
		"mid":  simple.New(&model.Block{Id: "mid", ChildrenIds: []string{"leaf"}, Content: piTextModel("mid").Content}),
		"leaf": simple.New(piTextModel("leaf")),
	}).(*State)
	d.EnableParentIndex()
	d.parentIdx.ambiguous["mid"] = struct{}{}

	// when: a re-create of "leaf" replays (create acts as move)
	d.ApplyChangeIgnoreErr(piCreate("root", model.Block_Inner, piTextModel("leaf")))

	// then: the old occurrence must be unlinked (unverifiable → full scan),
	// not left duplicated under mid
	assert.Empty(t, d.Pick("mid").Model().ChildrenIds)
	assert.Equal(t, []string{"mid", "leaf"}, d.Pick("root").Model().ChildrenIds)
}

func TestParentIndex_SideMoveDoesNotPoisonAmbiguity(t *testing.T) {
	// given: the real-account shape found by the shadow sweep — a container
	// with children gets side-moved (wrapToRow re-parents it into a column)
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*State)
	d.EnableParentIndex()
	d.ApplyChangeIgnoreErr(piCreate("root", model.Block_Inner, piTextModel("w1")))
	d.ApplyChangeIgnoreErr(piCreate("w1", model.Block_Inner, piTextModel("w2")))
	d.ApplyChangeIgnoreErr(piCreate("root", model.Block_Inner, piTextModel("w3")))

	// when
	d.ApplyChangeIgnoreErr(piMove("w1", model.Block_Right, "w3"))

	// then: the re-parented container is not flagged ambiguous, and lookups
	// under it still resolve through the index
	assert.NotContains(t, d.parentIdx.ambiguous, "w1")
	p, status := d.lookupParentFast("w2")
	require.Equal(t, lookupHit, status)
	assert.Equal(t, "w1", p.Model().Id)
}

func TestParentIndex_BigFlatDocLookups(t *testing.T) {
	// smoke test that hits the fast path at some scale
	children := make([]string, 300)
	blocks := map[string]simple.Block{}
	for i := range children {
		id := fmt.Sprintf("b%d", i)
		children[i] = id
		blocks[id] = simple.New(piTextModel(id))
	}
	blocks["root"] = simple.New(&model.Block{Id: "root", ChildrenIds: children})
	d := NewDoc("root", blocks).(*State)
	d.EnableParentIndex()
	for _, id := range children {
		require.Equal(t, "root", d.PickParentOf(id).Model().Id)
	}
}
