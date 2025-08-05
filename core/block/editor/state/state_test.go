package state

import (
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	text2 "github.com/anyproto/anytype-heart/util/text"
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

func TestState_Snippet(t *testing.T) {
	t.Run("snippet cut - when the content is too long", func(t *testing.T) {
		givenState := buildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					strings.Repeat("a", 301),
					blockbuilder.ID("1"),
				),
			)))

		// when
		snippet := givenState.NewState().Snippet()

		// then

		assert.Equal(t, 300, text2.UTF16RuneCountString(snippet))
		assert.Equal(t, 300, len(strings.Repeat("a", 300)))
	})

	t.Run("snippet empty - when the style is title or description", func(t *testing.T) {
		givenState := buildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"some text 1",
					blockbuilder.ID("1"),
					blockbuilder.TextStyle(model.BlockContentText_Title),
				),
				blockbuilder.Text(
					"some text 2",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Description),
				),
			)))

		// when
		snippet := givenState.NewState().Snippet()

		// then
		assert.Equal(t, "", snippet)
	})

	t.Run("snippet empty - when the style is title or description", func(t *testing.T) {
		givenState := buildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					" text 1 ",
					blockbuilder.ID("1"),
				),
				blockbuilder.Text(
					" text 2 ",
					blockbuilder.ID("2"),
				),
			)))

		// when
		snippet := givenState.NewState().Snippet()

		// then
		assert.Equal(t, "text 1\ntext 2", snippet)
	})
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

		msgs, hist, err := ApplyState("", s, true)
		require.NoError(t, err)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Len(t, hist.Remove, 0)
		require.Len(t, msgs, 2)
	})
	t.Run("details handle", func(t *testing.T) {
		t.Run("init new text-of-detail bloc", func(t *testing.T) {
			d := NewDoc("1", map[string]simple.Block{
				"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{}}),
			})
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				"name": domain.String("new name"),
			}))
			s.Add(text.NewDetails(&model.Block{
				Id: "title",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{},
				},
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						text.DetailsKeyFieldName: pbtypes.String("name"),
					},
				},
			}, text.DetailsKeys{
				Text:    "name",
				Checked: "done",
			}))
			_, _, err := ApplyState("", s, true)
			assert.Equal(t, "new name", s.Pick("title").Model().GetText().Text)
			require.NoError(t, err)
		})
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
			s.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				"name": domain.String("new name"),
			}))
			msgs, _, err := ApplyState("", s, true)
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
			d.(*State).SetDetail("done", domain.Bool(true))
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				"done": domain.Bool(false),
			}))
			msgs, _, err := ApplyState("", s, true)
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
			d.(*State).SetDetail("done", domain.Bool(true))
			d.BlocksInit(d.(simple.DetailsService))
			s := d.NewState()
			s.Get("2").(text.Block).SetChecked(false)
			msgs, _, err := ApplyState("", s, true)
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
	t.Run("state doesn't have settings store", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when do nothing
		// then
		assert.Nil(t, st.GetSetting("setting"))
	})

	t.Run("settings are empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{SettingsStoreKey}, nil)
		// then
		assert.Nil(t, st.GetSetting("setting"))
	})

	t.Run("settings are not empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		settings := pbtypes.Struct(&types.Struct{Fields: map[string]*types.Value{"setting": pbtypes.String("setting")}})
		st.SetInStore([]string{SettingsStoreKey}, settings)
		// then
		assert.NotNil(t, st.GetSetting("setting"))
		assert.Equal(t, settings.GetStructValue().GetFields()["setting"], st.GetSetting("setting"))
	})
}

func TestState_GetStoreSlice(t *testing.T) {
	t.Run("state store is empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when do nothing
		// then
		assert.Nil(t, st.GetStoreSlice("collection"))
	})
	t.Run("collection store is empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{"collection"}, nil)
		// then
		assert.Nil(t, st.GetStoreSlice("collection"))
	})
	t.Run("add objects to collection store", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{"collection"}, pbtypes.StringList([]string{"object1", "object2"}))
		// then
		assert.NotNil(t, st.GetStoreSlice("collection"))
		assert.Equal(t, []string{"object1", "object2"}, st.GetStoreSlice("collection"))
	})
}

