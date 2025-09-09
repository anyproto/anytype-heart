package state

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	. "github.com/anyproto/anytype-heart/tests/blockbuilder"
)

func makeStoreWithTwoKeysAndValue(first, second, value string) *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			first: {Kind: &types.Value_StructValue{
				StructValue: &types.Struct{Fields: map[string]*types.Value{second: pbtypes.String(value)}},
			}}}}
}

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
	_, _, err := ApplyState("", s, true)
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

func TestState_ChangesCreate_Collection_Set(t *testing.T) {
	d := NewDoc("root", nil)
	s := d.NewState()
	s.SetInStore([]string{"coll1", "key1"}, pbtypes.String("1"))
	_, _, err := ApplyState("", s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	require.Len(t, changes, 1)
	assert.Equal(t, (&pb.ChangeContent{
		Value: &pb.ChangeContentValueOfStoreKeySet{
			StoreKeySet: &pb.ChangeStoreKeySet{
				Path:  []string{"coll1", "key1"},
				Value: pbtypes.String("1"),
			},
		},
	}).String(), changes[0].String())
}

func TestState_ChangesCreate_Collection_Unset(t *testing.T) {
	d := NewDoc("root", nil)
	d.(*State).store = makeStoreWithTwoKeysAndValue("coll1", "key1", "1")
	s := d.NewState()
	s.RemoveFromStore([]string{"coll1", "key1"})
	_, _, err := ApplyState("", s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	require.Len(t, changes, 1)
	assert.Equal(t, (&pb.ChangeContent{
		Value: &pb.ChangeContentValueOfStoreKeyUnset{
			StoreKeyUnset: &pb.ChangeStoreKeyUnset{
				Path: []string{"coll1", "key1"},
			},
		},
	}).String(), changes[0].String())
}

func TestState_ChangesCreate_StoreSlice(t *testing.T) {
	const key = "uniqueKeyInternal"
	for _, tc := range []struct {
		before  []string
		after   []string
		changes []pb.IsChangeContentValue
	}{
		{
			before: nil,
			after:  []string{"1", "2", "3"},
			changes: []pb.IsChangeContentValue{
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfAdd{
							Add: &pb.ChangeStoreSliceUpdateAdd{
								AfterId: "",
								Ids:     []string{"1", "2", "3"},
							},
						},
					},
				},
			},
		},
		{
			before: []string{"1", "2", "3"},
			after:  []string{"2"},
			changes: []pb.IsChangeContentValue{
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfRemove{
							Remove: &pb.ChangeStoreSliceUpdateRemove{
								Ids: []string{"1", "3"},
							},
						},
					},
				},
			},
		},
		{
			before: []string{"1", "2", "3"},
			after:  []string{"1", "3", "2"},
			changes: []pb.IsChangeContentValue{
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfMove{
							Move: &pb.ChangeStoreSliceUpdateMove{
								AfterId: "1",
								Ids:     []string{"3"},
							},
						},
					},
				},
			},
		},
		{
			before: []string{"1", "2", "3"},
			after:  []string{"3", "2", "4"},
			changes: []pb.IsChangeContentValue{
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfMove{
							Move: &pb.ChangeStoreSliceUpdateMove{
								AfterId: "",
								Ids:     []string{"3"},
							},
						},
					},
				},
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfAdd{
							Add: &pb.ChangeStoreSliceUpdateAdd{
								AfterId: "2",
								Ids:     []string{"4"},
							},
						},
					},
				},
				&pb.ChangeContentValueOfStoreSliceUpdate{
					StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
						Key: key,
						Operation: &pb.ChangeStoreSliceUpdateOperationOfRemove{
							Remove: &pb.ChangeStoreSliceUpdateRemove{
								Ids: []string{"1"},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("%s -> %s", tc.before, tc.after), func(t *testing.T) {
			doc := NewDoc("root", nil)
			doc.(*State).UpdateStoreSlice(key, tc.before)

			newState := doc.NewState()
			newState.UpdateStoreSlice(key, tc.after)

			_, _, err := ApplyState("", newState, false)
			require.NoError(t, err)

			got := doc.(*State).GetChanges()
			gotContent := make([]pb.IsChangeContentValue, 0, len(got))
			for _, c := range got {
				gotContent = append(gotContent, c.Value)
			}

			assert.Equal(t, tc.changes, gotContent)
		})
	}
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
	dc, err := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	require.NoError(t, err)
	s := d.NewState()

	s.Add(simple.New(&model.Block{Id: "div", ChildrenIds: []string{"a", "b"}}))
	s.Get("root").Model().ChildrenIds = []string{"div"}

	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	_, _, err = ApplyState("", s2, true)
	require.NoError(t, err)
	assert.Equal(t, d.(*State).String(), dc.String())
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
	dc, err := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	require.NoError(t, err)
	s := d.NewState()

	s.Unlink("4")
	s.Unlink("5")
	s.InsertTo("1", model.Block_Left, "4", "5")

	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	_, _, err = ApplyState("", s2, true)
	require.NoError(t, err)
	assert.Equal(t, d.(*State).String(), dc.String())
}

