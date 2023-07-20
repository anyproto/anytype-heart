package state

import (
	"errors"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"math/rand"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestState_Add(t *testing.T) {
	s := NewDoc("1", nil).NewState()
	assert.Nil(t, s.Get("1"))
	assert.True(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
	})))
	assert.NotNil(t, s.Get("1"))
	assert.False(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
	})))
}

func TestState_AddRemoveAdd(t *testing.T) {
	s := NewDoc("1", nil).NewState()
	assert.Nil(t, s.Get("1"))
	assert.True(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
		Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id:   "v1",
			Name: "v1",
		}}}},
	})))
	assert.NotNil(t, s.Get("1"))
	s.Unlink("1")
	s.Set(base.NewBase(&model.Block{
		Id: "1",
		Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id:   "v1",
			Name: "v1",
		}}}},
	}))
	assert.False(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
	})))
}

func TestState_Get(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1"}),
	}).NewState()
	assert.NotNil(t, s.Get("1"))
	assert.NotNil(t, s.NewState().Get("1"))
}

func TestState_Pick(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1"}),
	}).NewState()
	assert.NotNil(t, s.Pick("1"))
	assert.NotNil(t, s.NewState().Pick("1"))
}

func TestState_Unlink(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
		"2": base.NewBase(&model.Block{Id: "2"}),
	}).NewState()
	assert.True(t, s.Unlink("2"))
	assert.Len(t, s.Pick("1").Model().ChildrenIds, 0)
	assert.False(t, s.Unlink("2"))
}

func TestState_GetParentOf(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		s := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
			"2": base.NewBase(&model.Block{Id: "2"}),
		}).NewState()
		assert.Equal(t, "1", s.GetParentOf("2").Model().Id)
	})
	t.Run("direct", func(t *testing.T) {
		s := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
			"2": base.NewBase(&model.Block{Id: "2"}),
		}).(*State)
		assert.Equal(t, "1", s.GetParentOf("2").Model().Id)
	})
}

func TestApplyState(t *testing.T) {
	t.Run("intermediate apply", func(t *testing.T) {
		d := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
			"2": base.NewBase(&model.Block{Id: "2"}),
		})
		s := d.NewState()
		s.Add(simple.New(&model.Block{Id: "3"}))
		s.InsertTo("2", model.Block_Bottom, "3")
		s.changeId = "1"

		s = s.NewState()
		s.Add(simple.New(&model.Block{Id: "4"}))
		s.InsertTo("3", model.Block_Bottom, "4")
		s.changeId = "2"

		s = s.NewState()
		s.Unlink("3")
		s.changeId = "3"

		s = s.NewState()
		s.Add(simple.New(&model.Block{Id: "5"}))
		s.InsertTo("4", model.Block_Bottom, "5")
		s.changeId = "4"

		msgs, hist, err := ApplyState(s, true)
		require.NoError(t, err)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 0)
		require.Len(t, msgs, 2)
	})
	t.Run("details handle", func(t *testing.T) {
		t.Run("text", func(t *testing.T) {
			d := NewDoc("1", map[string]simple.Block{
				"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
				"2": text.NewDetails(&model.Block{
					Id: "2",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{},
					},
					Fields: &types.Struct{
						Fields: map[string]*types.Value{
							text.DetailsKeyFieldName: pbtypes.String("name"),
						},
					},
				}, text.DetailsKeys{
					Text: "name",
				}),
			})
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.SetDetails(&types.Struct{
				Fields: map[string]*types.Value{
					"name": pbtypes.String("new name"),
				},
			})
			msgs, _, err := ApplyState(s, true)
			require.NoError(t, err)
			assert.Len(t, msgs, 2)
		})
		t.Run("checked", func(t *testing.T) {
			d := NewDoc("1", map[string]simple.Block{
				"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
				"2": text.NewDetails(&model.Block{
					Id: "2",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{},
					},
					Fields: &types.Struct{
						Fields: map[string]*types.Value{
							text.DetailsKeyFieldName: pbtypes.StringList([]string{"", "done"}),
						},
					},
				}, text.DetailsKeys{
					Checked: "done",
				}),
			})
			d.(*State).SetDetail("done", pbtypes.Bool(true))
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.SetDetails(&types.Struct{
				Fields: map[string]*types.Value{
					"done": pbtypes.Bool(false),
				},
			})
			msgs, _, err := ApplyState(s, true)
			require.NoError(t, err)
			assert.Len(t, msgs, 2)
		})
		t.Run("setChecked", func(t *testing.T) {
			d := NewDoc("1", map[string]simple.Block{
				"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
				"2": text.NewDetails(&model.Block{
					Id: "2",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{},
					},
					Fields: &types.Struct{
						Fields: map[string]*types.Value{
							text.DetailsKeyFieldName: pbtypes.StringList([]string{"", "done"}),
						},
					},
				}, text.DetailsKeys{
					Checked: "done",
				}),
			})
			d.(*State).SetDetail("done", pbtypes.Bool(true))
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.Get("2").(text.Block).SetChecked(false)
			msgs, _, err := ApplyState(s, true)
			require.NoError(t, err)
			assert.Len(t, msgs, 2)
		})
	})

}

func TestState_IsChild(t *testing.T) {
	s := NewDoc("root", map[string]simple.Block{
		"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2"}}),
		"2":    base.NewBase(&model.Block{Id: "2", ChildrenIds: []string{"3"}}),
		"3":    base.NewBase(&model.Block{Id: "3"}),
	}).NewState()
	assert.True(t, s.IsChild("2", "3"))
	assert.True(t, s.IsChild("root", "3"))
	assert.True(t, s.IsChild("root", "2"))
	assert.False(t, s.IsChild("root", "root"))
	assert.False(t, s.IsChild("3", "2"))
}