func TestState_GetSubObjectCollection(t *testing.T) {
	const collectionName = "subcollection"
	t.Run("state store is empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when do nothing
		// then
		assert.Nil(t, st.GetSubObjectCollection(collectionName))
	})
	t.Run("sub object collection is empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{collectionName}, nil)
		// then
		assert.Nil(t, st.GetSubObjectCollection(collectionName))
	})
	t.Run("add sub objects", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		subObjectDetails := pbtypes.Struct(
			&types.Struct{
				Fields: map[string]*types.Value{
					"subobject1": pbtypes.Struct(nil),
					"subobject2": pbtypes.Struct(nil),
				},
			})
		// when
		st.SetInStore([]string{collectionName}, subObjectDetails)
		// then
		assert.NotNil(t, st.GetSubObjectCollection(collectionName))
		assert.Equal(t, subObjectDetails.GetStructValue(), st.GetSubObjectCollection(collectionName))
	})
}

func TestState_ContainsInStore(t *testing.T) {
	const collectionName = "subcollection"
	t.Run("state store is empty", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when do nothing
		// then
		assert.False(t, st.ContainsInStore([]string{collectionName, "subobject1"}))
	})

	t.Run("subcollection store doesn't contain given subobject", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{collectionName}, nil)
		// then
		assert.False(t, st.ContainsInStore([]string{collectionName, "subobject2"}))
	})

	t.Run("subcollection store contains given subobjects", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
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
		// when
		st.SetInStore([]string{collectionName}, subObjectDetails)
		// then
		assert.True(t, st.ContainsInStore([]string{collectionName, "subobject1"}))
		assert.True(t, st.ContainsInStore([]string{collectionName, "subobject2"}))
		// nested
		assert.False(t, st.ContainsInStore([]string{collectionName, "subobject3"}))
		assert.False(t, st.ContainsInStore([]string{collectionName, "subobject1", "subobject3"}))
		assert.True(t, st.ContainsInStore([]string{collectionName, "subobject2", "subobject3"}))
	})
}

func TestState_HasInStore(t *testing.T) {
	const collectionName = "subcollection"
	t.Run("state has empty store", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when do nothing
		// then
		assert.False(t, st.HasInStore([]string{collectionName, "subobject1"}))
	})
	t.Run("subcollection store doesn't have subobjects", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
		// when
		st.SetInStore([]string{collectionName}, nil)
		// then
		assert.False(t, st.HasInStore([]string{collectionName, "subobject2"}))
	})
	t.Run("subcollection store has given subobjects", func(t *testing.T) {
		// given
		st := NewDoc("test", nil).(*State)
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
		// when
		st.SetInStore([]string{collectionName}, subObjectDetails)
		// then
		assert.True(t, st.HasInStore([]string{collectionName, "subobject1"}))
		assert.True(t, st.HasInStore([]string{collectionName, "subobject2"}))
		// nested
		assert.False(t, st.HasInStore([]string{collectionName, "subobject3"}))
		assert.False(t, st.HasInStore([]string{collectionName, "subobject1", "subobject3"}))
		assert.True(t, st.HasInStore([]string{collectionName, "subobject2", "subobject3"}))
	})
}

func TestState_Validate(t *testing.T) {
	t.Run("validate valid state", func(t *testing.T) {
		// given
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
		// when
		err := s.Validate()
		// then
		assert.Nil(t, err)
	})
	t.Run("validate not valid state, which has block with two parents", func(t *testing.T) {
		// given
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
		// when
		err := childrenWithTwoParents.Validate()
		// then
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "two children with same id")
	})
	t.Run("validate not valid state with missed children blocks", func(t *testing.T) {
		// given
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
		// when
		err := missedChildBlockState.Validate()
		// then
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "missed block")
	})
}

func TestState_CheckRestrictions(t *testing.T) {
	t.Run("state doesn't have parent state", func(t *testing.T) {
		// given
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
		// when do nothing
		// then
		assert.Nil(t, st.CheckRestrictions())
	})
	t.Run("state has parent state without restrictions", func(t *testing.T) {
		// given
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
		// when
		parentState := NewDoc("root", map[string]simple.Block{
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
		// then
		assert.Nil(t, st.CheckRestrictions()) // no restrictions
	})
}

func TestState_CheckRestrictionsBlockHasRestriction(t *testing.T) {
	t.Run("state has restriction in block, but without changes in block", func(t *testing.T) {
		// given
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

		// when
		st.SetParent(parentState)
		// then
		assert.Nil(t, st.CheckRestrictions()) // no changes
	})
	t.Run("state has restriction in block, block was edited", func(t *testing.T) {
		// given
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
						Text: "parentText",
					},
				},
			})}).(*State)

		// when
		st.SetParent(parentState)

		// then
		assert.NotNil(t, st.CheckRestrictions())
		assert.True(t, errors.Is(st.CheckRestrictions(), ErrRestricted))
	})
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

	t.Run("apply BlockCreate change: new block created", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("newTextBlock")
		assert.NotNil(t, b)
		assert.NotNil(t, b.Model().GetText())
		assert.Equal(t, "new text", b.Model().GetText().GetText())
		assert.Len(t, st.blocks, 3)

	})

	t.Run("apply BlockCreate change: skip root block", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Len(t, st.blocks, 3) // root block not added
	})

	t.Run("apply BlockCreate change: add dataview block", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockCreate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.NotNil(t, b.Model().GetDataview())
		assert.Len(t, st.blocks, 4)
	})
}