func TestState_ChangesCreate_MoveAdd_Side_NewBlock(t *testing.T) {
	makeState := func() *State {
		return buildStateFromAST(t,
			Root(
				ID("root"),
				Children(
					Text("1", ID("1")),
					Text("2", ID("2")),
					Text("3", ID("3")),
				),
			),
		)
	}
	assertApplyingChanges := func(t *testing.T, state *State, wantState *State, originalState *State) {
		AssertTreesEqual(t, wantState.Blocks(), state.Blocks())

		_, _, err := ApplyState("", state, true)
		require.NoError(t, err)
		changes := state.GetChanges()
		err = originalState.ApplyChange(changes...)
		require.NoError(t, err)

		AssertTreesEqual(t, wantState.Blocks(), originalState.Blocks())
	}

	t.Run("only one new block", func(t *testing.T) {
		state := makeState()
		originalState := state.Copy()

		wantState := buildStateFromAST(
			t,
			Root(
				ID("root"),
				Children(
					Text("1", ID("1")),
					Row(Children(
						Column(Children(
							Text("2", ID("2")))),
						Column(Children(
							Text("4", ID("4")))))),
					Text("3", ID("3")))))

		newBlock := simple.New(Text("4", ID("4")).Block())
		state.Add(newBlock)
		err := state.InsertTo("2", model.Block_Right, "4")
		require.NoError(t, err)

		assertApplyingChanges(t, state, wantState, originalState)
	})

	t.Run("mixed old and new blocks", func(t *testing.T) {
		state := makeState()
		originalState := state.Copy()

		wantState := buildStateFromAST(
			t,
			Root(
				ID("root"),
				Children(
					Text("1", ID("1")),
					Row(Children(
						Column(Children(
							Text("2", ID("2")))),
						Column(Children(
							Text("4", ID("4")),
							Text("3", ID("3")))))))))

		newBlock := simple.New(Text("4", ID("4")).Block())
		state.Unlink("3")
		state.Add(newBlock)
		err := state.InsertTo("2", model.Block_Right, "4", "3")
		require.NoError(t, err)

		assertApplyingChanges(t, state, wantState, originalState)
	})
}

func TestState_SetParent(t *testing.T) {
	orig := NewDoc("root", nil).(*State)
	orig.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"header"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	orig.Add(simple.New(&model.Block{Id: "header"}))
	orig.SetObjectTypeKey("orig")
	orig.AddRelationLinks(&model.RelationLink{Format: model.RelationFormat_longtext, Key: "one"})
	st := orig.Copy()

	newState := NewDoc("root", nil).(*State)
	newState.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"child", "header"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	newState.Add(simple.New(&model.Block{Id: "child"}))
	newState.Add(simple.New(&model.Block{Id: "header"}))
	newState.SetObjectTypeKeys([]domain.TypeKey{"newOT1", "newOT2"})
	newState.AddRelationLinks(&model.RelationLink{Format: model.RelationFormat_longtext, Key: "newOne"})
	newState.AddRelationLinks(&model.RelationLink{Format: model.RelationFormat_longtext, Key: "newTwo"})

	ns := newState.Copy()

	ns.SetRootId(st.RootId())
	ns.SetParent(st)
	_, _, err := ApplyState("", ns, false)
	require.NoError(t, err)

	st2 := orig.Copy()
	require.NoError(t, st2.ApplyChange(st.GetChanges()...))

	assert.Equal(t, st.StringDebug(), st2.StringDebug())
}

