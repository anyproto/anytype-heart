package state

import (
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
		s := New("1", nil).New()
		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 0)
		assert.Empty(t, hist)
	})

	t.Run("clean missing children", func(t *testing.T) {
		r := New("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"one"}}),
			"one":  simple.New(&model.Block{Id: "one", ChildrenIds: []string{"missingid"}}),
		})
		s := r.New()
		s.Get("one")
		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 1)
		assert.Len(t, hist.Change, 1)
		assert.Len(t, r.Pick("one").Model().ChildrenIds, 0)
	})

	t.Run("remove empty layouts", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1", "t1"}}))
		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "c2", Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "t1"}))

		s := r.New()
		s.Get("c1")
		s.Get("c2")

		msgs, hist, err := s.Apply()
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
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1", "t1"}}))

		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", ChildrenIds: []string{"t2"}, Content: contColumn}))
		r.Add(simple.New(&model.Block{Id: "t1"}))
		r.Add(simple.New(&model.Block{Id: "t2"}))

		s := r.New()
		s.Get("c1")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2) // 1 remove + 1 change
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 2)
		assert.Equal(t, []string{"t2", "t1"}, r.Pick("root").Model().ChildrenIds)
		assert.Nil(t, r.Pick("r1"))
		assert.Nil(t, r.Pick("c1"))
	})
	t.Run("cleanup width", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"r1"}}))

		r.Add(simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2", "c3"}, Content: contRow}))
		r.Add(simple.New(&model.Block{Id: "c1", Content: contColumn, ChildrenIds: []string{"t1"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "c2", Content: contColumn, ChildrenIds: []string{"t2"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "c3", Content: contColumn, ChildrenIds: []string{"t3"}, Fields: fieldsWidth(0.3)}))
		r.Add(simple.New(&model.Block{Id: "t1"}))
		r.Add(simple.New(&model.Block{Id: "t2"}))
		r.Add(simple.New(&model.Block{Id: "t3"}))

		s := r.New()
		s.Remove("c2")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 4) // 1 row change + 1 remove + 2 width reset
		assert.Len(t, hist.Remove, 1)
		assert.Len(t, hist.Change, 3)
		assert.Equal(t, []string{"r1"}, r.Pick("root").Model().ChildrenIds)
		assert.Nil(t, r.Pick("c2"))
		assert.Equal(t, float64(0), r.Pick("c1").Model().Fields.Fields["width"].GetNumberValue())
		assert.Equal(t, float64(0), r.Pick("c3").Model().Fields.Fields["width"].GetNumberValue())
	})

}