func TestState_ApplyChangeIgnoreErrBlockRemove(t *testing.T) {
	t.Run("apply BlockRemove change: remove block", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("textBlock")
		assert.Nil(t, b)
		assert.Len(t, st.Blocks(), 1)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateBase(t *testing.T) {
	// given
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

	t.Run("apply BlockUpdate change: change align to Right", func(t *testing.T) {
		// given
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
		// when
		st.ApplyChangeIgnoreErr(change)
		// then
		b := st.Get("textBlock")
		assert.NotNil(t, b)
		assert.Equal(t, model.Block_AlignRight, b.Model().Align)
	})

	t.Run("apply BlockUpdate change: change vertical align to Bottom", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
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
		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("textBlock")
		assert.NotNil(t, b)
		assert.Equal(t, model.Block_VerticalAlignBottom, b.Model().VerticalAlign)
	})

	t.Run("apply BlockUpdate change: change background color", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("textBlock")
		assert.NotNil(t, b)
		assert.Equal(t, "pink", b.Model().BackgroundColor)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateBookmarkUrl(t *testing.T) {
	t.Run("apply BlockUpdate change: change bookmark url", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("bookmark")
		assert.NotNil(t, b)
		assert.Equal(t, "http://example1.com", b.Model().GetBookmark().Url)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateTable(t *testing.T) {
	// given
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

	t.Run("apply BlockUpdate change: make table row as header", func(t *testing.T) {
		// given
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
		// when
		st.ApplyChangeIgnoreErr(change)
		// then
		b := st.Get("row")
		assert.NotNil(t, b)
		assert.Equal(t, true, b.Model().GetTableRow().IsHeader)
	})

	t.Run("apply BlockUpdate change: unset table header", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("row")
		assert.NotNil(t, b)
		assert.Equal(t, false, b.Model().GetTableRow().IsHeader)
	})
}

func TestState_ApplyChangeIgnoreErrBlockMove(t *testing.T) {
	// given
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

	// when
	st.ApplyChangeIgnoreErr(change)

	// then
	b := st.Get("textBlock")
	assert.NotNil(t, b)
	assert.Equal(t, []string{"textBlock1"}, b.Model().GetChildrenIds())
}

func TestState_ApplyChangeIgnoreErrBlockUpdateDiv(t *testing.T) {
	// given
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

	t.Run("apply BlockUpdate change: update div style to Line", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("div")
		assert.NotNil(t, b)
		assert.Equal(t, model.BlockContentDiv_Line, b.Model().GetDiv().Style)
	})

	t.Run("apply BlockUpdate change: update div style to Dots", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfBlockUpdate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("div")
		assert.NotNil(t, b)
		assert.Equal(t, model.BlockContentDiv_Dots, b.Model().GetDiv().Style)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateTextParams(t *testing.T) {
	t.Run("apply BlockUpdate change: update text block", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("textBlock")
		assert.NotNil(t, b)
		assert.Equal(t, "updated text", b.Model().GetText().Text)
		assert.Equal(t, model.BlockContentText_Checkbox, b.Model().GetText().Style)
		assert.Equal(t, "pink", b.Model().GetText().Color)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateField(t *testing.T) {
	t.Run("apply BlockUpdate change: update block fields", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("textBlock")
		assert.NotNil(t, b)
		assert.Equal(t, "java", b.Model().GetFields().GetFields()["lang"].GetStringValue())
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateLinkParams(t *testing.T) {
	t.Run("apply BlockUpdate change: update link block", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("link")
		assert.NotNil(t, b)
		assert.Equal(t, "newID", b.Model().GetLink().GetTargetBlockId())
		assert.Equal(t, model.BlockContentLink_Dataview, b.Model().GetLink().GetStyle())
		assert.Equal(t, model.BlockContentLink_Inline, b.Model().GetLink().GetCardStyle())
		assert.Equal(t, model.BlockContentLink_Content, b.Model().GetLink().GetDescription())
	})
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

	t.Run("apply BlockUpdate change: update source in dataview", func(t *testing.T) {
		// given
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
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, []string{"rel-changedId"}, b.Model().GetDataview().Source)

	})
	t.Run("apply BlockUpdate change: update dataview style to Gallery", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfBlockDataviewViewSet{
								BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
									Id:     "dataview",
									ViewId: "id",
									View:   &model.BlockContentDataviewView{Type: model.BlockContentDataviewView_Gallery},
								},
							},
						},
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, model.BlockContentDataviewView_Gallery, b.Model().GetDataview().Views[0].Type)
	})
	t.Run("apply BlockUpdate change: update dataview order of views", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfBlockDataviewViewOrder{
								BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
									Id:      "dataview",
									ViewIds: []string{"id2", "id", "id1"},
								},
							},
						},
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, "id2", b.Model().GetDataview().Views[0].Id)
		assert.Equal(t, "id", b.Model().GetDataview().Views[1].Id)
	})
	t.Run("apply BlockUpdate change: remove view with id1", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfBlockDataviewViewDelete{
								BlockDataviewViewDelete: &pb.EventBlockDataviewViewDelete{
									Id:     "dataview",
									ViewId: "id1",
								},
							},
						},
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Len(t, b.Model().GetDataview().Views, 2)
	})

	t.Run("apply BlockUpdate change: update object order in view", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
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
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, []string{"object2", "object1"}, b.Model().GetDataview().ObjectOrders[0].ObjectIds)
	})

	t.Run("apply BlockUpdate change: remove relations from dataview", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
								BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
									Id:           "dataview",
									RelationKeys: []string{"relation1", "relation2"},
								},
							},
						},
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Len(t, b.Model().GetDataview().RelationLinks, 1)

	})

	t.Run("apply BlockUpdate change: add relation to dataview", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
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
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Len(t, b.Model().GetDataview().RelationLinks, 2)
		assert.Equal(t, "relation3", b.Model().GetDataview().RelationLinks[0].Key)
		assert.Equal(t, "relation4", b.Model().GetDataview().RelationLinks[1].Key)
	})

	t.Run("apply BlockUpdate change: change target object id", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
								BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
									Id:             "dataview",
									TargetObjectId: "newTargetID",
								},
							},
						},
					},
				},
			},
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, "newTargetID", b.Model().GetDataview().TargetObjectId)

	})

	t.Run("apply BlockUpdate change: make collection", func(t *testing.T) {
		// given
		changes := []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{
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
		},
		}

		// when
		st.ApplyChangeIgnoreErr(changes...)

		// then
		b := st.Get("dataview")
		assert.NotNil(t, b)
		assert.Equal(t, true, b.Model().GetDataview().IsCollection)
	})

}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetLatex(t *testing.T) {
	t.Run("apply BlockUpdate change: change embed text", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"embed"},
			}),
			"embed": simple.New(&model.Block{Id: "embed", Restrictions: &model.BlockRestrictions{Edit: true},
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

								Id:   "embed",
								Text: &pb.EventBlockSetLatexText{Value: "new text"},
							},
						},
					},
				},
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("embed")
		assert.NotNil(t, b)
		assert.Equal(t, "new text", b.Model().GetLatex().Text)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetRelations(t *testing.T) {
	t.Run("apply BlockUpdate change: change relation in relation block", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"embed"},
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("relation")
		assert.NotNil(t, b)
		assert.Equal(t, "newRelationKey", b.Model().GetRelation().Key)
	})
}