func TestStateNormalizeMerge(t *testing.T) {
	d := NewDoc("root", nil).(*State)
	s := d.NewState()
	s.Add(simple.New(&model.Block{Id: "root"}))
	s.Add(simple.New(&model.Block{Id: "parent"}))
	s.InsertTo("root", model.Block_Inner, "parent")
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("common%d", i)
		s.Add(simple.New(&model.Block{Id: id}))
		if i == 0 {
			s.InsertTo("parent", model.Block_Inner, id)
		} else {
			s.InsertTo(fmt.Sprintf("common%d", i-1), model.Block_Bottom, id)
		}
	}
	_, _, err := ApplyState("", s, true)
	require.NoError(t, err)

	t.Run("parallel normalize", func(t *testing.T) {
		d := d.Copy()
		ids := d.Pick("parent").Copy().Model().ChildrenIds
		docA := d.Copy()
		stateA := docA.NewState()
		stateA.Add(simple.New(&model.Block{Id: "a1"}))
		stateA.Add(simple.New(&model.Block{Id: "a2"}))
		stateA.InsertTo("common39", model.Block_Bottom, "a1", "a2")
		_, _, err = ApplyState("", stateA, true)
		require.NoError(t, err)
		// t.Log(docA.String())
		changesA := stateA.GetChanges()

		docB := d.Copy()
		stateB := docB.NewState()
		stateB.Add(simple.New(&model.Block{Id: "b1"}))
		stateB.InsertTo("common39", model.Block_Bottom, "b1")
		_, _, err = ApplyState("", stateB, true)
		require.NoError(t, err)
		// t.Log(docB.String())
		changesB := stateB.GetChanges()

		s = d.NewState()
		require.NoError(t, s.ApplyChange(changesB...))
		_, _, err = ApplyStateFastOne("", s)
		require.NoError(t, err)
		s = d.NewState()
		require.NoError(t, s.ApplyChange(changesA...))
		_, _, err = ApplyState("", s, true)
		require.NoError(t, err)

		s = d.NewState()
		assert.True(t, CleanupLayouts(s) > 0)
		ids = append(ids, "a1", "a2", "b1")
		assert.Equal(t, ids, s.Pick("parent").Model().ChildrenIds)
		// t.Log(s.String())
	})
	t.Run("rebalance", func(t *testing.T) {
		d := d.Copy()
		ids := d.Pick("parent").Copy().Model().ChildrenIds
		s := d.NewState()
		s.Add(simple.New(&model.Block{Id: "common40"}))
		s.InsertTo("common39", model.Block_Bottom, "common40")
		ids = append(ids, "common40")
		_, _, err := ApplyState("", s, true)
		require.NoError(t, err)

		aIds := []string{}
		docA := d.Copy()
		stateA := docA.NewState()
		for i := 0; i < 21; i++ {
			id := fmt.Sprintf("a%d", i)
			aIds = append(aIds, id)
			stateA.Add(simple.New(&model.Block{Id: id}))
		}
		stateA.InsertTo("common0", model.Block_Top, aIds...)
		_, _, err = ApplyState("", stateA, true)
		require.NoError(t, err)
		changesA := stateA.GetChanges()

		bIds := []string{}
		docB := d.Copy()
		stateB := docB.NewState()
		for i := 0; i < 11; i++ {
			id := fmt.Sprintf("b%d", i)
			bIds = append(bIds, id)
			stateB.Add(simple.New(&model.Block{Id: id}))
		}
		stateB.InsertTo("common40", model.Block_Bottom, bIds...)
		_, _, err = ApplyState("", stateB, true)
		require.NoError(t, err)
		changesB := stateB.GetChanges()

		s = d.NewState()
		require.NoError(t, s.ApplyChange(changesB...))
		_, _, err = ApplyStateFastOne("", s)
		require.NoError(t, err)
		s = d.NewState()
		require.NoError(t, s.ApplyChange(changesA...))
		_, _, err = ApplyState("", s, true)
		require.NoError(t, err)

		s = d.NewState()
		assert.True(t, CleanupLayouts(s) > 0)
		ids = append(aIds, ids...)
		ids = append(ids, bIds...)
		assert.Equal(t, ids, s.Pick("parent").Model().ChildrenIds)
	})
}

