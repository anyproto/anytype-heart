package state

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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
		r := NewDoc("1", nil)
		r.(*State).Add(simple.New(&model.Block{Id: "1"}))
		s := r.NewState()
		msgs, hist, err := ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 0)
		assert.Empty(t, hist)
	})

	t.Run("lastmodifieddate should not change", func(t *testing.T) {
		r := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1"}),
		})

		r.(*State).SetLastModified(1, "abc")

		s := r.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		s.InsertTo("1", model.Block_Inner, "2")

		s.SetLastModified(2, "abc")
		msgs, hist, err := ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)      // BlockSetChildrenIds, BlockAdd, ObjectDetailsAmend(lastmodifieddate)
		assert.Len(t, s.changes, 1) // BlockCreate
		assert.Len(t, hist.Add, 1)
		assert.Len(t, hist.Change, 1)
		assert.Nil(t, hist.Details)

		s = s.NewState()
		s.SetLastModified(3, "abc")
		msgs, hist, err = ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 0) // last modified should be reverted and not msg should be produced
		assert.Len(t, s.changes, 0)
		assert.Len(t, hist.Add, 0)
		assert.Len(t, hist.Change, 0)
		assert.Nil(t, hist.Details)
	})

	t.Run("clean missing children", func(t *testing.T) {
		r := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"one"}}),
			"one":  simple.New(&model.Block{Id: "one", ChildrenIds: []string{"missingid"}}),
		}).(*State)
		s := r.NewState()
		s.Get("one")
		msgs, hist, err := ApplyState(s, true)
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

		msgs, hist, err := ApplyState(s, true)
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

		msgs, hist, err := ApplyState(s, true)
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
		s.Unlink("c2")

		msgs, hist, err := ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 4) // 1 row change + 1 remove + 2 width reset
		assert.Len(t, hist.Remove, 2)
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
		ApplyState(s, true)
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

	t.Run("normalize tree with numeric list", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		var rootIds []string
		for i := 0; i < maxChildrenThreshold+1; i++ {
			rootIds = append(rootIds, fmt.Sprint(i))
			r.Add(simple.New(&model.Block{Id: fmt.Sprint(i)}))
		}
		for i := 0; i < maxChildrenThreshold+1; i++ {
			rootIds = append(rootIds, fmt.Sprint("n", i))
			r.Add(simple.New(&model.Block{
				Id: fmt.Sprint("n", i),
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Style: model.BlockContentText_Numbered,
					},
				},
			}))
		}
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: rootIds}))

		s := r.NewState()
		_, _, err := ApplyState(s, true)
		require.NoError(t, err)
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
			msgs, _, err := ApplyState(s, true)
			require.NoError(t, err)
			for _, msg := range msgs {
				if add := msg.Msg.GetBlockAdd(); add != nil {
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
	t.Run("split with numeric #349", func(t *testing.T) {
		data, err := ioutil.ReadFile("./testdata/349_blocks.pb")
		require.NoError(t, err)
		var ev = &pb.EventObjectShow{}
		require.NoError(t, ev.Unmarshal(data))

		r := NewDoc(ev.RootId, nil).(*State)
		for _, b := range ev.Blocks {
			r.Add(simple.New(b))
		}
		_, _, err = ApplyState(r.NewState(), true)
		require.NoError(t, err)
		//t.Log(r.String())

		root := r.Pick(ev.RootId).Model()
		for _, childId := range root.ChildrenIds {
			m := r.Pick(childId).Model()
			assert.NotNil(t, m.GetLayout())
			assert.True(t, len(m.ChildrenIds) > 0)
		}
	})
	t.Run("remove duplicates", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"a", "b", "b", "c", "a", "a"}}))
		r.Add(simple.New(&model.Block{Id: "a", ChildrenIds: []string{"b", "d"}}))
		r.Add(simple.New(&model.Block{Id: "b"}))
		r.Add(simple.New(&model.Block{Id: "c", ChildrenIds: []string{"e", "e"}}))
		r.Add(simple.New(&model.Block{Id: "d"}))
		r.Add(simple.New(&model.Block{Id: "e"}))
		s := r.NewState()
		require.NoError(t, s.Normalize(false))
		_, _, err := ApplyState(s, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, r.Pick("root").Model().ChildrenIds)
		assert.Equal(t, []string{"d"}, r.Pick("a").Model().ChildrenIds)
		assert.Equal(t, []string{"e"}, r.Pick("c").Model().ChildrenIds)
	})
	t.Run("normalize header", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		var rootIds []string
		for i := 0; i < 200; i++ {
			rootIds = append(rootIds, fmt.Sprint(i))
			r.Add(simple.New(&model.Block{Id: fmt.Sprint(i)}))
		}
		r.Add(simple.New(&model.Block{Id: "header", Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Header,
			},
		}}))
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: append([]string{"header"}, rootIds...)}))

		s := r.NewState()
		s.normalizeTree()
		ApplyState(s, true)
		assert.Equal(t, "header", s.Pick(s.RootId()).Model().ChildrenIds[0])
	})

	t.Run("normalize relation: reset status max count", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r1 := &model.Relation{
			Key:      "a1",
			Format:   model.RelationFormat_status,
			Name:     "test",
			MaxCount: 0,
		}
		r.AddRelation(r1)

		s := r.NewState()
		s.NormalizeRelations()
		ApplyState(s, true)
		assert.Equal(t, int32(1), pbtypes.GetRelation(s.ExtraRelations(), "a1").MaxCount)
	})


	t.Run("normalize relation: revert bundle relation", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r1 := bundle.MustGetRelation(bundle.RelationKeyDone)
		r1.Name = "Done2"
		r.AddRelation(r1)

		s := r.NewState()
		s.NormalizeRelations()
		ApplyState(s, true)
		assert.Equal(t, "Done", pbtypes.GetRelation(s.ExtraRelations(), bundle.RelationKeyDone.String()).Name)
	})

	t.Run("normalize dv: reset status max count", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r1 := &model.Relation{
			Key:      "a1",
			Format:   model.RelationFormat_status,
			Name:     "test",
			MaxCount: 0,
		}
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"dataview"}}))

		r.Add(simple.New(&model.Block{
			Id: "dataview",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Relations: []*model.Relation{r1},
			}},
		}))

		s := r.NewState()
		d := s.Pick("dataview")
		s.normalizeDvRelations(d)
		ApplyState(s, true)

		assert.Equal(t, int32(1), pbtypes.GetRelation(r.Pick("dataview").Model().GetDataview().Relations, "a1").MaxCount)
	})

	t.Run("normalize dv: revert bundle relation", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r1 := bundle.MustGetRelation(bundle.RelationKeyDone)
		r1.Name = "Done2"

		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"dataview"}}))

		r.Add(simple.New(&model.Block{
			Id: "dataview",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Relations: []*model.Relation{r1},
			}},
		}))

		s := r.NewState()
		d := s.Pick("dataview")
		s.normalizeDvRelations(d)
		ApplyState(s, true)

		assert.Equal(t, "Done", pbtypes.GetRelation(r.Pick("dataview").Model().GetDataview().Relations, bundle.RelationKeyDone.String()).Name)
	})

	t.Run("normalize dv: remove duplicate relations", func(t *testing.T) {
		r := NewDoc("root", nil).(*State)
		r1 := &model.Relation{Name: "rel1", Key: "123", Format: model.RelationFormat_longtext}
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"dataview"}}))

		r.Add(simple.New(&model.Block{
			Id: "dataview",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Relations: []*model.Relation{pbtypes.CopyRelation(r1), pbtypes.CopyRelation(r1), pbtypes.CopyRelation(r1)},
			}},
		}))

		s := r.NewState()
		d := s.Pick("dataview")
		s.normalizeDvRelations(d)
		ApplyState(s, true)

		assert.Len(t, r.Pick("dataview").Model().GetDataview().Relations, 1)
	})


}