func TestState_ApplyChangeIgnoreErrBlockUpdateSetWidget(t *testing.T) {
	t.Run("apply BlockUpdate change: update widget parameters (layout, limit, viewID)", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Id:          "root",
				ChildrenIds: []string{"embed"},
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		b := st.Get("widget")
		assert.NotNil(t, b)
		assert.Equal(t, model.BlockContentWidget_Tree, b.Model().GetWidget().Layout)
		assert.Equal(t, int32(20), b.Model().GetWidget().Limit)
		assert.Equal(t, "newID", b.Model().GetWidget().ViewId)
	})
}

func TestState_ApplyChangeIgnoreErrDetailsSet(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply DetailsSet change: add new detail", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
			DetailsSet: &pb.ChangeDetailsSet{
				Key:   "relationKey",
				Value: pbtypes.String("changed value"),
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, "changed value", st.Details().GetString("relationKey"))
	})

	t.Run("apply DetailsSet change: update existing relation", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
			DetailsSet: &pb.ChangeDetailsSet{
				Key:   "relationKey",
				Value: pbtypes.String("value"),
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, "value", st.Details().GetString("relationKey"))
	})

	t.Run("apply DetailsSet change: set relation value to nil", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsSet{
			DetailsSet: &pb.ChangeDetailsSet{
				Key:   "relationKey",
				Value: nil,
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.False(t, st.Details().Has("relationKey"))
	})
}