func TestState_ChangeDataviewOrder(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"dv"}}),
		"dv": simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{Id: "1"},
					{Id: "2"},
					{Id: "3"},
				},
			},
		}}),
	})
	dc, err := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	require.NoError(t, err)
	s := d.NewState()
	s.Get("dv").(dataview.Block).SetViewOrder([]string{"3", "1", "2"})

	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)
	changes := d.(*State).GetChanges()
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	_, _, err = ApplyState("", s2, true)
	require.NoError(t, err)
	assert.Equal(t, d.(*State).Pick("dv").Model().String(), dc.Pick("dv").Model().String())
}

func TestState_ChangeDataviewUnlink(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{}}),
	})
	dc, err := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	require.NoError(t, err)
	s := d.NewState()
	s.Add(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Views: []*model.BlockContentDataviewView{
				{Id: "1"},
			},
		},
	}}))
	s.InsertTo("root", model.Block_Inner, "dv")
	_, _, err = ApplyState("", s, true)
	changes := d.(*State).GetChanges()
	s = d.NewState()
	s.Unlink("dv")
	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)
	changes = append(changes, d.(*State).GetChanges()...)

	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	require.Nil(t, s2.Get("dv"))
	require.Nil(t, s2.Pick("dv"))

	_, _, err = ApplyState("", s2, true)
	require.NoError(t, err)
	require.Nil(t, dc.Get("dv"))
	require.Nil(t, dc.Pick("dv"))
}

func TestState_ChangeDataviewRemoveAdd(t *testing.T) {
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{}}),
	})
	dc, err := NewDocFromSnapshot("root", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: d.Blocks(),
		},
	})
	require.NoError(t, err)
	s := d.NewState()
	s.Add(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Views: []*model.BlockContentDataviewView{
				{Id: "1"},
			},
		},
	}}))
	s.InsertTo("root", model.Block_Inner, "dv")
	_, _, err = ApplyState("", s, true)
	changes := d.(*State).GetChanges()
	s = d.NewState()
	s.Unlink("dv")
	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)
	changes = append(changes, d.(*State).GetChanges()...)
	s = d.NewState()
	s.Add(simple.New(&model.Block{Id: "dv", Content: &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Views: []*model.BlockContentDataviewView{
				{Id: "2"},
			},
		},
	}}))
	s.InsertTo("root", model.Block_Inner, "dv")
	_, _, err = ApplyState("", s, true)
	require.NoError(t, err)

	changes = append(changes, d.(*State).GetChanges()...)
	s2 := dc.NewState()
	require.NoError(t, s2.ApplyChange(changes...))
	require.NotNil(t, s2.Get("dv"))
	require.Len(t, s2.Get("dv").Model().GetDataview().Views, 1)
	require.Equal(t, "2", s2.Get("dv").Model().GetDataview().Views[0].Id)

	_, _, err = ApplyState("", s2, true)
	require.NoError(t, err)
	require.NotNil(t, dc.Get("dv"))
	require.Len(t, dc.Get("dv").Model().GetDataview().Views, 1)
	require.Equal(t, "2", dc.Get("dv").Model().GetDataview().Views[0].Id)
}

func TestState_ChangeDataviewRemoveMove(t *testing.T) {
	d1 := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"s"}}),
		"s":    simple.New(&model.Block{Id: "s", ChildrenIds: []string{}}),
	})

	changes := []*pb.ChangeContent{
		{
			Value: &pb.ChangeContentValueOfBlockCreate{BlockCreate: &pb.ChangeBlockCreate{TargetId: "root", Position: model.Block_Inner, Blocks: []*model.Block{{Id: "b"}}}},
		},
		{
			Value: &pb.ChangeContentValueOfBlockRemove{BlockRemove: &pb.ChangeBlockRemove{Ids: []string{"b"}}},
		},
		{
			Value: &pb.ChangeContentValueOfBlockMove{BlockMove: &pb.ChangeBlockMove{TargetId: "s", Position: model.Block_Inner, Ids: []string{"b"}}},
		},
	}

	s := d1.NewState()
	require.NoError(t, s.ApplyChange(changes...))
	ApplyState("", s, true)
	require.Nil(t, d1.(*State).blocks["b"])
	require.NotContains(t, d1.(*State).Pick("s").Model().ChildrenIds, "b")
	require.NotContains(t, d1.(*State).Pick("root").Model().ChildrenIds, "b")

}