func TestState_HasParent(t *testing.T) {
	s := NewDoc("root", map[string]simple.Block{
		"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"1", "2"}}),
		"1":    base.NewBase(&model.Block{Id: "1"}),
		"2":    base.NewBase(&model.Block{Id: "2", ChildrenIds: []string{"3"}}),
		"3":    base.NewBase(&model.Block{Id: "3"}),
	}).NewState()
	t.Run("not parent", func(t *testing.T) {
		assert.False(t, s.HasParent("1", "2"))
		assert.False(t, s.HasParent("1", ""))
	})
	t.Run("parent", func(t *testing.T) {
		assert.True(t, s.HasParent("3", "2"))
		assert.True(t, s.HasParent("3", "root"))
		assert.True(t, s.HasParent("2", "root"))
	})
}

func BenchmarkState_Iterate(b *testing.B) {
	newBlock := func(id string, childrenIds ...string) simple.Block {
		return simple.New(&model.Block{Id: id, ChildrenIds: childrenIds})
	}

	s := NewDoc("root", nil).NewState()
	root := newBlock("root")
	s.Add(root)
	for i := 0; i < 100; i++ {
		ch1Id := bson.NewObjectId().Hex()
		root.Model().ChildrenIds = append(root.Model().ChildrenIds, ch1Id)
		ch1 := newBlock(ch1Id)
		s.Add(ch1)
		for j := 0; j < 10; j++ {
			ch2Id := bson.NewObjectId().Hex()
			ch2 := newBlock(ch2Id)
			ch1.Model().ChildrenIds = append(ch1.Model().ChildrenIds, ch2Id)
			s.Add(ch2)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.Iterate(func(b simple.Block) (isContinue bool) {
			return true
		})
	}
}

func TestState_IsEmpty(t *testing.T) {
	t.Run("without title block", func(t *testing.T) {
		s := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"header", "emptyText"},
			}),
			"header": simple.New(&model.Block{Id: "header"}),
			"emptyText": simple.New(&model.Block{Id: "emptyText",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
				}}),
		}).(*State)
		assert.True(t, s.IsEmpty(true))
		s.Pick("emptyText").Model().GetText().Text = "1"
		assert.False(t, s.IsEmpty(true))
	})

	t.Run("with title block", func(t *testing.T) {
		s := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"header"},
			}),
			"header": simple.New(&model.Block{Id: "header", ChildrenIds: []string{"title"}}),
			"title": simple.New(&model.Block{Id: "title",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
				}}),
		}).(*State)

		assert.True(t, s.IsEmpty(true))
		assert.True(t, s.IsEmpty(false))

		s.Pick("title").Model().GetText().Text = "1"
		assert.False(t, s.IsEmpty(true))
		assert.True(t, s.IsEmpty(false))
	})

	t.Run("with title block and empty block", func(t *testing.T) {
		s := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"header", "emptyText"},
			}),
			"header": simple.New(&model.Block{Id: "header", ChildrenIds: []string{"title"}}),
			"title": simple.New(&model.Block{Id: "title",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
				}}),
			"emptyText": simple.New(&model.Block{Id: "emptyText",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
				}}),
		}).(*State)

		assert.False(t, s.IsEmpty(true))
		assert.False(t, s.IsEmpty(false))
	})
}

func TestState_Descendants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		blocks []*model.Block
		rootId string
		want   []string
	}{
		{
			name: "root is absent",
			blocks: []*model.Block{
				{Id: "test"},
			},
			rootId: "foo",
			want:   []string{},
		},
		{
			name: "root without descendants",
			blocks: []*model.Block{
				{Id: "test"},
			},
			rootId: "test",
			want:   []string{},
		},
		{
			name: "root with one level of descendants",
			blocks: []*model.Block{
				{Id: "test", ChildrenIds: []string{"1", "2"}},
				{Id: "1"},
				{Id: "2"},
			},
			rootId: "test",
			want:   []string{"1", "2"},
		},
		{
			name: "root with one level of descendants and some blocks are nil",
			blocks: []*model.Block{
				{Id: "test", ChildrenIds: []string{"1", "2"}},
				{Id: "1"},
			},
			rootId: "test",
			want:   []string{"1"},
		},
		{
			name: "root with multiple level of descendants",
			blocks: []*model.Block{
				{Id: "test", ChildrenIds: []string{"1", "2"}},
				{Id: "1", ChildrenIds: []string{"1.1", "1.2"}},
				{Id: "1.1"},
				{Id: "1.2", ChildrenIds: []string{"1.2.1", "1.2.2"}},
				{Id: "1.2.1"},
				{Id: "1.2.2"},
				{Id: "2", ChildrenIds: []string{"2.1"}},
				{Id: "2.1"},
			},
			rootId: "test",
			want:   []string{"1", "2", "1.1", "1.2", "1.2.1", "1.2.2", "2.1"},
		},

		{
			name: "complex tree and request for descendants of middle node",
			blocks: []*model.Block{
				{Id: "test", ChildrenIds: []string{"1", "2"}},
				{Id: "1", ChildrenIds: []string{"1.1", "1.2"}},
				{Id: "1.1"},
				{Id: "1.2", ChildrenIds: []string{"1.2.1", "1.2.2"}},
				{Id: "1.2.1"},
				{Id: "1.2.2"},
				{Id: "2", ChildrenIds: []string{"2.1"}},
				{Id: "2.1"},
			},
			rootId: "1.2",
			want:   []string{"1.2.1", "1.2.2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewDoc("root", nil).NewState()
			for _, b := range tc.blocks {
				s.Add(simple.New(b))
			}

			got := s.Descendants(tc.rootId)

			gotIds := make([]string, 0, len(got))
			for _, b := range got {
				b2 := s.Pick(b.Model().Id)
				require.NotNil(t, b2)
				assert.Equal(t, b2, b)

				gotIds = append(gotIds, b.Model().Id)
			}

			assert.ElementsMatch(t, tc.want, gotIds)
		})
	}
}