func TestState_ApplyChangeIgnoreErrDetailsUnset(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply DetailsUnset change: remove non existing relation", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsUnset{
			DetailsUnset: &pb.ChangeDetailsUnset{
				Key: "relationKey",
			},
		}}

		// apply
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.False(t, st.Details().Has("relationKey"))
	})

	t.Run("apply DetailsUnset change: remove existing relation", func(t *testing.T) {
		// given
		st.SetDetail("relationKey", domain.String("value"))
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfDetailsUnset{
			DetailsUnset: &pb.ChangeDetailsUnset{
				Key: "relationKey",
			},
		}}

		// apply
		st.ApplyChangeIgnoreErr(change)

		// when
		assert.False(t, st.Details().Has("relationKey"))
	})
}

func TestState_ApplyChangeIgnoreErrRelationAdd(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply RelationAdd change: add new relation", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Contains(t, st.GetRelationLinks(), &model.RelationLink{Key: "relation1", Format: model.RelationFormat_longtext})
	})

	t.Run("apply RelationAdd change: add already existing relation - no changes", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Contains(t, st.GetRelationLinks(), &model.RelationLink{Key: "relation1", Format: model.RelationFormat_longtext})
	})
}

func TestState_ApplyChangeIgnoreErrRelationRemove(t *testing.T) {
	t.Run("apply RelationRemove change: remove relations", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Len(t, st.GetRelationLinks(), originLength-2)
	})
}

func TestState_ApplyChangeIgnoreErrObjectTypeAdd(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply ObjectTypeAdd change: add new object type", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
			ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
				Url: "ot-page",
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, domain.TypeKey("page"), st.ObjectTypeKey())
	})

	t.Run("apply ObjectTypeAdd change: add another object type", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
			ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
				Url: "ot-note",
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// apply
		assert.Equal(t, []domain.TypeKey{"page", "note"}, st.ObjectTypeKeys())
	})

	t.Run("apply ObjectTypeAdd change: add existing object type - no changes", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeAdd{
			ObjectTypeAdd: &pb.ChangeObjectTypeAdd{
				Url: "ot-note",
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, []domain.TypeKey{"page", "note"}, st.ObjectTypeKeys())
	})
}

func TestState_ApplyChangeIgnoreErrObjectTypeRemove(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)
	st.objectTypeKeys = append(st.objectTypeKeys, "page")

	t.Run("apply ObjectTypeRemove change: remove existing object type", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeRemove{
			ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
				Url: "ot-page",
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Empty(t, st.ObjectTypeKeys())
	})

	t.Run("apply ObjectTypeRemove change: remove non existing object type", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfObjectTypeRemove{
			ObjectTypeRemove: &pb.ChangeObjectTypeRemove{
				Url: "ot-page",
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Empty(t, st.ObjectTypeKeys())
	})
}

func TestState_ApplyChangeIgnoreErrStoreKeySet(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply StoreKeySet change: set new value in store", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
			StoreKeySet: &pb.ChangeStoreKeySet{
				Path:  []string{"objects"},
				Value: pbtypes.String("value"),
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, "value", st.Store().GetFields()["objects"].GetStringValue())
	})

	t.Run("apply StoreKeySet change: update existing value in store", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
			StoreKeySet: &pb.ChangeStoreKeySet{
				Path:  []string{"objects"},
				Value: pbtypes.String("newvalue"),
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Equal(t, "newvalue", st.Store().GetFields()["objects"].GetStringValue())
	})

	t.Run("apply StoreKeySet change: set existing value to nil", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeySet{
			StoreKeySet: &pb.ChangeStoreKeySet{
				Path:  []string{"objects"},
				Value: nil,
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Nil(t, st.Store().GetFields()["objects"])
	})
}

func TestState_ApplyChangeIgnoreErrStoreKeyUnset(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply StoreKeyUnset change: remove existing value from store", func(t *testing.T) {
		// given
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Nil(t, st.Store().GetFields()["objects"])
	})

	t.Run("apply StoreKeyUnset change: remove non existing value from store", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreKeyUnset{
			StoreKeyUnset: &pb.ChangeStoreKeyUnset{
				Path: []string{"objects"},
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.Nil(t, st.Store().GetFields()["objects"])
	})
}

