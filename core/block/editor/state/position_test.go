package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	. "github.com/anyproto/anytype-heart/tests/blockbuilder"
)

func TestState_InsertTo(t *testing.T) {
	t.Run("default insert", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("", 0, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second"}, r.Pick("root").Model().ChildrenIds)
		assert.True(t, r.Exists("first"))
		assert.True(t, r.Exists("second"))
	})
	t.Run("bottom", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Bottom, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"target", "first", "second"}, r.Pick("root").Model().ChildrenIds)
	})
	t.Run("top", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Top, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second", "target"}, r.Pick("root").Model().ChildrenIds)
	})
	t.Run("inner", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Inner, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"target"}, r.Pick("root").Model().ChildrenIds)
		assert.Equal(t, []string{"first", "second"}, r.Pick("target").Model().ChildrenIds)
	})
	t.Run("replace", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target", ChildrenIds: []string{"child"}}))
		r.Add(simple.New(&model.Block{Id: "child", Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first", Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Replace, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
		assert.Len(t, hist.Remove, 1)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second"}, r.Pick("root").Model().ChildrenIds)
		assert.Equal(t, []string{"child"}, r.Pick("first").Model().ChildrenIds)

		s = r.NewState()
		require.NoError(t, r.InsertTo("first", model.Block_Replace, "child"))
		_, _, err = ApplyState("", s, true)
		require.NoError(t, err)
		assert.Equal(t, []string{"child", "second"}, r.Pick("root").Model().ChildrenIds)
	})

	t.Run("do not replace target with children when non-text block inserted", func(t *testing.T) {
		// given
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target", ChildrenIds: []string{"child"}}))
		r.Add(simple.New(&model.Block{Id: "child", ChildrenIds: []string{}}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "link", Content: &model.BlockContentOfLink{}}))

		// when
		err := s.InsertTo("target", model.Block_Replace, "link")
		_, _, err2 := ApplyState("", s, true)

		// then
		assert.NoError(t, err)
		assert.NoError(t, err2)
		assert.Len(t, s.Pick("root").Model().ChildrenIds, 2)
	})

	t.Run("replace target without children when non-text block inserted", func(t *testing.T) {
		// given
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target", ChildrenIds: []string{}}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "link", Content: &model.BlockContentOfLink{}}))

		// when
		err := s.InsertTo("target", model.Block_Replace, "link")
		_, _, err2 := ApplyState("", s, true)

		// then
		assert.NoError(t, err)
		assert.NoError(t, err2)
		assert.Equal(t, s.Pick("root").Model().ChildrenIds, []string{"link"})
	})

	t.Run("innerFirst", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target", ChildrenIds: []string{"e1", "e2"}}))
		r.Add(simple.New(&model.Block{Id: "e1"}))
		r.Add(simple.New(&model.Block{Id: "e2"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		require.NoError(t, s.InsertTo("target", model.Block_InnerFirst, "first", "second"))
		assert.Equal(t, []string{"first", "second", "e1", "e2"}, s.Pick("target").Model().ChildrenIds)
	})

	moveFromSide := func(t *testing.T, pos model.BlockPosition) (r *State, c1, c2 simple.Block) {
		r = NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", pos, "first", "second")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.NotEmpty(t, msgs)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 5) // 2 new + 2 columns + 1 row
		assert.Len(t, hist.Change, 1)

		require.Len(t, r.Pick("root").Model().ChildrenIds, 1)
		rowId := r.Pick("root").Model().ChildrenIds[0]
		row := r.Pick(rowId)
		assert.Equal(t, model.BlockContentLayout_Row, row.Model().GetLayout().Style)
		require.Len(t, row.Model().ChildrenIds, 2)
		c1 = r.Pick(row.Model().ChildrenIds[0])
		c2 = r.Pick(row.Model().ChildrenIds[1])
		return
	}

	t.Run("left to generic", func(t *testing.T) {
		_, c1, c2 := moveFromSide(t, model.Block_Left)
		assert.Len(t, c1.Model().ChildrenIds, 2)
		assert.Len(t, c2.Model().ChildrenIds, 1)
	})
	t.Run("right to generic", func(t *testing.T) {
		_, c1, c2 := moveFromSide(t, model.Block_Right)
		assert.Len(t, c1.Model().ChildrenIds, 1)
		assert.Len(t, c2.Model().ChildrenIds, 2)
	})
	t.Run("left to column", func(t *testing.T) {
		r, c1, _ := moveFromSide(t, model.Block_Left)
		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c1.Model().Id, model.Block_Left, "third")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})
	t.Run("left to column 2", func(t *testing.T) {
		r, _, c2 := moveFromSide(t, model.Block_Left)
		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c2.Model().Id, model.Block_Left, "third")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})
	t.Run("left to column 2", func(t *testing.T) {
		r, _, c2 := moveFromSide(t, model.Block_Left)
		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c2.Model().Id, model.Block_Right, "third")

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})

	t.Run("cycle ref error", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root"}))

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "1", ChildrenIds: []string{"2"}}))
		s.Add(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"1"}}))
		s.Get("root").Model().ChildrenIds = []string{"1"}

		_, _, err := ApplyState("", s, true)
		assert.Error(t, err)
	})

	t.Run("determinate layout ids", func(t *testing.T) {
		r, c1, c2 := moveFromSide(t, model.Block_Left)
		row := r.PickParentOf(c1.Model().Id)
		c1Id := c1.Model().Id
		c2Id := c2.Model().Id
		rowId := row.Model().Id

		assert.NotEqual(t, c1Id, c2Id)
		assert.NotEqual(t, c1Id, rowId)

		r, c1, c2 = moveFromSide(t, model.Block_Left)
		row = r.PickParentOf(c1.Model().Id)

		assert.Equal(t, rowId, row.Model().Id)
		assert.Equal(t, c1Id, c1.Model().Id)
		assert.Equal(t, c2Id, c2.Model().Id)

		r, c1, c2 = moveFromSide(t, model.Block_Right)
		row = r.PickParentOf(c1.Model().Id)
		assert.NotEqual(t, rowId, row.Model().Id)
		assert.NotEqual(t, c1Id, c1.Model().Id)
		assert.NotEqual(t, c2Id, c2.Model().Id)
	})
}