func TestState_SelectRoots(t *testing.T) {
	t.Run("simple state", func(t *testing.T) {
		s := NewDoc("root", nil).NewState()
		s.Add(mkBlock("root", "1", "2", "3"))
		s.Add(mkBlock("1"))
		s.Add(mkBlock("2", "2.1"))
		s.Add(mkBlock("3"))

		assert.Equal(t, []string{"root"}, s.SelectRoots([]string{"root", "2", "3"}))
		assert.Equal(t, []string{"root"}, s.SelectRoots([]string{"3", "root", "2"}))
		assert.Equal(t, []string{"1", "2"}, s.SelectRoots([]string{"1", "2", "2.1"}))
		assert.Equal(t, []string{}, s.SelectRoots([]string{"4"}))
	})

	t.Run("with complex state", func(t *testing.T) {
		s := mkComplexState()

		assert.Equal(t, []string{"root"}, s.SelectRoots([]string{"root", "1.3.4"}))
		assert.Equal(t, []string{"1.3.4"}, s.SelectRoots([]string{"1.3.4"}))
		assert.Equal(t, []string{"1.1", "1.2", "1.3"}, s.SelectRoots([]string{"1.1", "1.2", "1.3"}))
		assert.Equal(t, []string{"1.1", "1.2", "1.3"}, s.SelectRoots([]string{"1.1", "1.2", "1.3"}))

		t.Run("chaotic args", func(t *testing.T) {
			var allIds []string
			for _, b := range s.Blocks() {
				allIds = append(allIds, b.Id)
			}
			for i := 0; i < len(allIds); i++ {
				rand.Shuffle(len(allIds), func(i, j int) { allIds[i], allIds[j] = allIds[j], allIds[i] })
				assert.Equal(t, []string{"root"}, s.SelectRoots(allIds))
			}
		})
	})
}

func mkBlock(id string, children ...string) simple.Block {
	return simple.New(&model.Block{Id: id, ChildrenIds: children})
}

func mkComplexState() *State {
	s := NewDoc("root", nil).NewState()
	for _, b := range []simple.Block{
		mkBlock("root", "1", "2", "3"),
		mkBlock("1", "1.1", "1.2", "1.3"),
		mkBlock("1.1"),
		mkBlock("1.2"),
		mkBlock("1.3", "1.3.1", "1.3.2", "1.3.3", "1.3.4"),
		mkBlock("1.3.1"),
		mkBlock("1.3.2"),
		mkBlock("1.3.3"),
		mkBlock("1.3.4"),
		mkBlock("2"),
		mkBlock("3"),
	} {
		s.Add(b)
	}
	return s
}

func BenchmarkState_SelectRoots(b *testing.B) {
	s := mkComplexState()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = s.SelectRoots([]string{"3", "root", "2", "1.3.1", "1.2", "1.3", "1.1"})
	}
}

func TestState_GetChangedStoreKeys(t *testing.T) {
	p := NewDoc("test", nil).(*State)
	p.SetInStore([]string{"one", "two", "v1"}, pbtypes.String("val1"))
	p.SetInStore([]string{"one", "two", "v2"}, pbtypes.String("val2"))
	p.SetInStore([]string{"one", "two", "v3"}, pbtypes.String("val3"))
	p.SetInStore([]string{"other"}, pbtypes.String("val42"))
	changed := p.GetChangedStoreKeys()

	s := p.NewState()
	s.SetInStore([]string{"one", "two", "v2"}, pbtypes.String("val2ch"))
	s.SetInStore([]string{"other"}, pbtypes.String("changed"))
	s.RemoveFromStore([]string{"one", "two", "v3"})

	changed = s.GetChangedStoreKeys()
	assert.Len(t, changed, 3)
	assert.Contains(t, changed, []string{"one", "two", "v2"})
	assert.Contains(t, changed, []string{"one", "two"})
	assert.Contains(t, changed, []string{"other"})

	changed = s.GetChangedStoreKeys("one")
	assert.Len(t, changed, 2)
	changed = s.GetChangedStoreKeys("one", "two", "v2")
	assert.Len(t, changed, 1)
}

func TestState_GetSetting(t *testing.T) {
	st := NewDoc("test", nil).(*State)
	assert.Nil(t, st.GetSetting("setting"))

	st.SetInStore([]string{SettingsStoreKey}, nil)
	assert.Nil(t, st.GetSetting("setting"))

	settings := pbtypes.Struct(&types.Struct{Fields: map[string]*types.Value{"setting": pbtypes.String("setting")}})
	st.SetInStore([]string{SettingsStoreKey}, settings)
	assert.NotNil(t, st.GetSetting("setting"))
	assert.Equal(t, settings.GetStructValue().GetFields()["setting"], st.GetSetting("setting"))
}

func TestState_GetStoreSlice(t *testing.T) {
	st := NewDoc("test", nil).(*State)
	assert.Nil(t, st.GetStoreSlice("collection"))

	st.SetInStore([]string{"collection"}, nil)
	assert.Nil(t, st.GetStoreSlice("collection"))

	st.SetInStore([]string{"collection"}, pbtypes.StringList([]string{"object1", "object2"}))
	assert.NotNil(t, st.GetStoreSlice("collection"))
	assert.Equal(t, []string{"object1", "object2"}, st.GetStoreSlice("collection"))
}

func TestState_GetSubObjectCollection(t *testing.T) {
	const collectionName = "subcollection"
	st := NewDoc("test", nil).(*State)
	assert.Nil(t, st.GetSubObjectCollection(collectionName))

	st.SetInStore([]string{collectionName}, nil)
	assert.Nil(t, st.GetSubObjectCollection(collectionName))

	subObjectDetails := pbtypes.Struct(
		&types.Struct{
			Fields: map[string]*types.Value{
				"subobject1": pbtypes.Struct(nil),
				"subobject2": pbtypes.Struct(nil),
			},
		})
	st.SetInStore([]string{collectionName}, subObjectDetails)
	assert.NotNil(t, st.GetSubObjectCollection(collectionName))
	assert.Equal(t, subObjectDetails.GetStructValue(), st.GetSubObjectCollection(collectionName))
}