func TestState_ApplyChangeIgnoreErrSliceUpdate(t *testing.T) {
	st := NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id: "root",
		}),
	}).(*State)

	t.Run("apply SliceUpdate change: add new objects to store", func(t *testing.T) {
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
	})

	t.Run("apply SliceUpdate change: move object in store to another position", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreSliceUpdate{
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

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.NotNil(t, st.Store().GetFields()["objects"])
		assert.Len(t, st.Store().GetFields()["objects"].GetListValue().Values, 2)
		assert.Equal(t, "id1", st.Store().GetFields()["objects"].GetListValue().Values[0].GetStringValue())
		assert.Equal(t, "id", st.Store().GetFields()["objects"].GetListValue().Values[1].GetStringValue())
	})

	t.Run("apply SliceUpdate change: remove object from store", func(t *testing.T) {
		// given
		change := &pb.ChangeContent{Value: &pb.ChangeContentValueOfStoreSliceUpdate{
			StoreSliceUpdate: &pb.ChangeStoreSliceUpdate{
				Key: "objects",
				Operation: &pb.ChangeStoreSliceUpdateOperationOfRemove{
					Remove: &pb.ChangeStoreSliceUpdateRemove{
						Ids: []string{"id"},
					},
				},
			},
		}}

		// when
		st.ApplyChangeIgnoreErr(change)

		// then
		assert.NotNil(t, st.Store().GetFields()["objects"])
		assert.Len(t, st.Store().GetFields()["objects"].GetListValue().Values, 1)
		assert.Equal(t, "id1", st.Store().GetFields()["objects"].GetListValue().Values[0].GetStringValue())
	})
}

func TestState_RootId(t *testing.T) {
	t.Run("root id - when set", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)

		// when
		id := st.RootId()

		// then
		assert.Equal(t, "root", id)
	})

	t.Run("root id - when is not set", func(t *testing.T) {
		// given
		blocks := blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"text 1",
					blockbuilder.ID("1"),
					blockbuilder.Children(
						blockbuilder.Text(
							"text 2",
							blockbuilder.ID("2"),
						),
					),
				),
			)).Build()

		st := NewDoc("", map[string]simple.Block{
			"1":    simple.New(blocks[1]),
			"2":    simple.New(blocks[2]),
			"root": simple.New(blocks[0]),
		}).(*State)

		// when
		id := st.RootId()

		// then
		assert.Equal(t, "root", id)
	})

}

// TODO: GO-2062 Need to review tests after details shortening refactor
// func Test_ShortenDetailsToLimit(t *testing.T) {
//	t.Run("SetDetails", func(t *testing.T) {
//		//given
//		s := &State{rootId: "first"}
//		detail := pbtypes.StringList([]string{"hello", "world", strings.Repeat("a", detailSizeLimit-9)})
//
//		//when
//		s.SetDetails(&types.Struct{Fields: map[string]*types.Value{
//			"key": pbtypes.CopyVal(detail),
//		}})
//
//		//then
//		assert.Greater(t, detail.Size(), detailSizeLimit)
//		assert.True(t, assertAllDetailsLessThenLimit(s.CombinedDetails()))
//	})
//
//	t.Run("SetDetail", func(t *testing.T) {
//		//given
//		s := &State{rootId: "first"}
//		detail := pbtypes.StringList([]string{"hello", "world", strings.Repeat("a", detailSizeLimit-9)})
//
//		//when
//		s.SetDetail(bundle.RelationKeyType, pbtypes.CopyVal(detail))
//
//		//then
//		assert.Greater(t, detail.Size(), detailSizeLimit)
//		assert.True(t, assertAllDetailsLessThenLimit(s.CombinedDetails()))
//	})
// }

func TestState_AddDevice(t *testing.T) {
	t.Run("add device", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)

		// when
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})

		// then
		assert.NotNil(t, st.deviceStore["id"])
	})
	t.Run("add device - device exist", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})
		newState := st.NewState()

		// when
		newState.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test1",
		})

		// then
		assert.NotNil(t, st.deviceStore["id"])
		assert.Equal(t, "test", st.deviceStore["id"].Name)
	})
}

func TestState_GetDevice(t *testing.T) {
	t.Run("get device, device not exist", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)

		// when
		device := st.GetDevice("id")

		// then
		assert.Nil(t, device)
	})
	t.Run("add device - device exist", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})

		// when
		device := st.GetDevice("id")

		// then
		assert.NotNil(t, device)
	})
	t.Run("add device - device with given id not exist", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})

		// when
		device := st.GetDevice("id1")

		// then
		assert.Nil(t, device)
	})
}

func TestState_ListDevices(t *testing.T) {
	t.Run("list devices, no devices", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)

		// when
		devices := st.ListDevices()

		// then
		assert.Empty(t, devices)
	})
	t.Run("list devices", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})

		// when
		devices := st.ListDevices()

		// then
		assert.Len(t, devices, 1)
	})
}

