package state

import (
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_ChangesCreate_MoveAdd(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b"}}),
		"a":    simple.New(&model.Block{Id: "a", ChildrenIds: []string{"1", "2", "3", "4", "5"}}),
		"b":    simple.New(&model.Block{Id: "b"}),
		"1":    simple.New(&model.Block{Id: "1"}),
		"2":    simple.New(&model.Block{Id: "2"}),
		"3":    simple.New(&model.Block{Id: "3"}),
		"4":    simple.New(&model.Block{Id: "4"}),
		"5":    simple.New(&model.Block{Id: "5"}),
	})
	s := d.NewState()
	ids := []string{"1", "2", "3", "4", "5"}
	for _, id := range ids {
		require.True(t, s.Unlink(id))
	}
	require.NoError(t, s.InsertTo("b", model.Block_Inner, "1", "2", "4", "5"))
	s.Add(simple.New(&model.Block{Id: "3.1"}))
	require.NoError(t, s.InsertTo("2", model.Block_Bottom, "3.1"))
	_, _, err := ApplyState(s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	require.Len(t, changes, 4)
	assert.Equal(t, []*pb.ChangeContent{
		newMoveChange("b", model.Block_Inner, "1", "2"),
		newCreateChange("2", model.Block_Bottom, &model.Block{Id: "3.1"}),
		newMoveChange("3.1", model.Block_Bottom, "4", "5"),
		newRemoveChange("3"),
	}, changes)
}

func TestState_ChangesCreate_MoveAdd_Wrap(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b"}}),
		"a":    simple.New(&model.Block{Id: "a", ChildrenIds: []string{"1", "2", "3", "4", "5"}}),
		"b":    simple.New(&model.Block{Id: "b"}),
		"1":    simple.New(&model.Block{Id: "1"}),
		"2":    simple.New(&model.Block{Id: "2"}),
		"3":    simple.New(&model.Block{Id: "3"}),
		"4":    simple.New(&model.Block{Id: "4"}),
		"5":    simple.New(&model.Block{Id: "5"}),
	})
	dc := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	s := d.NewState()

	s.Add(simple.New(&model.Block{Id: "div", ChildrenIds: []string{"a", "b"}}))
	s.Get("root").Model().ChildrenIds = []string{"div"}

	_, _, err := ApplyState(s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	_, _, err = ApplyState(s2, true)
	require.NoError(t, err)
	assert.Equal(t, d.(*State).String(), dc.(*State).String())
}

func TestState_ChangesCreate_MoveAdd_Side(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b"}}),
		"a":    simple.New(&model.Block{Id: "a", ChildrenIds: []string{"1", "2", "3", "4", "5"}}),
		"b":    simple.New(&model.Block{Id: "b"}),
		"1":    simple.New(&model.Block{Id: "1"}),
		"2":    simple.New(&model.Block{Id: "2"}),
		"3":    simple.New(&model.Block{Id: "3"}),
		"4":    simple.New(&model.Block{Id: "4"}),
		"5":    simple.New(&model.Block{Id: "5"}),
	})
	dc := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	s := d.NewState()

	s.Unlink("4")
	s.Unlink("5")
	s.InsertTo("1", model.Block_Left, "4", "5")

	_, _, err := ApplyState(s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	_, _, err = ApplyState(s2, true)
	require.NoError(t, err)
	assert.Equal(t, d.(*State).String(), dc.(*State).String())
}

func TestStateNormalizeMerge(t *testing.T) {
	d := NewDoc("root", nil).(*State)
	s := d.NewState()
	s.Add(simple.New(&model.Block{Id: "root"}))
	s.Add(simple.New(&model.Block{Id: "parent"}))
	s.InsertTo("root", model.Block_Inner, "parent")
	var ids []string
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("common%d", i)
		s.Add(simple.New(&model.Block{Id: id}))
		if i == 0 {
			s.InsertTo("parent", model.Block_Inner, id)
		} else {
			s.InsertTo(fmt.Sprintf("common%d", i-1), model.Block_Bottom, id)
		}
		ids = append(ids, id)
	}
	_, _, err := ApplyState(s, true)
	require.NoError(t, err)

	docA := d.Copy()
	stateA := docA.NewState()
	stateA.Add(simple.New(&model.Block{Id: "a1"}))
	stateA.Add(simple.New(&model.Block{Id: "a2"}))
	stateA.InsertTo("common39", model.Block_Bottom, "a1", "a2")
	_, _, err = ApplyState(stateA, true)
	require.NoError(t, err)
	//t.Log(docA.String())
	changesA := stateA.GetChanges()

	docB := d.Copy()
	stateB := docB.NewState()
	stateB.Add(simple.New(&model.Block{Id: "b1"}))
	stateB.InsertTo("common39", model.Block_Bottom, "b1")
	_, _, err = ApplyState(stateB, true)
	require.NoError(t, err)
	//t.Log(docB.String())
	changesB := stateB.GetChanges()

	s = d.NewState()
	require.NoError(t, s.ApplyChange(changesB...))
	_, _, err = ApplyStateFastOne(s)
	require.NoError(t, err)
	s = d.NewState()
	require.NoError(t, s.ApplyChange(changesA...))
	_, _, err = ApplyState(s, true)
	require.NoError(t, err)

	s = d.NewState()
	assert.True(t, CleanupLayouts(s) > 0)
	ids = append(ids, "a1", "a2", "b1")
	assert.Equal(t, ids, s.Pick("parent").Model().ChildrenIds)
	//t.Log(s.String())
}

func newMoveChange(targetId string, pos model.BlockPosition, ids ...string) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfBlockMove{
			BlockMove: &pb.ChangeBlockMove{
				TargetId: targetId,
				Position: pos,
				Ids:      ids,
			},
		},
	}
}

func newCreateChange(targetId string, pos model.BlockPosition, b ...*model.Block) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfBlockCreate{
			BlockCreate: &pb.ChangeBlockCreate{
				TargetId: targetId,
				Position: pos,
				Blocks:   b,
			},
		},
	}
}

func newRemoveChange(ids ...string) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfBlockRemove{
			BlockRemove: &pb.ChangeBlockRemove{
				Ids: ids,
			},
		},
	}
}
