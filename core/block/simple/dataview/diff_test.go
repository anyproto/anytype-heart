package dataview

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiff(t *testing.T) {
	testBlock := func() *Dataview {
		return NewDataview(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}},
		}).(*Dataview)
	}
	t.Run("is collection changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.IsCollection = true
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewIsCollectionSet).BlockDataviewIsCollectionSet
		assert.Equal(t, true, change.Value)
	})
	t.Run("target object id changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.TargetObjectId = "1"
		b2.content.TargetObjectId = "2"
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewTargetObjectIdSet).BlockDataviewTargetObjectIdSet
		assert.Equal(t, "2", change.TargetObjectId)
	})
	t.Run("source changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.Source = []string{"1"}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewSourceSet).BlockDataviewSourceSet
		assert.Len(t, change.Source, 1)
		assert.Equal(t, "1", change.Source[0])
	})
	t.Run("unset source", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.Source = nil
		b1.content.Source = []string{"1"}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewSourceSet).BlockDataviewSourceSet
		assert.Len(t, change.Source, 0)
	})
	t.Run("order of views changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{{Id: "1"}, {Id: "2"}}
		b2.content.Views = []*model.BlockContentDataviewView{{Id: "2"}, {Id: "1"}}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewOrder).BlockDataviewViewOrder
		assert.Len(t, change.ViewIds, 2)
		assert.Equal(t, change.ViewIds, []string{"2", "1"})
	})
	t.Run("group order changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.GroupOrders = []*model.BlockContentDataviewGroupOrder{
			{
				ViewId: "1",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "1",
						Index:   1,
						Hidden:  true,
					},
					{
						GroupId: "2",
						Index:   2,
						Hidden:  false,
					},
				},
			},
			{
				ViewId: "2",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "1",
						Index:   1,
						Hidden:  false,
					},
				},
			},
		}
		view1Groups := []*model.BlockContentDataviewViewGroup{
			{
				GroupId: "2",
				Index:   2,
				Hidden:  true,
			},
			{
				GroupId: "3",
				Index:   3,
				Hidden:  true,
			},
		}
		view2Groups := []*model.BlockContentDataviewViewGroup{
			{
				GroupId: "1",
				Index:   1,
				Hidden:  false,
			},
			{
				GroupId: "2",
				Index:   1,
				Hidden:  true,
			},
		}
		b2.content.GroupOrders = []*model.BlockContentDataviewGroupOrder{
			{
				ViewId:     "1",
				ViewGroups: view1Groups,
			},
			{
				ViewId:     "2",
				ViewGroups: view2Groups,
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 2)

		change1 := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataViewGroupOrderUpdate).BlockDataViewGroupOrderUpdate
		change2 := diff[1].Msg.Value.(*pb.EventMessageValueOfBlockDataViewGroupOrderUpdate).BlockDataViewGroupOrderUpdate

		assert.Equal(t, "1", change1.GroupOrder.ViewId)
		assert.Equal(t, view1Groups, change1.GroupOrder.ViewGroups)

		assert.Equal(t, "2", change2.GroupOrder.ViewId)
		assert.Equal(t, view2Groups, change2.GroupOrder.ViewGroups)
	})
	t.Run("order of objects: remove object", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2", "object3"},
			},
		}
		b2.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object3"},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataViewObjectOrderUpdate).BlockDataViewObjectOrderUpdate
		assert.Equal(t, "1", change.ViewId)
		assert.Len(t, change.SliceChanges, 1)

		assert.Equal(t, pb.EventBlockDataview_SliceOperationRemove, change.SliceChanges[0].Op)
		assert.Equal(t, []string{"object2"}, change.SliceChanges[0].Ids)
	})
	t.Run("order of objects: add object", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2"},
			},
		}
		b2.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2", "object3"},
			},
			{
				ViewId:    "2",
				GroupId:   "1",
				ObjectIds: []string{"object1"},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 2)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataViewObjectOrderUpdate).BlockDataViewObjectOrderUpdate
		change1 := diff[1].Msg.Value.(*pb.EventMessageValueOfBlockDataViewObjectOrderUpdate).BlockDataViewObjectOrderUpdate

		assert.Equal(t, "1", change.ViewId)
		assert.Len(t, change.SliceChanges, 1)
		assert.Equal(t, pb.EventBlockDataview_SliceOperationAdd, change.SliceChanges[0].Op)
		assert.Equal(t, []string{"object3"}, change.SliceChanges[0].Ids)

		assert.Equal(t, "2", change1.ViewId)
		assert.Len(t, change1.SliceChanges, 1)
		assert.Equal(t, pb.EventBlockDataview_SliceOperationAdd, change1.SliceChanges[0].Op)
		assert.Equal(t, []string{"object1"}, change1.SliceChanges[0].Ids)
	})
	t.Run("order of objects: move objects", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2", "object3"},
			},
		}
		b2.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object3", "object2"},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataViewObjectOrderUpdate).BlockDataViewObjectOrderUpdate

		assert.Equal(t, "1", change.ViewId)
		assert.Len(t, change.SliceChanges, 1)
		assert.Equal(t, pb.EventBlockDataview_SliceOperationMove, change.SliceChanges[0].Op)
		assert.Equal(t, []string{"object3"}, change.SliceChanges[0].Ids)
		assert.Equal(t, "object1", change.SliceChanges[0].AfterId)
	})
	t.Run("relation links add change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
		}
		b2.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
			{
				Key:    "relation2",
				Format: model.RelationFormat_longtext,
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewRelationSet).BlockDataviewRelationSet
		assert.Len(t, change.RelationLinks, 1)
		assert.Equal(t, "relation2", change.RelationLinks[0].Key)
	})
	t.Run("relation links remove change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
			{
				Key:    "relation2",
				Format: model.RelationFormat_longtext,
			},
		}
		b2.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewRelationDelete).BlockDataviewRelationDelete
		assert.Len(t, change.RelationKeys, 1)
		assert.Equal(t, "relation2", change.RelationKeys[0])
	})
	t.Run("view field changes", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id:                    "1",
				Type:                  model.BlockContentDataviewView_Table,
				Name:                  "All",
				CoverRelationKey:      "cover",
				HideIcon:              false,
				CardSize:              model.BlockContentDataviewView_Medium,
				CoverFit:              false,
				GroupRelationKey:      "status",
				GroupBackgroundColors: false,
				PageLimit:             10,
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:                    "1",
				Type:                  model.BlockContentDataviewView_List,
				Name:                  "New Name",
				CoverRelationKey:      "cover",
				HideIcon:              true,
				CardSize:              model.BlockContentDataviewView_Large,
				CoverFit:              true,
				GroupRelationKey:      "tag",
				GroupBackgroundColors: true,
				PageLimit:             11,
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Equal(t, "New Name", change.Fields.Name)
		assert.Equal(t, model.BlockContentDataviewView_List, change.Fields.Type)
		assert.Equal(t, true, change.Fields.HideIcon)
		assert.Equal(t, model.BlockContentDataviewView_Large, change.Fields.CardSize)
		assert.Equal(t, true, change.Fields.CoverFit)
		assert.Equal(t, "tag", change.Fields.GroupRelationKey)
		assert.Equal(t, true, change.Fields.GroupBackgroundColors)
		assert.Equal(t, int32(11), change.Fields.PageLimit)
	})
	t.Run("view add", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
			},
			{
				Id: "2",
			},
		}

		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 2) // TODO here we have 2 events because diffOrderOfViews also make change for view order, need to rewrite it to avoid not needed changes

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewSet).BlockDataviewViewSet
		assert.Equal(t, "2", change.ViewId)
	})
	t.Run("view remove change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
			},
			{
				Id: "2",
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "2",
			},
		}

		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 2)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewDelete).BlockDataviewViewDelete
		assert.Equal(t, "1", change.ViewId)
	})
	t.Run("view sort update change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Sorts: []*model.BlockContentDataviewSort{
					{
						Id:          "1",
						RelationKey: "name",
						Type:        model.BlockContentDataviewSort_Desc,
						Format:      model.RelationFormat_longtext,
						IncludeTime: false,
					},
				},
			},
		}
		newSort := &model.BlockContentDataviewSort{
			Id:          "1",
			RelationKey: "name",
			Type:        model.BlockContentDataviewSort_Asc,
			Format:      model.RelationFormat_longtext,
			IncludeTime: false,
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:    "1",
				Sorts: []*model.BlockContentDataviewSort{newSort},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Sort, 1)
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfUpdate))
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfUpdate).Update)
		updateSort := change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfUpdate).Update.Item
		assert.Equal(t, newSort, updateSort)
	})
	t.Run("view sort add change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Sorts: []*model.BlockContentDataviewSort{
					{
						Id:          "1",
						RelationKey: "name",
						Type:        model.BlockContentDataviewSort_Desc,
						Format:      model.RelationFormat_longtext,
						IncludeTime: false,
					},
				},
			},
		}
		addedSort := &model.BlockContentDataviewSort{
			Id:          "2",
			RelationKey: "date",
			Type:        model.BlockContentDataviewSort_Asc,
			Format:      model.RelationFormat_date,
			IncludeTime: false,
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Sorts: []*model.BlockContentDataviewSort{
					{
						Id:          "1",
						RelationKey: "name",
						Type:        model.BlockContentDataviewSort_Desc,
						Format:      model.RelationFormat_longtext,
						IncludeTime: false,
					},
					addedSort,
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Sort, 1)
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfAdd))
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfAdd).Add)
		addedSortChange := change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfAdd).Add
		assert.Equal(t, "1", addedSortChange.AfterId)
		assert.Len(t, addedSortChange.Items, 1)
		assert.Equal(t, addedSort, addedSortChange.Items[0])
	})
	t.Run("view sort remove change", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Sorts: []*model.BlockContentDataviewSort{
					{
						Id:          "1",
						RelationKey: "name",
						Type:        model.BlockContentDataviewSort_Desc,
						Format:      model.RelationFormat_longtext,
						IncludeTime: false,
					},
				},
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Sort, 1)
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfRemove))
		assert.NotNil(t, change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfRemove).Remove)
		removedSorts := change.Sort[0].Operation.(*pb.EventBlockDataviewViewUpdateSortOperationOfRemove).Remove.Ids
		assert.Equal(t, "1", removedSorts[0])
	})
	t.Run("view filter update", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
				},
			},
		}

		newFilter := &model.BlockContentDataviewFilter{
			Id:          "1",
			RelationKey: "name",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String("name1"),
			IncludeTime: false,
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:      "1",
				Filters: []*model.BlockContentDataviewFilter{newFilter},
			},
		}

		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Filter, 1)
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate))
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate).Update)
		updatedFilter := change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate).Update
		assert.Equal(t, "1", updatedFilter.Id)
		assert.Equal(t, newFilter, updatedFilter.Item)
	})
	t.Run("view filter remove", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
				},
			},
		}

		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:      "1",
				Filters: []*model.BlockContentDataviewFilter{},
			},
		}

		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Filter, 1)
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfRemove))
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfRemove).Remove)
		removedFilter := change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfRemove).Remove
		assert.Len(t, removedFilter.Ids, 1)
		assert.Equal(t, "1", removedFilter.Ids[0])
	})
	t.Run("view filter add", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
				},
			},
		}

		newSort := &model.BlockContentDataviewFilter{
			Id:          "2",
			RelationKey: "description",
			Condition:   model.BlockContentDataviewFilter_Empty,
			IncludeTime: false,
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
					newSort,
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Filter, 1)
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfAdd))
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfAdd).Add)
		addFilter := change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfAdd).Add
		assert.Equal(t, "1", addFilter.AfterId)
		assert.Len(t, addFilter.Items, 1)
		assert.Equal(t, newSort, addFilter.Items[0])
	})
	t.Run("view filter move", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
					{
						Id:          "2",
						RelationKey: "description",
						Condition:   model.BlockContentDataviewFilter_Empty,
						IncludeTime: false,
					},
				},
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "2",
						RelationKey: "description",
						Condition:   model.BlockContentDataviewFilter_Empty,
						IncludeTime: false,
					},
					{
						Id:          "1",
						RelationKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("name"),
						IncludeTime: false,
					},
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Filter, 1)
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfMove))
		assert.NotNil(t, change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfMove).Move)
		moveFilter := change.Filter[0].Operation.(*pb.EventBlockDataviewViewUpdateFilterOperationOfMove).Move
		assert.Len(t, moveFilter.Ids, 1)
		assert.Equal(t, "2", moveFilter.Ids[0])
	})
	t.Run("view relation update", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
				},
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: false,
						Width:     1,
					},
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Relation, 1)
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfUpdate))
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfUpdate).Update)
		updateRelations := change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfUpdate).Update
		assert.Equal(t, false, updateRelations.Item.IsVisible)
	})
	t.Run("view relation add", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
				},
			},
		}

		addRelation := &model.BlockContentDataviewRelation{
			Key:       "description",
			IsVisible: false,
			Width:     1,
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
					addRelation,
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Relation, 1)
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfAdd))
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfAdd).Add)
		addRelationsChange := change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfAdd).Add
		assert.Len(t, addRelationsChange.Items, 1)
		assert.Equal(t, addRelation, addRelationsChange.Items[0])
	})
	t.Run("view relation remove", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
				},
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:        "1",
				Relations: []*model.BlockContentDataviewRelation{},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Relation, 1)
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfRemove))
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfRemove).Remove)
		removeRelationsChange := change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfRemove).Remove
		assert.Len(t, removeRelationsChange.Ids, 1)
		assert.Equal(t, "name", removeRelationsChange.Ids[0])
	})
	t.Run("view relation move", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
					{
						Key:       "description",
						IsVisible: false,
						Width:     1,
					},
				},
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id: "1",
				Relations: []*model.BlockContentDataviewRelation{
					{
						Key:       "description",
						IsVisible: false,
						Width:     1,
					},
					{
						Key:       "name",
						IsVisible: true,
						Width:     1,
					},
				},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockDataviewViewUpdate).BlockDataviewViewUpdate
		assert.Len(t, change.Relation, 1)
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfMove))
		assert.NotNil(t, change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfMove).Move)
		moveRelationsChange := change.Relation[0].Operation.(*pb.EventBlockDataviewViewUpdateRelationOperationOfMove).Move
		assert.Len(t, moveRelationsChange.Ids, 1)
		assert.Equal(t, "description", moveRelationsChange.Ids[0])
	})
	t.Run("multiple changes", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.content.Views = []*model.BlockContentDataviewView{
			{
				Id:                    "1",
				Type:                  model.BlockContentDataviewView_Table,
				Name:                  "All",
				CoverRelationKey:      "cover",
				HideIcon:              false,
				CardSize:              model.BlockContentDataviewView_Medium,
				CoverFit:              false,
				GroupRelationKey:      "status",
				GroupBackgroundColors: false,
				PageLimit:             10,
			},
		}
		b2.content.Views = []*model.BlockContentDataviewView{
			{
				Id:                    "1",
				Type:                  model.BlockContentDataviewView_List,
				Name:                  "New Name",
				CoverRelationKey:      "cover",
				HideIcon:              true,
				CardSize:              model.BlockContentDataviewView_Large,
				CoverFit:              true,
				GroupRelationKey:      "tag",
				GroupBackgroundColors: true,
				PageLimit:             11,
			},
		}

		b1.content.GroupOrders = []*model.BlockContentDataviewGroupOrder{
			{
				ViewId: "1",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "1",
						Index:   1,
						Hidden:  true,
					},
					{
						GroupId: "2",
						Index:   2,
						Hidden:  false,
					},
				},
			},
			{
				ViewId: "2",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "1",
						Index:   1,
						Hidden:  false,
					},
				},
			},
		}

		b2.content.GroupOrders = []*model.BlockContentDataviewGroupOrder{
			{
				ViewId: "1",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "2",
						Index:   2,
						Hidden:  true,
					},
					{
						GroupId: "3",
						Index:   3,
						Hidden:  true,
					},
				},
			},
			{
				ViewId: "2",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{
						GroupId: "1",
						Index:   1,
						Hidden:  false,
					},
				},
			},
		}
		b1.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
		}
		b2.content.RelationLinks = []*model.RelationLink{
			{
				Key:    "relation1",
				Format: model.RelationFormat_longtext,
			},
			{
				Key:    "relation2",
				Format: model.RelationFormat_longtext,
			},
		}

		b1.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2"},
			},
		}
		b2.content.ObjectOrders = []*model.BlockContentDataviewObjectOrder{
			{
				ViewId:    "1",
				GroupId:   "1",
				ObjectIds: []string{"object1", "object2", "object3"},
			},
		}
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 4)
	})
}