func TestState_SetDeviceName(t *testing.T) {
	t.Run("set device name, device not exist", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)

		// when
		st.SetDeviceName("id", "test")

		// then
		assert.NotNil(t, st.deviceStore["id"])
		assert.Equal(t, st.deviceStore["id"].Name, "test")
	})

	t.Run("set device name, device exists", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).(*State)
		st.AddDevice(&model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})

		newState := st.NewState()
		// when
		newState.SetDeviceName("id", "test1")

		// then
		assert.NotNil(t, newState.deviceStore["id"])
		assert.Equal(t, newState.deviceStore["id"].Name, "test1")
	})
}

func TestAddBundledRealtionLinks(t *testing.T) {
	t.Run("with relationLinks in state", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			st := &State{
				relationLinks: []*model.RelationLink{},
			}
			st.AddBundledRelationLinks(bundle.RelationKeyName, bundle.RelationKeyIconOption)

			want := &State{
				relationLinks: []*model.RelationLink{
					{
						Key:    bundle.RelationKeyName.String(),
						Format: model.RelationFormat_shorttext,
					},
					{
						Key:    bundle.RelationKeyIconOption.String(),
						Format: model.RelationFormat_number,
					},
				},
			}

			assert.Equal(t, want, st)
		})
		t.Run("one already exists, one not", func(t *testing.T) {
			st := &State{
				relationLinks: []*model.RelationLink{
					{
						Key:    bundle.RelationKeyName.String(),
						Format: model.RelationFormat_shorttext,
					},
				},
			}
			st.AddBundledRelationLinks(bundle.RelationKeyName, bundle.RelationKeyIconOption)

			want := &State{
				relationLinks: []*model.RelationLink{
					{
						Key:    bundle.RelationKeyName.String(),
						Format: model.RelationFormat_shorttext,
					},
					{
						Key:    bundle.RelationKeyIconOption.String(),
						Format: model.RelationFormat_number,
					},
				},
			}

			assert.Equal(t, want, st)
		})
	})
	t.Run("with relationLinks only in parent state", func(t *testing.T) {
		st := &State{
			relationLinks: nil,
			parent: &State{
				relationLinks: []*model.RelationLink{
					{
						Key:    bundle.RelationKeyName.String(),
						Format: model.RelationFormat_shorttext,
					},
				},
			},
		}
		st.AddBundledRelationLinks(bundle.RelationKeyName, bundle.RelationKeyIconOption)

		want := &State{
			relationLinks: []*model.RelationLink{
				{
					Key:    bundle.RelationKeyName.String(),
					Format: model.RelationFormat_shorttext,
				},
				{
					Key:    bundle.RelationKeyIconOption.String(),
					Format: model.RelationFormat_number,
				},
			},
			parent: &State{
				relationLinks: []*model.RelationLink{
					{
						Key:    bundle.RelationKeyName.String(),
						Format: model.RelationFormat_shorttext,
					},
				},
			},
		}

		assert.Equal(t, want, st)
	})
}

func TestState_FileRelationKeys(t *testing.T) {
	t.Run("no file relations", func(t *testing.T) {
		// given
		s := &State{}

		// when
		keys := s.FileRelationKeys()

		// then
		assert.Empty(t, keys)
	})
	t.Run("there are file relations", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Format: model.RelationFormat_file, Key: "fileKey1"},
				{Format: model.RelationFormat_file, Key: "fileKey2"},
			},
		}

		// when
		keys := s.FileRelationKeys()

		// then
		expectedKeys := []domain.RelationKey{"fileKey1", "fileKey2"}
		assert.ElementsMatch(t, keys, expectedKeys)
	})
	t.Run("duplicated file relations", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Format: model.RelationFormat_file, Key: "fileKey1"},
				{Format: model.RelationFormat_file, Key: "fileKey1"},
			},
		}

		// when
		keys := s.FileRelationKeys()

		// then
		expectedKeys := []domain.RelationKey{"fileKey1"}
		assert.ElementsMatch(t, keys, expectedKeys)
	})
	t.Run("coverId relation", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Key: bundle.RelationKeyCoverId.String()},
			},
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverType: domain.Int64(1),
			}),
		}

		// when
		keys := s.FileRelationKeys()

		// then
		expectedKeys := []domain.RelationKey{bundle.RelationKeyCoverId}
		assert.ElementsMatch(t, keys, expectedKeys)
	})
	t.Run("mixed relations", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Format: model.RelationFormat_file, Key: "fileKey1"},
				{Key: bundle.RelationKeyCoverId.String()},
			},
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverType: domain.Int64(4),
			}),
		}

		// when
		keys := s.FileRelationKeys()

		// then
		expectedKeys := []domain.RelationKey{"fileKey1", bundle.RelationKeyCoverId}
		assert.ElementsMatch(t, keys, expectedKeys, "Expected both file keys and cover ID")
	})
	t.Run("coverType not in details", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Key: bundle.RelationKeyCoverId.String()},
			},
		}

		// when
		keys := s.FileRelationKeys()

		// then
		assert.Empty(t, keys)
	})
	t.Run("unsplash cover", func(t *testing.T) {
		// given
		s := &State{
			relationLinks: pbtypes.RelationLinks{
				{Key: bundle.RelationKeyCoverId.String()},
			},
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverType: domain.Int64(5),
			}),
		}

		// when
		keys := s.FileRelationKeys()

		// then
		assert.Len(t, keys, 1)
	})
}