func TestCleanupLayouts(t *testing.T) {
	newDiv := func(id string, childrenIds ...string) simple.Block {
		return simple.New(&model.Block{
			Id:          id,
			ChildrenIds: childrenIds,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Div,
				},
			},
		})
	}
	newText := func(id string, childrenIds ...string) simple.Block {
		return simple.New(&model.Block{
			Id:          id,
			ChildrenIds: childrenIds,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: id,
				},
			},
		})
	}
	d := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}}),
		"div1": newDiv("div1", "div3", "3", "4"),
		"div2": newDiv("div2", "5", "6"),
		"div3": newDiv("div3", "1", "2"),
		"1":    newText("1"),
		"2":    newText("2"),
		"3":    newText("3"),
		"4":    newText("4"),
		"5":    newText("5"),
		"6":    newText("6"),
	})
	s := d.NewState()
	assert.Equal(t, CleanupLayouts(s), 3)
	assert.Len(t, s.Pick("root").Model().ChildrenIds, 6)
}

func BenchmarkNormalize(b *testing.B) {
	data, err := ioutil.ReadFile("./testdata/349_blocks.pb")
	require.NoError(b, err)
	var ev = &pb.EventObjectShow{}
	require.NoError(b, ev.Unmarshal(data))

	r := NewDoc(ev.RootId, nil).(*State)
	for _, b := range ev.Blocks {
		r.Add(simple.New(b))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ApplyState(r.NewState(), true)
	}
}
