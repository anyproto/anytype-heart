package common

import (
	"testing"

	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestReplaceChunks(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		oldToNew map[string]string
		want     []string
	}{
		{
			name: "Test 1",
			s:    "thequickbrownfoxjumpsoverthelazydog",
			oldToNew: map[string]string{
				"brown": "blue",
				"lazy":  "energetic",
				"jumps": "flies",
			},
			want: []string{"thequick", "blue", "fox", "flies", "overthe", "energetic"},
		},
		{
			name: "Test 2",
			s:    "loremipsumdolorsitamet",
			oldToNew: map[string]string{
				"ipsum": "filler",
				"dolor": "pain",
				"amet":  "meet",
			},
			want: []string{"lorem", "filler", "pain", "sit", "meet"},
		},
		{
			name: "Test 3",
			s:    "abcde",
			oldToNew: map[string]string{
				"a": "1",
				"b": "2",
				"c": "3",
				"d": "4",
				"e": "5",
			},
			want: []string{"1", "2", "3", "4", "5"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceChunks(tc.s, tc.oldToNew)

			if !assert.Equal(t, tc.want, got) {
				t.Errorf("replaceChunks(%q, %v) = %q; want %q", tc.s, tc.oldToNew, got, tc.want)
			}
		})
	}
}

func TestUpdateLinksToObjects(t *testing.T) {
	t.Run("icon image is set in text block", func(t *testing.T) {
		// given
		rawCid, err := cidutil.NewCidFromBytes([]byte("test"))
		assert.Nil(t, err)
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: rawCid,
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		oldToNew := map[string]string{rawCid: "newFileObjectId"}

		// when
		err = UpdateLinksToObjects(st, oldToNew)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newFileObjectId", st.Get("test").Model().GetText().GetIconImage())
	})
	t.Run("icon image is not set in text block", func(t *testing.T) {
		// given
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "", st.Get("test").Model().GetText().GetIconImage())
	})
	t.Run("icon image is set in text block, but file is not present", func(t *testing.T) {
		// given
		rawCid, err := cidutil.NewCidFromBytes([]byte("test"))
		assert.Nil(t, err)
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: rawCid,
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err = UpdateLinksToObjects(st, map[string]string{})

		// then
		assert.Nil(t, err)
		assert.Equal(t, addr.MissingObject, st.Get("test").Model().GetText().GetIconImage())
	})
	t.Run("icon image is url from Notion", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: "url",
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "url", st.Get("test").Model().GetText().GetIconImage())
	})
	t.Run("update data view filters relations", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "id",
						Filters: []*model.BlockContentDataviewFilter{
							{
								Id:          "id1",
								RelationKey: "key",
								Value:       pbtypes.String("test"),
							},
							{
								Id:          "id1",
								RelationKey: "key1",
								Value:       pbtypes.String("test"),
							},
						},
					},
				},
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{"key": "newKey"})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newKey", st.Get("test").Model().GetDataview().GetViews()[0].GetFilters()[0].RelationKey)
		assert.Equal(t, "key1", st.Get("test").Model().GetDataview().GetViews()[0].GetFilters()[1].RelationKey)
	})
	t.Run("update data view filters relations", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "id",
						Filters: []*model.BlockContentDataviewFilter{
							{
								Id:          "id1",
								RelationKey: "key",
								Value:       pbtypes.String("test"),
							},
							{
								Id:          "id1",
								RelationKey: "key1",
								Value:       pbtypes.String("test"),
							},
						},
					},
				},
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{"key": "newKey"})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newKey", st.Get("test").Model().GetDataview().GetViews()[0].GetFilters()[0].RelationKey)
		assert.Equal(t, "key1", st.Get("test").Model().GetDataview().GetViews()[0].GetFilters()[1].RelationKey)
	})
	t.Run("update data view relations", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "id",
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key: "key",
							},
							{
								Key: "key1",
							},
						},
					},
				},
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{"key": "newKey"})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newKey", st.Get("test").Model().GetDataview().GetViews()[0].GetRelations()[0].Key)
		assert.Equal(t, "key1", st.Get("test").Model().GetDataview().GetViews()[0].GetRelations()[1].Key)
	})
	t.Run("update data view relations links", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				RelationLinks: []*model.RelationLink{
					{
						Key: "key",
					},
					{
						Key: "key1",
					},
				},
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{"key": "newKey"})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newKey", st.Get("test").Model().GetDataview().GetRelationLinks()[0].Key)
		assert.Equal(t, "key1", st.Get("test").Model().GetDataview().GetRelationLinks()[1].Key)
	})
	t.Run("update data view sort", func(t *testing.T) {
		// given
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "id",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: "key",
							},
							{
								RelationKey: "key1",
							},
						},
					},
				},
			}},
		}
		rootBlock := &model.Block{
			Id:          "root",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}
		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		st := state.NewDoc("root", map[string]simple.Block{"test": simpleBlock, "root": rootSimpleBlock}).(*state.State)

		// when
		err := UpdateLinksToObjects(st, map[string]string{"key": "newKey"})

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newKey", st.Get("test").Model().GetDataview().GetViews()[0].GetSorts()[0].RelationKey)
		assert.Equal(t, "key1", st.Get("test").Model().GetDataview().GetViews()[0].GetSorts()[1].RelationKey)
	})
}