func TestState_AddRelationLinks(t *testing.T) {
	t.Run("add new link", func(t *testing.T) {
		// given
		s := &State{}
		newLink := &model.RelationLink{
			Key:    "newLink",
			Format: model.RelationFormat_shorttext,
		}

		// when
		s.AddRelationLinks(newLink)

		// then
		assert.True(t, s.GetRelationLinks().Has("newLink"))
	})
	t.Run("add existing link", func(t *testing.T) {
		// given
		s := &State{}
		newLink := &model.RelationLink{
			Key:    "existingLink",
			Format: model.RelationFormat_shorttext,
		}

		// when
		s.AddRelationLinks(newLink)
		s.AddRelationLinks(newLink)

		// then
		assert.True(t, s.GetRelationLinks().Has("existingLink"))
		assert.Len(t, s.GetRelationLinks(), 1)
	})
}

func TestFilter(t *testing.T) {
	t.Run("remove blocks", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2"}}),
			"2":    base.NewBase(&model.Block{Id: "2"}),
		}).(*State)
		st.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyCoverType:      domain.Int64(1),
			bundle.RelationKeyName:           domain.String("name"),
			bundle.RelationKeyAssignee:       domain.String("assignee"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
		}))
		st.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyCoverType.String(),
			Format: model.RelationFormat_number,
		},
			&model.RelationLink{
				Key:    bundle.RelationKeyName.String(),
				Format: model.RelationFormat_longtext,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyResolvedLayout.String(),
				Format: model.RelationFormat_number,
			},
		)

		// when
		filteredState := st.Filter(&Filters{RemoveBlocks: true})

		// then
		assert.Len(t, filteredState.blocks, 1)
		assert.NotNil(t, filteredState.blocks["root"])
	})
	t.Run("filter relations by white list", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2"}}),
			"2":    base.NewBase(&model.Block{Id: "2"}),
		}).(*State)
		st.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyCoverType:      domain.Int64(1),
			bundle.RelationKeyName:           domain.String("name"),
			bundle.RelationKeyAssignee:       domain.String("assignee"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
		}))
		st.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyCoverType.String(),
			Format: model.RelationFormat_number,
		},
			&model.RelationLink{
				Key:    bundle.RelationKeyName.String(),
				Format: model.RelationFormat_longtext,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyResolvedLayout.String(),
				Format: model.RelationFormat_number,
			},
		)

		// when
		filteredState := st.Filter(&Filters{RelationsWhiteList: map[model.ObjectTypeLayout][]domain.RelationKey{
			model.ObjectType_todo: {bundle.RelationKeyAssignee},
		}})

		// then
		assert.Equal(t, filteredState.details.Len(), 1)
		assert.Equal(t, filteredState.localDetails.Len(), 0)
		assert.Len(t, filteredState.relationLinks, 1)
		assert.Equal(t, bundle.RelationKeyAssignee.String(), filteredState.relationLinks[0].Key)
	})
	t.Run("empty white list relations", func(t *testing.T) {
		// given
		st := NewDoc("root", map[string]simple.Block{
			"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2"}}),
			"2":    base.NewBase(&model.Block{Id: "2"}),
		}).(*State)
		st.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyCoverType:      domain.Int64(1),
			bundle.RelationKeyName:           domain.String("name"),
			bundle.RelationKeyAssignee:       domain.String("assignee"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
		}))
		st.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyCoverType.String(),
			Format: model.RelationFormat_number,
		},
			&model.RelationLink{
				Key:    bundle.RelationKeyName.String(),
				Format: model.RelationFormat_longtext,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
			&model.RelationLink{
				Key:    bundle.RelationKeyResolvedLayout.String(),
				Format: model.RelationFormat_number,
			},
		)

		// when
		filteredState := st.Filter(&Filters{RelationsWhiteList: map[model.ObjectTypeLayout][]domain.RelationKey{
			model.ObjectType_todo: {},
		}})

		// then
		assert.Equal(t, filteredState.details.Len(), 0)
		assert.Equal(t, filteredState.localDetails.Len(), 0)
		assert.Len(t, filteredState.relationLinks, 0)
	})
}