func Test_ApplyChange(t *testing.T) {
	t.Run("object types remove", func(t *testing.T) {
		root := NewDoc("root", nil)
		root.(*State).SetObjectTypeKeys([]domain.TypeKey{"one", "two"})
		s := root.NewState()
		require.NoError(t, s.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfObjectTypeRemove{
				ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
					Url: "one",
				},
			},
		}))
		assert.Equal(t, []domain.TypeKey{"two"}, s.ObjectTypeKeys())

		require.NoError(t, s.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfObjectTypeRemove{
				ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
					Url: "two",
				},
			},
		}))
		assert.Len(t, s.ObjectTypeKeys(), 0)
	})

	t.Run("collection set/unset uniqueKeyInternal", func(t *testing.T) {
		root := NewDoc("root", nil)

		s := root.NewState()
		require.NoError(t, s.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeySet{
				StoreKeySet: &pb.ChangeStoreKeySet{
					Path:  []string{"coll1", "key1"},
					Value: pbtypes.String("1"),
				},
			},
		}))
		assert.Equal(t, makeStoreWithTwoKeysAndValue("coll1", "key1", "1"), s.Store())

		require.NoError(t, s.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeyUnset{
				StoreKeyUnset: &pb.ChangeStoreKeyUnset{
					Path: []string{"coll1", "key1"},
				},
			},
		}))
		require.NoError(t, s.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeyUnset{
				StoreKeyUnset: &pb.ChangeStoreKeyUnset{
					Path: []string{"coll1", "key1"},
				},
			},
		}))
		assert.Equal(t, &types.Struct{Fields: map[string]*types.Value{}}, s.Store())
	})
}

func TestRelationChanges(t *testing.T) {
	a := NewDoc("root", nil).(*State)
	a.relationLinks = []*model.RelationLink{{Key: "1"}, {Key: "2"}, {Key: "3"}}
	ac := a.Copy()
	b := a.NewState()
	b.relationLinks = []*model.RelationLink{{Key: "3"}, {Key: "4"}, {Key: "5"}}
	_, _, err := ApplyState("", b, false)
	require.NoError(t, err)
	chs := a.GetChanges()
	require.NoError(t, ac.ApplyChange(chs...))
	require.Equal(t, a.relationLinks, ac.relationLinks)
}

func TestLocalRelationChanges(t *testing.T) {
	t.Run("local relation added", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		a.relationLinks = []*model.RelationLink{}
		b := a.NewState()
		b.relationLinks = []*model.RelationLink{{Key: bundle.RelationKeySyncStatus.String(), Format: model.RelationFormat_number}}

		// when
		_, _, err := ApplyState("", b, false)
		require.NoError(t, err)
		chs := a.GetChanges()

		// then
		require.Len(t, chs, 0)
	})
	t.Run("local relation removed", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		a.relationLinks = []*model.RelationLink{{Key: bundle.RelationKeySyncStatus.String(), Format: model.RelationFormat_number}}
		b := a.NewState()
		b.relationLinks = []*model.RelationLink{}

		// when
		_, _, err := ApplyState("", b, false)
		require.NoError(t, err)
		chs := a.GetChanges()

		// then
		require.Len(t, chs, 0)
	})
	t.Run("derived relation added", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		a.relationLinks = []*model.RelationLink{}
		b := a.NewState()
		b.relationLinks = []*model.RelationLink{{Key: bundle.RelationKeySpaceId.String(), Format: model.RelationFormat_longtext}}

		// when
		_, _, err := ApplyState("", b, false)
		require.NoError(t, err)
		chs := a.GetChanges()

		// then
		require.Len(t, chs, 0)
	})
	t.Run("derived relation removed", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		a.relationLinks = []*model.RelationLink{{Key: bundle.RelationKeySpaceId.String(), Format: model.RelationFormat_longtext}}
		b := a.NewState()
		b.relationLinks = []*model.RelationLink{}

		// when
		_, _, err := ApplyState("", b, false)
		require.NoError(t, err)
		chs := a.GetChanges()

		// then
		require.Len(t, chs, 0)
	})
}

