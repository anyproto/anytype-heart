package csv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type stubName struct {
	name string
}

func (s stubName) Get(path, hash, title, ext string) (name string) {
	return s.name
}

func TestConverter(t *testing.T) {
	t.Run("no dataview block", func(t *testing.T) {
		// given
		converter := NewConverter(objectstore.NewStoreFixture(t), nil)
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}),
		}).(*state.State)

		// when
		result := converter.Convert(st)

		// then
		assert.Nil(t, result)
	})
	t.Run("empty dataview", func(t *testing.T) {
		// given
		converter := NewConverter(objectstore.NewStoreFixture(t), nil)
		st := state.NewDoc("root", map[string]simple.Block{
			"root":     simple.New(&model.Block{Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}),
			"dataview": simple.New(&model.Block{Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}}}),
		}).(*state.State)

		// when
		result := converter.Convert(st)

		// then
		assert.Nil(t, result)
	})
	t.Run("no known docs", func(t *testing.T) {
		// given
		converter := NewConverter(objectstore.NewStoreFixture(t), nil)
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}),
			"dataview": simple.New(&model.Block{ChildrenIds: []string{"dataview"}, Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key:       bundle.RelationKeyName.String(),
								IsVisible: true,
							},
							{
								Key:       bundle.RelationKeyDueDate.String(),
								IsVisible: false,
							},
						},
					},
					{
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key:       bundle.RelationKeyCamera.String(),
								IsVisible: true,
							},
						},
					},
				},
			}}}),
		}).(*state.State)
		st.SetLocalDetail(bundle.RelationKeySpaceId, domain.String("spaceId"))
		st.UpdateStoreSlice(template.CollectionStoreKey, []string{"test1"})

		// when
		result := converter.Convert(st)

		// then
		assert.Empty(t, result)
	})
	t.Run("convert to csv", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String("id1"),
				bundle.RelationKeyName:        domain.String("Name"),
				bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeySpaceId:     domain.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          domain.String("id2"),
				bundle.RelationKeyName:        domain.String("Due date"),
				bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeyDueDate.String()),
				bundle.RelationKeySpaceId:     domain.String("spaceId"),
			},
		})
		converter := NewConverter(storeFixture, map[string]*domain.Details{
			"test1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:    domain.String("Test Name"),
				bundle.RelationKeyDueDate: domain.Int64(time.Now().Unix()),
				bundle.RelationKeyCamera:  domain.String("test"),
			}),
		})
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{ChildrenIds: []string{"dataview"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}),
			"dataview": simple.New(&model.Block{Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key:       bundle.RelationKeyName.String(),
								IsVisible: true,
							},
							{
								Key:       bundle.RelationKeyDueDate.String(),
								IsVisible: false,
							},
						},
					},
					{
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key:       bundle.RelationKeyCamera.String(),
								IsVisible: true,
							},
						},
					},
				},
			}}}),
		}).(*state.State)
		st.SetLocalDetail(bundle.RelationKeySpaceId, domain.String("spaceId"))
		st.UpdateStoreSlice(template.CollectionStoreKey, []string{"test1"})

		// when
		result := converter.Convert(st)

		// then
		assert.Equal(t, "Name\nTest Name\n", string(result))
	})
}
