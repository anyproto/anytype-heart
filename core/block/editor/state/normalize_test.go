package state

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	_ "github.com/anyproto/anytype-heart/core/block/simple/text"
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
		div = &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Div,
			},
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

		r.Add(simple.New(&model.Block{Id: "t1", ChildrenIds: []string{"tableRows", "tableColumns"}, Content: &model.BlockContentOfTable{
			Table: &model.BlockContentTable{},
		}}))
		r.Add(simple.New(&model.Block{Id: "tableRows", Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows,
			},
		}}))
		r.Add(simple.New(&model.Block{Id: "tableColumns", Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns,
			},
		}}))

		s := r.NewState()
		s.Get("c1")
		s.Get("c2")
		s.Get("tableRows")
		s.Get("tableColumns")

		msgs, hist, err := ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, msgs, 2) // 1 remove + 1 change
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 3)
		assert.Equal(t, []string{"t1"}, r.blocks["root"].Model().ChildrenIds)
		assert.Nil(t, r.Pick("r1"))
		assert.Nil(t, r.Pick("c1"))
		assert.Nil(t, r.Pick("c2"))
		assert.NotNil(t, r.Pick("tableRows"))    // Do not remove table rows
		assert.NotNil(t, r.Pick("tableColumns")) // Do not remove table columns
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
		ev := &model.ObjectView{}
		require.NoError(t, ev.Unmarshal(data))

		r := NewDoc(ev.RootId, nil).(*State)
		for _, b := range ev.Blocks {
			r.Add(simple.New(b))
		}
		_, _, err = ApplyState(r.NewState(), true)
		require.NoError(t, err)
		// t.Log(r.String())

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

	t.Run("normalize div", func(t *testing.T) {
		// given original state
		r := NewDoc("root", nil).(*State)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div-1", "div-2", "div-3"}}))
		r.Add(simple.New(&model.Block{Id: "div-1", ChildrenIds: []string{"div-4", "b"}, Content: div}))
		r.Add(simple.New(&model.Block{Id: "div-2", ChildrenIds: []string{"c", "div-5"}, Content: div}))
		r.Add(simple.New(&model.Block{Id: "div-3", ChildrenIds: []string{"e", "f"}, Content: div}))

		r.Add(simple.New(&model.Block{Id: "div-4", Content: div}))
		r.Add(simple.New(&model.Block{Id: "div-5", ChildrenIds: []string{"div-6"}, Content: div}))
		r.Add(simple.New(&model.Block{Id: "div-6", Content: div}))
		r.Add(simple.New(&model.Block{Id: "c"}))
		r.Add(simple.New(&model.Block{Id: "d"}))
		r.Add(simple.New(&model.Block{Id: "e"}))
		r.Add(simple.New(&model.Block{Id: "f"}))

		// remove blocks
		s := r.NewState()
		s.Unlink("div-4")
		s.Unlink("div-5")
		s.Unlink("c")
		s.Unlink("d")
		s.Unlink("e")
		s.Unlink("f")

		// when
		require.NoError(t, s.Normalize(false))
		_, msg, err := ApplyState(s, false)

		// then
		require.NoError(t, err)
		assert.Len(t, s.Pick("root").Model().ChildrenIds, 0)
		expectedRemovedIDs := []string{"div-1", "div-2", "div-3", "div-4", "div-5", "div-6"}
		expectedRemovedIDsCount := lo.CountBy(msg.Remove, func(block simple.Block) bool { return slices.Contains(expectedRemovedIDs, block.Model().Id) })
		assert.Equal(t, len(expectedRemovedIDs), expectedRemovedIDsCount)
	})

	// t.Run("normalize size - big details", func(t *testing.T) {
	//	//given
	//	blocks := map[string]simple.Block{
	//		"root": simple.New(&model.Block{
	//			Id: "root",
	//			Fields: &types.Struct{Fields: map[string]*types.Value{
	//				"name": pbtypes.String(strings.Repeat("a", blockSizeLimit)),
	//			}},
	//		},
	//		)}
	//	s := NewDoc("root", blocks).(*State)
	//
	//	//when
	//	err := s.normalizeSize()
	//
	//	//then
	//	assert.Less(t, blockSizeLimit, s.blocks["root"].Model().Size())
	//	assert.Error(t, err)
	// })
	//
	// t.Run("normalize size - big content", func(t *testing.T) {
	//	//given
	//	blocks := map[string]simple.Block{
	//		"root": simple.New(&model.Block{
	//			Id: "root",
	//			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
	//				Text: strings.Repeat("b", blockSizeLimit)},
	//			},
	//		}),
	//	}
	//	s := NewDoc("root", blocks).(*State)
	//
	//	//when
	//	err := s.normalizeSize()
	//
	//	//then
	//	assert.Less(t, blockSizeLimit, s.blocks["root"].Model().Size())
	//	assert.Error(t, err)
	// })
	//
	// t.Run("normalize size - no error", func(t *testing.T) {
	//	//given
	//	blocks := map[string]simple.Block{
	//		"root": simple.New(&model.Block{
	//			Id: "root",
	//			Fields: &types.Struct{Fields: map[string]*types.Value{
	//				"name": pbtypes.String(strings.Repeat("a", blockSizeLimit/3)),
	//			}},
	//			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
	//				Text: strings.Repeat("b", blockSizeLimit/3)},
	//			},
	//		}),
	//	}
	//	s := NewDoc("root", blocks).(*State)
	//
	//	//when
	//	err := s.normalizeSize()
	//
	//	//then
	//	assert.Less(t, s.blocks["root"].Model().Size(), blockSizeLimit)
	//	assert.NoError(t, err)
	// })
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
	ev := &model.ObjectView{}
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