func TestState_ContainsInStore(t *testing.T) {
	const collectionName = "subcollection"
	st := NewDoc("test", nil).(*State)
	assert.False(t, st.ContainsInStore([]string{collectionName, "subobject1"}))

	st.SetInStore([]string{collectionName}, nil)
	assert.False(t, st.ContainsInStore([]string{collectionName, "subobject2"}))

	subObjectDetails := pbtypes.Struct(
		&types.Struct{
			Fields: map[string]*types.Value{
				"subobject1": pbtypes.Struct(&types.Struct{}),
				"subobject2": pbtypes.Struct(&types.Struct{
					Fields: map[string]*types.Value{
						"subobject3": pbtypes.Struct(nil),
					},
				}),
			},
		})
	st.SetInStore([]string{collectionName}, subObjectDetails)
	assert.False(t, st.ContainsInStore([]string{collectionName, "subobject3"}))
	//nested
	assert.False(t, st.ContainsInStore([]string{collectionName, "subobject1", "subobject3"}))
	assert.True(t, st.ContainsInStore([]string{collectionName, "subobject2", "subobject3"}))
	assert.True(t, st.ContainsInStore([]string{collectionName, "subobject1"}))
}

func TestState_HasInStore(t *testing.T) {
	const collectionName = "subcollection"
	st := NewDoc("test", nil).(*State)
	assert.False(t, st.HasInStore([]string{collectionName, "subobject1"}))

	st.SetInStore([]string{collectionName}, nil)
	assert.False(t, st.HasInStore([]string{collectionName, "subobject2"}))

	subObjectDetails := pbtypes.Struct(
		&types.Struct{
			Fields: map[string]*types.Value{
				"subobject1": pbtypes.Struct(&types.Struct{}),
				"subobject2": pbtypes.Struct(&types.Struct{
					Fields: map[string]*types.Value{
						"subobject3": pbtypes.Struct(nil),
					},
				}),
			},
		})
	st.SetInStore([]string{collectionName}, subObjectDetails)
	assert.False(t, st.HasInStore([]string{collectionName, "subobject3"}))
	//nested
	assert.False(t, st.HasInStore([]string{collectionName, "subobject1", "subobject3"}))
	assert.True(t, st.HasInStore([]string{collectionName, "subobject2", "subobject3"}))
	assert.True(t, st.HasInStore([]string{collectionName, "subobject1"}))
}

func TestState_Validate(t *testing.T) {
	s := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock", ChildrenIds: []string{"childBlock1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
			}}),
		"childBlock1": simple.New(&model.Block{Id: "childBlock1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
			}}),
	}).(*State)

	//Valid state
	assert.Nil(t, s.Validate())

}
func TestState_ValidateChildWithTwoParents(t *testing.T) {
	childrenWithTwoParents := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock1"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock", ChildrenIds: []string{"childBlock1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
			}}),
		"childBlock1": simple.New(&model.Block{Id: "childBlock1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
			}}),
	}).(*State)

	//Not valid state
	assert.NotNil(t, childrenWithTwoParents.Validate())
	assert.Contains(t, childrenWithTwoParents.Validate().Error(), "two children with same id")

}

func TestState_ValidateMissedChildBlockState(t *testing.T) {
	missedChildBlockState := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock", ChildrenIds: []string{"childBlock1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}},
			}}),
	}).(*State)

	//Not valid state
	assert.NotNil(t, missedChildBlockState.Validate())
	assert.Contains(t, missedChildBlockState.Validate().Error(), "missed block")
}

func TestState_DepSmartIdsLinks(t *testing.T) {
	stateWithLinks := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
	}).(*State)

	objectIDs := stateWithLinks.DepSmartIds(true, false, false, false, false)
	assert.Len(t, objectIDs, 4)

	objectIDs = stateWithLinks.DepSmartIds(false, false, false, false, false)
	assert.Len(t, objectIDs, 0)
}

func TestState_DepSmartIdsLinksAndRelations(t *testing.T) {
	stateWithLinks := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
	}).(*State)

	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_file,
		},
		{
			Key:    "relation2",
			Format: model.RelationFormat_tag,
		},
		{
			Key:    "relation3",
			Format: model.RelationFormat_status,
		},
		{
			Key:    "relation4",
			Format: model.RelationFormat_object,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	objectIDs := stateWithLinks.DepSmartIds(true, false, false, false, false)
	assert.Len(t, objectIDs, 4)

	objectIDs = stateWithLinks.DepSmartIds(true, false, true, false, false)
	assert.Len(t, objectIDs, 10) // 4 links + 4 relations + 2 derived relations
}

func TestState_DepSmartIdsLinksDetailsAndRelations(t *testing.T) {
	stateWithLinks := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
	}).(*State)

	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_file,
		},
		{
			Key:    "relation2",
			Format: model.RelationFormat_tag,
		},
		{
			Key:    "relation3",
			Format: model.RelationFormat_status,
		},
		{
			Key:    "relation4",
			Format: model.RelationFormat_object,
		},
		{
			Key:    "relation5",
			Format: model.RelationFormat_date,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	stateWithLinks.SetDetail("relation1", pbtypes.String("file"))
	stateWithLinks.SetDetail("relation2", pbtypes.String("option1"))
	stateWithLinks.SetDetail("relation3", pbtypes.String("option2"))
	stateWithLinks.SetDetail("relation4", pbtypes.String("option3"))
	stateWithLinks.SetDetail("relation5", pbtypes.Int64(time.Now().Unix()))
	objectIDs := stateWithLinks.DepSmartIds(true, false, false, false, false)
	assert.Len(t, objectIDs, 4) // links

	objectIDs = stateWithLinks.DepSmartIds(true, false, true, false, false)
	assert.Len(t, objectIDs, 11) // 4 links + 5 relations + 2 derived relations

	objectIDs = stateWithLinks.DepSmartIds(true, true, true, false, false)
	assert.Len(t, objectIDs, 16) // 4 links + 5 relations + 2 derived relations + 3 options + 1 fileID + 1 date
}