func Test_addChangesForSideMoving(t *testing.T) {
	makeState := func() *State {
		return buildStateFromAST(
			t,
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

	t.Run("move one existing block", func(t *testing.T) {
		state := makeState()

		state.addChangesForSideMoving("1", model.Block_Left, "2")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "1",
						Position: model.Block_Left,
						Ids:      []string{"2"},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})

	t.Run("move multiple existing blocks", func(t *testing.T) {
		state := makeState()

		state.addChangesForSideMoving("1", model.Block_Left, "2", "3")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "1",
						Position: model.Block_Left,
						Ids:      []string{"2", "3"},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})

	t.Run("try to move missing block", func(t *testing.T) {
		state := makeState()

		state.addChangesForSideMoving("1", model.Block_Left, "4")

		var want []*pb.ChangeContent
		assert.Equal(t, want, state.GetChanges())
	})

	t.Run("add one new block", func(t *testing.T) {
		state := makeState()

		newBlock := simple.New(Text("4", ID("4")).Block())
		state.Add(newBlock)
		state.addChangesForSideMoving("1", model.Block_Left, "4")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "1",
						Position: model.Block_Left,
						Blocks: []*model.Block{
							newBlock.Model(),
						},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})
	t.Run("add multiple new blocks", func(t *testing.T) {
		state := makeState()

		newBlock1 := simple.New(Text("4", ID("4")).Block())
		newBlock2 := simple.New(Text("5", ID("5")).Block())
		state.Add(newBlock1)
		state.Add(newBlock2)
		state.addChangesForSideMoving("1", model.Block_Left, "4", "5")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "1",
						Position: model.Block_Left,
						Blocks: []*model.Block{
							newBlock1.Model(),
							newBlock2.Model(),
						},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})
	t.Run("add one new block and move multiple existing blocks", func(t *testing.T) {
		state := makeState()

		newBlock := simple.New(Text("4", ID("4")).Block())
		state.Add(newBlock)
		state.addChangesForSideMoving("1", model.Block_Left, "4", "2", "3")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "1",
						Position: model.Block_Left,
						Blocks: []*model.Block{
							newBlock.Model(),
						},
					},
				},
			},
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "4",
						Position: model.Block_Bottom,
						Ids:      []string{"2", "3"},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})
	t.Run("move multiple existing blocks and add multiple new blocks", func(t *testing.T) {
		state := makeState()

		newBlock1 := simple.New(Text("4", ID("4")).Block())
		newBlock2 := simple.New(Text("5", ID("5")).Block())
		state.Add(newBlock1)
		state.Add(newBlock2)
		state.addChangesForSideMoving("1", model.Block_Left, "2", "3", "4", "5")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "1",
						Position: model.Block_Left,
						Ids:      []string{"2", "3"},
					},
				},
			},
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "3",
						Position: model.Block_Bottom,
						Blocks: []*model.Block{
							newBlock1.Model(),
							newBlock2.Model(),
						},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})
	t.Run("move multiple existing blocks and add multiple new blocks, but mixed", func(t *testing.T) {
		state := makeState()

		newBlock1 := simple.New(Text("4", ID("4")).Block())
		newBlock2 := simple.New(Text("5", ID("5")).Block())
		state.Add(newBlock1)
		state.Add(newBlock2)
		state.addChangesForSideMoving("1", model.Block_Left, "2", "4", "missing!", "3", "5")

		want := []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "1",
						Position: model.Block_Left,
						Ids:      []string{"2"},
					},
				},
			},
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "2",
						Position: model.Block_Bottom,
						Blocks: []*model.Block{
							newBlock1.Model(),
						},
					},
				},
			},
			{
				Value: &pb.ChangeContentValueOfBlockMove{
					BlockMove: &pb.ChangeBlockMove{
						TargetId: "4",
						Position: model.Block_Bottom,
						Ids:      []string{"3"},
					},
				},
			},
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						TargetId: "3",
						Position: model.Block_Bottom,
						Blocks: []*model.Block{
							newBlock2.Model(),
						},
					},
				},
			},
		}
		assert.Equal(t, want, state.GetChanges())
	})
}