func TestShortenDetailsToLimit(t *testing.T) {
	t.Run("shorten description", func(t *testing.T) {
		// given
		details := map[string]*types.Value{
			bundle.RelationKeyName.String():          pbtypes.String("my page"),
			bundle.RelationKeyDescription.String():   pbtypes.String(strings.Repeat("a", detailSizeLimit+10)),
			bundle.RelationKeyWidthInPixels.String(): pbtypes.Int64(20),
		}

		// when
		shortenDetailsToLimit("", details)

		// then
		assert.Len(t, details[bundle.RelationKeyName.String()].GetStringValue(), 7)
		assert.Less(t, len(details[bundle.RelationKeyDescription.String()].GetStringValue()), detailSizeLimit)
	})
}

func TestShortenValueOnN(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		// given
		value := pbtypes.String("abrakadabra")

		// when
		value, left := shortenValueByN(value, 7)

		// then
		assert.Equal(t, 0, left)
		assert.Equal(t, "abra", value.GetStringValue())
	})

	t.Run("string list", func(t *testing.T) {
		// given
		value := pbtypes.StringList([]string{"Liberté", "Égalité", "Fraternité"})

		// when
		value, left := shortenValueByN(value, 15)

		// then
		expected := pbtypes.StringList([]string{"", "É", "Fraternité"})

		assert.Equal(t, 0, left)
		assert.Equal(t, expected, value)
	})

	t.Run("cut off all strings", func(t *testing.T) {
		// given
		value := pbtypes.StringList([]string{"😂", "😄", "🥰", "😔", "😰", "😥", "🥕", "🍅", "🌶"})

		// when
		value, left := shortenValueByN(value, 100)

		// then
		assert.Equal(t, 100-(9*4), left)
		assert.Equal(t, 0, countStringsLength(value))
	})
}

func BenchmarkShorten(b *testing.B) {
	value := pbtypes.StringList([]string{strings.Repeat("War And Peace", 50), "Anna Karenina", strings.Repeat("Youth", 100), "After the Ball"})
	for i := 0; i < b.N; i++ {
		_, _ = shortenValueByN(value, 600)
	}
}

func countStringsLength(value *types.Value) (n int) {
	switch value.Kind.(type) {
	case *types.Value_StringValue:
		return len(value.GetStringValue())
	case *types.Value_ListValue:
		for _, v := range value.GetListValue().Values {
			n += countStringsLength(v)
		}
	case *types.Value_StructValue:
		for _, v := range value.GetStructValue().GetFields() {
			n += countStringsLength(v)
		}
	}
	return n
}