func TestState_DepSmartIdsLinksCreatorModifierWorkspace(t *testing.T) {
	stateWithLinks := NewDoc("root", nil).(*State)

	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_date,
		},
		{
			Key:    bundle.RelationKeyCreatedDate.String(),
			Format: model.RelationFormat_date,
		},
		{
			Key:    bundle.RelationKeyCreator.String(),
			Format: model.RelationFormat_object,
		},
		{
			Key:    bundle.RelationKeyWorkspaceId.String(),
			Format: model.RelationFormat_object,
		},
		{
			Key:    bundle.RelationKeyLastModifiedBy.String(),
			Format: model.RelationFormat_object,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	stateWithLinks.SetDetail("relation1", pbtypes.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreatedDate.String(), pbtypes.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreator.String(), pbtypes.String("creator"))
	stateWithLinks.SetDetail(bundle.RelationKeyWorkspaceId.String(), pbtypes.String("workspaceID"))
	stateWithLinks.SetDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String("lastModifiedBy"))

	objectIDs := stateWithLinks.DepSmartIds(false, true, false, false, true)
	assert.Len(t, objectIDs, 4) // creator + workspaceID + lastModifiedBy + 1 date

	objectIDs = stateWithLinks.DepSmartIds(false, true, true, false, true)
	assert.Len(t, objectIDs, 11) // 5 relations + creator + workspaceID + lastModifiedBy + 1 date + 2 derived relations
}

func TestState_DepSmartIdsObjectTypes(t *testing.T) {
	stateWithLinks := NewDoc("root", nil).(*State)

	stateWithLinks.SetObjectType(bundle.TypeKeyPage.URL())

	objectIDs := stateWithLinks.DepSmartIds(false, false, false, false, false)
	assert.Len(t, objectIDs, 0)

	objectIDs = stateWithLinks.DepSmartIds(false, false, false, true, false)
	assert.Len(t, objectIDs, 1)
	assert.Equal(t, objectIDs[0], bundle.TypeKeyPage.URL())
}

func TestState_CheckRestrictions(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	assert.Nil(t, st.CheckRestrictions()) // empty parent state

	parentState := NewDoc("root", nil).(*State)
	st.SetParent(parentState) // no same blocks

	parentState = NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		})}).(*State)

	st.SetParent(parentState)
	assert.Nil(t, st.CheckRestrictions()) // no restrictions
}

func TestState_CheckRestrictionsBlockHasRestriction(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	parentState := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		})}).(*State)

	st.SetParent(parentState)
	assert.Nil(t, st.CheckRestrictions()) // no changes

	parentState = NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "parentText",
				},
			},
		})}).(*State)

	st.SetParent(parentState)
	assert.NotNil(t, st.CheckRestrictions())
	assert.True(t, errors.Is(st.CheckRestrictions(), ErrRestricted))
}

