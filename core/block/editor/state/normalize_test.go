package state

import (
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Normalize(t *testing.T) {
	var (
		contRow = &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		}
		contColumn = &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		}
		fieldsWidth = func(w float64) *types.Struct {
			return &types.Struct{
				Fields: map[string]*types.Value{
					"width": pbtypes.Float64(w),
				},
			}
		}
	)

	t.Run("nothing to change", func(t *testing.T) {
		s := NewDoc("1", nil).NewState()
		msgs, hist, err := s.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 0)
		assert.Empty(t, hist)
	})

	t.Run("clean missing children", func(t *testing.T) {
		r := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"one"}}),
			"one":  simple.New(&model.Block{Id: "one", ChildrenIds: []string{"missingid"}}),
		}).(*State)
		s := r.NewState()
		s.Get("one")
		msgs, hist, err := s.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 1)
		assert.Len(t, hist.Change, 1)
		assert.Len(t, r.Pick("one").Model().ChildrenIds, 0)
	})

	t.Run("remove empty layouts", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1", "t1"}}))
		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "c2", Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "t1"}))

		s := r.NewState()
		s.Get("c1")
		s.Get("c2")

		msgs, hist, err := s.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2) // 1 remove + 1 change
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 3)
		assert.Equal(t, []string{"t1"}, r.blocks["root"].Model().ChildrenIds)
		assert.Nil(t, r.Pick("r1"))
		assert.Nil(t, r.Pick("c1"))
		assert.Nil(t, r.Pick("c2"))
	})

	t.Run("remove one column row", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1", "t1"}}))

		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", ChildrenIds: []string{"t2"}, Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "t1"}))
		r.Add(simple.New(&model.Block{Id: "t2"}))

		s := r.NewState()
		s.Get("c1")

		msgs, hist, err := s.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2) // 1 remove + 1 change
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 2)
		assert.Equal(t, []string{"t2", "t1"}, r.Pick("root").Model().ChildrenIds)
		assert.Nil(t, r.Pick("r1"))
		assert.Nil(t, r.Pick("c1"))
	})
	t.Run("cleanup width", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1"}}))

		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2", "c3"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", Content: contColumn, ChildrenIds: []string{"t1"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "c2", Content: contColumn, ChildrenIds: []string{"t2"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "c3", Content: contColumn, ChildrenIds: []string{"t3"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "t1"}))
		r.Add(simple.New(&model.Block{Id: "t2"}))
		r.Add(simple.New(&model.Block{Id: "t3"}))

		s := r.NewState()
		s.Remove("c2")

		msgs, hist, err := s.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 4) // 1 row change + 1 remove + 2 width reset
		assert.Len(t, hist.Remove, 1)
		assert.Len(t, hist.Change, 3)
		assert.Equal(t, []string{"r1"}, r.Pick("root").Model().ChildrenIds)
		assert.Nil(t, r.Pick("c2"))
		assert.Equal(t, float64(0), r.Pick("c1").Model().Fields.Fields["width"].GetNumberValue())
		assert.Equal(t, float64(0), r.Pick("c3").Model().Fields.Fields["width"].GetNumberValue())
	})

	t.Run("normalize tree", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		var rootIds []string
		for i := 0; i < 200; i++ {
			rootIds = append(rootIds, fmt.Sprint(i))
			r.Add(simple.New(&model.Block{Id: fmt.Sprint(i)}))
		}
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: rootIds}))

		s := r.NewState()
		s.normalizeTree()
		ApplyState(s)
		blocks := r.Blocks()
		result := []string{}
		divs := []string{}
		for _, m := range blocks {
			if m.Id == r.RootId() {
				continue
			}
			if m.GetLayout() != nil {
				divs = append(divs, m.Id)
			} else {
				result = append(result, m.Id)
			}
		}
		assert.Len(t, result, 200)
		assert.True(t, len(divs) > 0)
	})

	t.Run("div balance", func(t *testing.T) {
		genIds := func(length, start int) []string {
			res := make([]string, length)
			for i := range res {
				res[i] = fmt.Sprint(start)
				start++
			}
			return res
		}
		t.Run("0-0", func(t *testing.T) {
			d1 := &model.Block{}
			d2 := &model.Block{}
			require.False(t, divBalance(d1, d2))
			assert.Len(t, d1.ChildrenIds, 0)
			assert.Len(t, d2.ChildrenIds, 0)
		})
		t.Run("1-0", func(t *testing.T) {
			d1 := &model.Block{ChildrenIds: genIds(1, 1)}
			d2 := &model.Block{}
			require.False(t, divBalance(d1, d2))
			assert.Len(t, d1.ChildrenIds, 0)
			assert.Len(t, d2.ChildrenIds, 1)
		})
		t.Run("4-2", func(t *testing.T) {
			d1 := &model.Block{ChildrenIds: genIds(4, 1)}
			d2 := &model.Block{ChildrenIds: genIds(2, 5)}
			require.False(t, divBalance(d1, d2))
			assert.Equal(t, []string{"1", "2", "3"}, d1.ChildrenIds)
			assert.Equal(t, []string{"4", "5", "6"}, d2.ChildrenIds)
		})
		t.Run("overflow", func(t *testing.T) {
			d1 := &model.Block{ChildrenIds: genIds(maxChildrenThreshold, 1)}
			d2 := &model.Block{ChildrenIds: genIds(maxChildrenThreshold+1, maxChildrenThreshold+10)}
			require.True(t, divBalance(d1, d2))
			assert.Len(t, d1.ChildrenIds, divSize)
			assert.Len(t, d2.ChildrenIds, maxChildrenThreshold-divSize+maxChildrenThreshold+1)
		})
	})
	t.Run("normalize on insert", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root"}))
		targetId := "root"
		targetPos := model.Block_Inner
		for i := 0; i < 100; i++ {
			s := r.NewState()
			id := fmt.Sprint(i)
			s.Add(simple.New(&model.Block{Id: id}))
			s.InsertTo(targetId, targetPos, id)
			msgs, _, err := ApplyState(s)
			require.NoError(t, err)
			for _, msg := range msgs {
				if add := msg.GetBlockAdd(); add != nil {
					for _, nb := range add.Blocks {
						for _, nbch := range nb.ChildrenIds {
							require.NotEmpty(t, nbch)
						}
					}
				}
			}
			targetId = id
			targetPos = model.Block_Top
		}

		blocks := r.Blocks()
		result := []string{}
		divs := []string{}
		for _, m := range blocks {
			if m.Id == r.RootId() {
				continue
			}
			if m.GetLayout() != nil {
				divs = append(divs, m.Id)
			} else {
				result = append(result, m.Id)
			}
		}
		assert.Len(t, result, 100)
		assert.True(t, len(divs) > 0)
	})

}