func TestRootBlockChanges(t *testing.T) {
	a := NewDoc("root", nil).(*State)
	s := a.NewState()
	s.Add(simple.New(&model.Block{Id: "root", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	s.Add(simple.New(&model.Block{Id: "new"}))
	require.NoError(t, s.InsertTo("root", model.Block_Inner, "new"))

	_, _, err := ApplyState("", s, true)
	require.NoError(t, err)
	changes := s.GetChanges()

	b := a.NewState()
	require.NoError(t, b.ApplyChange(changes...))

	assert.Equal(t, a.String(), b.String())

	be := NewDoc("root", nil).(*State)

	require.NoError(t, be.ApplyChange(changes...))
	assert.Equal(t, a.String(), be.String())
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

// copy if testutil.BuildStateFromAST because of cyclic import
func buildStateFromAST(t *testing.T, root *Block) *State {
	st, err := NewDocFromSnapshot("", &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks: root.Build(),
		},
	})
	require.NoError(t, err)
	ApplyState("", st, true)
	return st.NewState()
}

func Test_migrateObjectTypeIDToKey(t *testing.T) {
	type args struct {
		old string
	}
	tests := []struct {
		name         string
		args         args
		wantNew      string
		wantMigrated bool
	}{
		{
			name: "type url to key",
			args: args{
				old: "ot-task",
			},
			wantNew:      "task",
			wantMigrated: true,
		},
		{
			name: "type bundled url to key",
			args: args{
				old: "_ottask",
			},
			wantNew:      "task",
			wantMigrated: true,
		},
		{
			name: "no migration",
			args: args{
				old: "task",
			},
			wantNew:      "task",
			wantMigrated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNew := migrateObjectTypeIDToKey(tt.args.old)
			assert.Equalf(t, tt.wantNew, gotNew, "migrateObjectTypeIDToKey(%v)", tt.args.old)
		})
	}
}

func TestRootDeviceChanges(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		s := a.NewState()

		// when
		_, _, err := ApplyState("", s, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, s.GetChanges(), 0)
	})
	t.Run("add new device", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		s := a.NewState()

		device := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}
		s.AddDevice(device)

		// when
		_, _, err := ApplyState("", s, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, s.GetChanges(), 1)
		assert.Equal(t, device, s.GetChanges()[0].GetDeviceAdd().GetDevice())
	})
	t.Run("update device", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		device := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}
		a.AddDevice(device)

		s := a.NewState()
		s.SetDeviceName("id", "test1")
		// when
		_, _, err := ApplyState("", s, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, s.GetChanges(), 1)
		assert.Equal(t, "test1", s.GetChanges()[0].GetDeviceUpdate().GetName())
	})
	t.Run("add device - parent nil", func(t *testing.T) {
		// given
		a := NewDoc("root", nil).(*State)
		s := a.NewState()

		device := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}
		s.AddDevice(device)
		s.parent = nil
		// when
		_, _, err := ApplyState("", s, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, s.GetChanges(), 1)
		assert.Equal(t, device, s.GetChanges()[0].GetDeviceAdd().GetDevice())
	})
}

func TestTableChanges(t *testing.T) {
	t.Run("change row header", func(t *testing.T) {
		contRow := &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		}
		contColumn := &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		}

		r := NewDoc("root", nil).(*State)
		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1", "t1"}}))
		s.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow}))
		s.Add(simple.New(&model.Block{Id: "c1", Content: contColumn}))
		s.Add(simple.New(&model.Block{Id: "c2", Content: contColumn}))

		s.Add(simple.New(&model.Block{Id: "t1", ChildrenIds: []string{"tableRows", "tableColumns"}, Content: &model.BlockContentOfTable{
			Table: &model.BlockContentTable{},
		}}))
		s.Add(simple.New(&model.Block{Id: "tableRows", ChildrenIds: []string{"tableRow1"}, Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows,
			},
		}}))
		s.Add(simple.New(&model.Block{Id: "tableRow1", Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: false}}}))

		s.Add(simple.New(&model.Block{Id: "tableColumns", Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns,
			},
		}}))

		msgs, _, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 1)

		s = s.NewState()
		rows := s.Get("tableRow1")
		require.NotNil(t, rows)
		rows.Model().GetTableRow().IsHeader = true
		msgs, _, err = ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 1)

	})
}