func TestState_ApplyChangeIgnoreErrBlockCreate(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{
		BlockCreate: &pb.ChangeBlockCreate{
			TargetId: "newTextBlock",
			Position: model.Block_Bottom,
			Blocks: []*model.Block{
				{
					Id:           "newTextBlock",
					Restrictions: &model.BlockRestrictions{Edit: true},
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text: "new text",
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("newTextBlock")
	assert.NotNil(t, b)
	assert.NotNil(t, b.Model().GetText())
	assert.Equal(t, "new text", b.Model().GetText().GetText())
	assert.Len(t, st.blocks, 3)

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{
		BlockCreate: &pb.ChangeBlockCreate{
			TargetId: "root",
			Position: model.Block_Inner,
			Blocks: []*model.Block{
				{
					Id:           "root",
					Restrictions: &model.BlockRestrictions{Edit: true},
					Content: &model.BlockContentOfSmartblock{
						Smartblock: &model.BlockContentSmartblock{},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Len(t, st.blocks, 3) // root block not added

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{
		BlockCreate: &pb.ChangeBlockCreate{
			TargetId: "dataview",
			Position: model.Block_Bottom,
			Blocks: []*model.Block{
				{
					Id:           "dataview",
					Restrictions: &model.BlockRestrictions{Edit: true},
					Content: &model.BlockContentOfDataview{
						Dataview: &model.BlockContentDataview{},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b = st.Get("dataview")
	assert.NotNil(t, b)
	assert.NotNil(t, b.Model().GetDataview())
	assert.Len(t, st.blocks, 4)
}

func TestState_ApplyChangeIgnoreErrBlockRemove(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockRemove{
		BlockRemove: &pb.ChangeBlockRemove{Ids: []string{"textBlock"}},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("textBlock")
	assert.Nil(t, b)
	assert.Len(t, st.Blocks(), 1)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateBase(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetAlign{
						BlockSetAlign: &pb.EventBlockSetAlign{
							Id:    "textBlock",
							Align: model.Block_AlignRight,
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, model.Block_AlignRight, b.Model().Align)

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
						BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
							Id:            "textBlock",
							VerticalAlign: model.Block_VerticalAlignBottom,
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b = st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, model.Block_VerticalAlignBottom, b.Model().VerticalAlign)

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetBackgroundColor{
						BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
							Id:              "textBlock",
							BackgroundColor: "pink",
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b = st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, "pink", b.Model().BackgroundColor)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateBookmark(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"bookmark"},
		}),
		"bookmark": simple.New(&model.Block{Id: "bookmark", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					Url: "http://example.com",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetBookmark{
						BlockSetBookmark: &pb.EventBlockSetBookmark{
							Id:  "bookmark",
							Url: &pb.EventBlockSetBookmarkUrl{Value: "http://example1.com"},
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("bookmark")
	assert.NotNil(t, b)
	assert.Equal(t, "http://example1.com", b.Model().GetBookmark().Url)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateTable(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"table"},
		}),
		"table": simple.New(&model.Block{
			Id:           "table",
			Restrictions: &model.BlockRestrictions{Edit: true},
			ChildrenIds:  []string{"row", "column"},
			Content: &model.BlockContentOfTable{
				Table: &model.BlockContentTable{},
			},
		}),
		"row": simple.New(&model.Block{Id: "row", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfTableRow{
				TableRow: &model.BlockContentTableRow{
					IsHeader: false,
				},
			},
		}),
		"column": simple.New(&model.Block{Id: "column", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfTableColumn{
				TableColumn: &model.BlockContentTableColumn{},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetTableRow{
						BlockSetTableRow: &pb.EventBlockSetTableRow{
							Id:       "row",
							IsHeader: &pb.EventBlockSetTableRowIsHeader{Value: true},
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("row")
	assert.NotNil(t, b)
	assert.Equal(t, true, b.Model().GetTableRow().IsHeader)

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetTableRow{
						BlockSetTableRow: &pb.EventBlockSetTableRow{
							Id:       "row",
							IsHeader: &pb.EventBlockSetTableRowIsHeader{Value: false},
						},
					}},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b = st.Get("row")
	assert.NotNil(t, b)
	assert.Equal(t, false, b.Model().GetTableRow().IsHeader)
}

func TestState_ApplyChangeIgnoreErrBlockMove(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"bookmark"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
		"textBlock1": simple.New(&model.Block{Id: "textBlock1", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text1",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockMove{
		BlockMove: &pb.ChangeBlockMove{
			TargetId: "textBlock",
			Position: model.Block_Inner,
			Ids:      []string{"textBlock1"},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, []string{"textBlock1"}, b.Model().GetChildrenIds())
}

func TestState_ApplyChangeIgnoreErrBlockUpdateDiv(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"div"},
		}),
		"div": simple.New(&model.Block{Id: "div", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfDiv{
				Div: &model.BlockContentDiv{
					Style: model.BlockContentDiv_Dots,
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetDiv{
						BlockSetDiv: &pb.EventBlockSetDiv{
							Id:    "div",
							Style: &pb.EventBlockSetDivStyle{Value: model.BlockContentDiv_Line},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("div")
	assert.NotNil(t, b)
	assert.Equal(t, model.BlockContentDiv_Line, b.Model().GetDiv().Style)

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetDiv{
						BlockSetDiv: &pb.EventBlockSetDiv{
							Id:    "div",
							Style: &pb.EventBlockSetDivStyle{Value: model.BlockContentDiv_Dots},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b = st.Get("div")
	assert.NotNil(t, b)
	assert.Equal(t, model.BlockContentDiv_Dots, b.Model().GetDiv().Style)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateText(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetText{
						BlockSetText: &pb.EventBlockSetText{
							Id:    "textBlock",
							Text:  &pb.EventBlockSetTextText{Value: "updated text"},
							Style: &pb.EventBlockSetTextStyle{Value: model.BlockContentText_Checkbox},
							Color: &pb.EventBlockSetTextColor{Value: "pink"},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, "updated text", b.Model().GetText().Text)
	assert.Equal(t, model.BlockContentText_Checkbox, b.Model().GetText().Style)
	assert.Equal(t, "pink", b.Model().GetText().Color)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateField(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"textBlock"},
		}),
		"textBlock": simple.New(&model.Block{Id: "textBlock",
			Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetFields{
						BlockSetFields: &pb.EventBlockSetFields{
							Id: "textBlock",
							Fields: &types.Struct{
								Fields: map[string]*types.Value{
									"lang": pbtypes.String("java"),
								},
							},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, "java", b.Model().GetFields().GetFields()["lang"].GetStringValue())
}

func TestState_ApplyChangeIgnoreErrBlockUpdateLink(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"file"},
		}),
		"link": simple.New(&model.Block{Id: "link",
			Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "id",
					Style:         model.BlockContentLink_Page,
					IconSize:      model.BlockContentLink_SizeMedium,
					CardStyle:     model.BlockContentLink_Card,
					Description:   model.BlockContentLink_Added,
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetLink{
						BlockSetLink: &pb.EventBlockSetLink{
							Id:            "link",
							TargetBlockId: &pb.EventBlockSetLinkTargetBlockId{Value: "newID"},
							Style:         &pb.EventBlockSetLinkStyle{Value: model.BlockContentLink_Dataview},
							CardStyle:     &pb.EventBlockSetLinkCardStyle{Value: model.BlockContentLink_Inline},
							Description:   &pb.EventBlockSetLinkDescription{Value: model.BlockContentLink_Content},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("link")
	assert.NotNil(t, b)
	assert.Equal(t, "newID", b.Model().GetLink().GetTargetBlockId())
	assert.Equal(t, model.BlockContentLink_Dataview, b.Model().GetLink().GetStyle())
	assert.Equal(t, model.BlockContentLink_Inline, b.Model().GetLink().GetCardStyle())
	assert.Equal(t, model.BlockContentLink_Content, b.Model().GetLink().GetDescription())
}

func TestState_ApplyChangeIgnoreErrBlockUpdateDataview(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"file"},
		}),
		"dataview": simple.New(&model.Block{Id: "dataview",
			Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfDataview{
				Dataview: &model.BlockContentDataview{
					Source: []string{"rel-id"},
					Views: []*model.BlockContentDataviewView{
						{
							Id:   "id",
							Type: model.BlockContentDataviewView_Kanban,
							Name: "Name",
						},
						{
							Id:   "id1",
							Type: model.BlockContentDataviewView_List,
							Name: "Name1",
						},
						{
							Id:   "id2",
							Type: model.BlockContentDataviewView_Table,
							Name: "Name2",
						},
					},
					GroupOrders: []*model.BlockContentDataviewGroupOrder{
						{
							ViewId: "id",
							ViewGroups: []*model.BlockContentDataviewViewGroup{
								{
									GroupId: "group1",
									Index:   1,
									Hidden:  true,
								},
								{
									GroupId: "group2",
									Index:   2,
									Hidden:  false,
								},
							},
						},
					},
					ObjectOrders: []*model.BlockContentDataviewObjectOrder{
						{
							ViewId:    "id",
							GroupId:   "group1",
							ObjectIds: []string{"object1", "object2"},
						},
						{
							ViewId:    "id1",
							GroupId:   "group2",
							ObjectIds: []string{"object3", "object4"},
						},
					},
					RelationLinks: []*model.RelationLink{
						{
							Key:    "relation1",
							Format: model.RelationFormat_shorttext,
						},
						{
							Key:    "relation2",
							Format: model.RelationFormat_shorttext,
						},
						{
							Key:    "relation3",
							Format: model.RelationFormat_shorttext,
						},
					},
				},
			},
		}),
	}).(*State)

	changes := []*pb.ChangeContent{{
		Value: &pb.ChangeContentValueOfBlockUpdate{
			BlockUpdate: &pb.ChangeBlockUpdate{
				Events: []*pb.EventMessage{
					{
						Value: &pb.EventMessageValueOfBlockDataviewSourceSet{
							BlockDataviewSourceSet: &pb.EventBlockDataviewSourceSet{
								Id:     "dataview",
								Source: []string{"rel-changedId"},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewViewSet{
							BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
								Id:     "dataview",
								ViewId: "id",
								View:   &model.BlockContentDataviewView{Type: model.BlockContentDataviewView_Gallery},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewViewOrder{
							BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
								Id:      "dataview",
								ViewIds: []string{"id2", "id", "id1"},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewViewDelete{
							BlockDataviewViewDelete: &pb.EventBlockDataviewViewDelete{
								Id:     "dataview",
								ViewId: "id1",
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
							BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
								Id:      "dataview",
								ViewId:  "id",
								GroupId: "group1",
								SliceChanges: []*pb.EventBlockDataviewSliceChange{
									{
										Op:      pb.EventBlockDataview_SliceOperationMove,
										Ids:     []string{"object1"},
										AfterId: "object2",
									},
								},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewRelationSet{
							BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
								Id: "dataview",
								RelationLinks: []*model.RelationLink{
									{
										Key:    "relation4",
										Format: model.RelationFormat_longtext,
									},
								},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
							BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
								Id:           "dataview",
								RelationKeys: []string{"relation1", "relation2"},
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
							BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
								Id:             "dataview",
								TargetObjectId: "newTargetID",
							},
						},
					},
					{
						Value: &pb.EventMessageValueOfBlockDataviewIsCollectionSet{
							BlockDataviewIsCollectionSet: &pb.EventBlockDataviewIsCollectionSet{
								Id:    "dataview",
								Value: true,
							},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(changes...)
	b := st.Get("dataview")
	assert.NotNil(t, b)
	assert.Equal(t, []string{"rel-changedId"}, b.Model().GetDataview().Source)
	assert.Equal(t, []string{"object2", "object1"}, b.Model().GetDataview().ObjectOrders[0].ObjectIds)
	assert.Equal(t, "newTargetID", b.Model().GetDataview().TargetObjectId)
	assert.Equal(t, true, b.Model().GetDataview().IsCollection)
	assert.Len(t, b.Model().GetDataview().RelationLinks, 2)
	assert.Equal(t, "relation3", b.Model().GetDataview().RelationLinks[0].Key)
	assert.Equal(t, "relation4", b.Model().GetDataview().RelationLinks[1].Key)
	assert.Len(t, b.Model().GetDataview().Views, 2)
	assert.Equal(t, "id2", b.Model().GetDataview().Views[0].Id)
	assert.Equal(t, "id", b.Model().GetDataview().Views[1].Id)
	assert.Equal(t, model.BlockContentDataviewView_Gallery, b.Model().GetDataview().Views[1].Type)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetLatex(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"latex"},
		}),
		"latex": simple.New(&model.Block{Id: "latex", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfLatex{
				Latex: &model.BlockContentLatex{
					Text: "text",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetLatex{
						BlockSetLatex: &pb.EventBlockSetLatex{

							Id:   "latex",
							Text: &pb.EventBlockSetLatexText{Value: "new text"},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("latex")
	assert.NotNil(t, b)
	assert.Equal(t, "new text", b.Model().GetLatex().Text)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetRelations(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"latex"},
		}),
		"relation": simple.New(&model.Block{Id: "relation", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{
					Key: "relationKey",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetRelation{
						BlockSetRelation: &pb.EventBlockSetRelation{
							Id:  "relation",
							Key: &pb.EventBlockSetRelationKey{Value: "newRelationKey"},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("relation")
	assert.NotNil(t, b)
	assert.Equal(t, "newRelationKey", b.Model().GetRelation().Key)
}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetWidget(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"latex"},
		}),
		"widget": simple.New(&model.Block{Id: "widget", Restrictions: &model.BlockRestrictions{Edit: true},
			Content: &model.BlockContentOfWidget{
				Widget: &model.BlockContentWidget{
					Layout: model.BlockContentWidget_List,
					Limit:  10,
					ViewId: "id",
				},
			},
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
		BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetWidget{
						BlockSetWidget: &pb.EventBlockSetWidget{
							Id:     "widget",
							Layout: &pb.EventBlockSetWidgetLayout{Value: model.BlockContentWidget_Tree},
							Limit:  &pb.EventBlockSetWidgetLimit{Value: 20},
							ViewId: &pb.EventBlockSetWidgetViewId{Value: "newID"},
						},
					},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	b := st.Get("widget")
	assert.NotNil(t, b)
	assert.Equal(t, model.BlockContentWidget_Tree, b.Model().GetWidget().Layout)
	assert.Equal(t, int32(20), b.Model().GetWidget().Limit)
	assert.Equal(t, "newID", b.Model().GetWidget().ViewId)
}

func TestState_ApplyChangeIgnoreErrDetailsSet(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	st.SetDetail("relationKey", pbtypes.String("value"))

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
		DetailsSet: &pb.ChangeDetailsSet{
			Key:   "relationKey",
			Value: pbtypes.String("changed value"),
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, "changed value", st.Details().GetFields()["relationKey"].GetStringValue())

	st.details = nil
	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
		DetailsSet: &pb.ChangeDetailsSet{
			Key:   "relationKey",
			Value: pbtypes.String("value"),
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, "value", st.Details().GetFields()["relationKey"].GetStringValue())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
		DetailsSet: &pb.ChangeDetailsSet{
			Key:   "relationKey",
			Value: nil,
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Details().GetFields()["relationKey"])
}

func TestState_ApplyChangeIgnoreErrDetailsUnset(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsUnset{
		DetailsUnset: &pb.ChangeDetailsUnset{
			Key: "relationKey",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Details().GetFields()["relationKey"])

	st.SetDetail("relationKey", pbtypes.String("value"))
	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsUnset{
		DetailsUnset: &pb.ChangeDetailsUnset{
			Key: "relationKey",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Details().GetFields()["relationKey"])
}

func TestState_ApplyChangeIgnoreErrRelationAdd(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfRelationAdd{
		RelationAdd: &pb.ChangeRelationAdd{
			RelationLinks: []*model.RelationLink{
				{
					Key:    "relation1",
					Format: model.RelationFormat_longtext,
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Contains(t, st.GetRelationLinks(), &model.RelationLink{Key: "relation1", Format: model.RelationFormat_longtext})

	st.ApplyChangeIgnoreErr(change)
	assert.Contains(t, st.GetRelationLinks(), &model.RelationLink{Key: "relation1", Format: model.RelationFormat_longtext})
}

func TestState_ApplyChangeIgnoreErrRelationRemove(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	st.AddRelationLinks([]*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_longtext,
		},
		{
			Key:    "relation2",
			Format: model.RelationFormat_shorttext,
		},
	}...)
	originLength := len(st.GetRelationLinks())
	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfRelationRemove{
		RelationRemove: &pb.ChangeRelationRemove{
			RelationKey: []string{"relation1", "relation2"},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Len(t, st.GetRelationLinks(), originLength-2)
}

func TestState_ApplyChangeIgnoreErrObjectTypeAdd(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
		ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
			Url: "ot-page",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, "ot-page", st.ObjectType())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
		ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
			Url: "ot-note",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, []string{"ot-page", "ot-note"}, st.ObjectTypes())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
		ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
			Url: "ot-note",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, []string{"ot-page", "ot-note"}, st.ObjectTypes())
}

func TestState_ApplyChangeIgnoreErrObjectTypeRemove(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)
	st.objectTypes = append(st.objectTypes, "ot-page")

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeRemove{
		ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
			Url: "ot-page",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Empty(t, st.ObjectTypes())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeRemove{
		ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
			Url: "ot-page",
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Empty(t, st.ObjectTypes())
}

func TestState_ApplyChangeIgnoreErrStoreKeySet(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
		StoreKeySet: &pb.ChangeStoreKeySet{
			Path:  []string{"objects"},
			Value: pbtypes.String("value"),
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, "value", st.Store().GetFields()["objects"].GetStringValue())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
		StoreKeySet: &pb.ChangeStoreKeySet{
			Path:  []string{"objects"},
			Value: pbtypes.String("newvalue"),
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Equal(t, "newvalue", st.Store().GetFields()["objects"].GetStringValue())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
		StoreKeySet: &pb.ChangeStoreKeySet{
			Path:  []string{"objects"},
			Value: nil,
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Store().GetFields()["objects"])
}

func TestState_ApplyChangeIgnoreErrStoreKeyUnset(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	st.SetInStore([]string{"objects"}, pbtypes.Struct(&types.Struct{
		Fields: map[string]*types.Value{
			"id":   pbtypes.String("id"),
			"name": pbtypes.String("name"),
		},
	}))

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeyUnset{
		StoreKeyUnset: &pb.ChangeStoreKeyUnset{
			Path: []string{"objects"},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Store().GetFields()["objects"])

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeyUnset{
		StoreKeyUnset: &pb.ChangeStoreKeyUnset{
			Path: []string{"objects"},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.Nil(t, st.Store().GetFields()["objects"])
}

func TestState_ApplyChangeIgnoreErrSliceUpdate(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreSliceUpdate{
		StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
			Key: "objects",
			Operation: &pb.ChangeStoreSliceUpdateOperationOfAdd{
				Add: &pb.ChangeStoreSliceUpdateAdd{
					Ids: []string{"id", "id1"},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.NotNil(t, st.Store().GetFields()["objects"])
	assert.Len(t, st.Store().GetFields()["objects"].GetListValue().Values, 2)
	assert.Equal(t, "id", st.Store().GetFields()["objects"].GetListValue().Values[0].GetStringValue())
	assert.Equal(t, "id1", st.Store().GetFields()["objects"].GetListValue().Values[1].GetStringValue())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreSliceUpdate{
		StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
			Key: "objects",
			Operation: &pb.ChangeStoreSliceUpdateOperationOfMove{
				Move: &pb.ChangeStoreSliceUpdateMove{
					AfterId: "id1",
					Ids:     []string{"id"},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.NotNil(t, st.Store().GetFields()["objects"])
	assert.Len(t, st.Store().GetFields()["objects"].GetListValue().Values, 2)
	assert.Equal(t, "id1", st.Store().GetFields()["objects"].GetListValue().Values[0].GetStringValue())
	assert.Equal(t, "id", st.Store().GetFields()["objects"].GetListValue().Values[1].GetStringValue())

	change = &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreSliceUpdate{
		StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
			Key: "objects",
			Operation: &pb.ChangeStoreSliceUpdateOperationOfRemove{
				Remove: &pb.ChangeStoreSliceUpdateRemove{
					Ids: []string{"id"},
				},
			},
		},
	}}
	st.ApplyChangeIgnoreErr(change)
	assert.NotNil(t, st.Store().GetFields()["objects"])
	assert.Len(t, st.Store().GetFields()["objects"].GetListValue().Values, 1)
	assert.Equal(t, "id1", st.Store().GetFields()["objects"].GetListValue().Values[0].GetStringValue())
}
