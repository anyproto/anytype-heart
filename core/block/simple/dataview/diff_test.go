package dataview

import (
	"github.com/anyproto/anytype-heart/core/block/simple/test"
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockDataviewIsCollectionSet{
			BlockDataviewIsCollectionSet: &pb.EventBlockDataviewIsCollectionSet{
				Id:    b1.Id,
				Value: true,
			},
		}), diff)
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
			BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
				Id:             b1.Id,
				TargetObjectId: "2",
			},
		}), diff)
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockDataviewSourceSet{
			BlockDataviewSourceSet: &pb.EventBlockDataviewSourceSet{
				Id:     b1.Id,
				Source: []string{"1"},
			},
		}), diff)
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockDataviewSourceSet{
			BlockDataviewSourceSet: &pb.EventBlockDataviewSourceSet{
				Id:     b1.Id,
				Source: nil,
			},
		}), diff)
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockDataviewViewOrder{
			BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
				Id:      b1.Id,
				ViewIds: []string{"2", "1"},
			},
		}), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
				BlockDataViewGroupOrderUpdate: &pb.EventBlockDataviewGroupOrderUpdate{
					Id: b1.Id,
					GroupOrder: &model.BlockContentDataviewGroupOrder{
						ViewId:     "1",
						ViewGroups: view1Groups,
					},
				},
			},
			&pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
				BlockDataViewGroupOrderUpdate: &pb.EventBlockDataviewGroupOrderUpdate{
					Id: b1.Id,
					GroupOrder: &model.BlockContentDataviewGroupOrder{
						ViewId:     "2",
						ViewGroups: view2Groups,
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
				BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
					Id:      b1.Id,
					ViewId:  "1",
					GroupId: "1",
					SliceChanges: []*pb.EventBlockDataviewSliceChange{
						{
							Op:      pb.EventBlockDataview_SliceOperationRemove,
							Ids:     []string{"object2"},
							AfterId: "",
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
				BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
					Id:      b1.Id,
					ViewId:  "1",
					GroupId: "1",
					SliceChanges: []*pb.EventBlockDataviewSliceChange{
						{
							Op:      pb.EventBlockDataview_SliceOperationAdd,
							Ids:     []string{"object3"},
							AfterId: "object2",
						},
					},
				},
			},
			&pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
				BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
					Id:      b1.Id,
					ViewId:  "2",
					GroupId: "1",
					SliceChanges: []*pb.EventBlockDataviewSliceChange{
						{
							Op:      pb.EventBlockDataview_SliceOperationAdd,
							Ids:     []string{"object1"},
							AfterId: "",
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
				BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
					Id:      b1.Id,
					ViewId:  "1",
					GroupId: "1",
					SliceChanges: []*pb.EventBlockDataviewSliceChange{
						{
							Op:      pb.EventBlockDataview_SliceOperationMove,
							Ids:     []string{"object3"},
							AfterId: "object1",
						},
					},
				},
			},
		), diff)
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
		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewRelationSet{
				BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
					Id: b1.Id,
					RelationLinks: []*model.RelationLink{
						{
							Key:    "relation2",
							Format: model.RelationFormat_longtext,
						},
					},
				},
			},
		), diff)
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
		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewRelationDelete{
				BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
					Id:           b1.Id,
					RelationKeys: []string{"relation2"},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					ViewId: "1",
					Fields: &pb.EventBlockDataviewViewUpdateFields{
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
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewSet{
				BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
					Id:     b1.Id,
					ViewId: "2",
					View: &model.BlockContentDataviewView{
						Id: "2",
					},
				},
			},
			&pb.EventMessageValueOfBlockDataviewViewOrder{
				BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
					Id:      b1.Id,
					ViewIds: []string{"1", "2"},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewDelete{
				BlockDataviewViewDelete: &pb.EventBlockDataviewViewDelete{
					Id:     b1.Id,
					ViewId: "1",
				},
			},
			&pb.EventMessageValueOfBlockDataviewViewOrder{
				BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
					Id:      b1.Id,
					ViewIds: []string{"2"},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Sort: []*pb.EventBlockDataviewViewUpdateSort{
						{
							Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfUpdate{
								Update: &pb.EventBlockDataviewViewUpdateSortUpdate{
									Id:   "1",
									Item: newSort,
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Sort: []*pb.EventBlockDataviewViewUpdateSort{
						{
							Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfAdd{
								Add: &pb.EventBlockDataviewViewUpdateSortAdd{
									AfterId: "1",
									Items:   []*model.BlockContentDataviewSort{addedSort},
								},
							},
						},
					},
				},
			},
		), diff)
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
		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Sort: []*pb.EventBlockDataviewViewUpdateSort{
						{
							Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfRemove{
								Remove: &pb.EventBlockDataviewViewUpdateSortRemove{
									Ids: []string{"1"},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Filter: []*pb.EventBlockDataviewViewUpdateFilter{
						{
							Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate{
								Update: &pb.EventBlockDataviewViewUpdateFilterUpdate{
									Id:   "1",
									Item: newFilter,
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Filter: []*pb.EventBlockDataviewViewUpdateFilter{
						{
							Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfRemove{
								Remove: &pb.EventBlockDataviewViewUpdateFilterRemove{
									Ids: []string{"1"},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Filter: []*pb.EventBlockDataviewViewUpdateFilter{
						{
							Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfAdd{
								Add: &pb.EventBlockDataviewViewUpdateFilterAdd{
									AfterId: "1",
									Items: []*model.BlockContentDataviewFilter{
										newSort,
									},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Filter: []*pb.EventBlockDataviewViewUpdateFilter{
						{
							Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfMove{
								Move: &pb.EventBlockDataviewViewUpdateFilterMove{
									Ids: []string{"2"},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Relation: []*pb.EventBlockDataviewViewUpdateRelation{
						{
							Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfUpdate{
								Update: &pb.EventBlockDataviewViewUpdateRelationUpdate{
									Id: "name",
									Item: &model.BlockContentDataviewRelation{
										Key:       "name",
										IsVisible: false,
										Width:     1,
									},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Relation: []*pb.EventBlockDataviewViewUpdateRelation{
						{
							Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfAdd{
								Add: &pb.EventBlockDataviewViewUpdateRelationAdd{
									AfterId: "name",
									Items:   []*model.BlockContentDataviewRelation{addRelation},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Relation: []*pb.EventBlockDataviewViewUpdateRelation{
						{
							Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfRemove{
								Remove: &pb.EventBlockDataviewViewUpdateRelationRemove{
									Ids: []string{"name"},
								},
							},
						},
					},
				},
			},
		), diff)
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

		assert.Equal(t, test.MakeEvent(
			&pb.EventMessageValueOfBlockDataviewViewUpdate{
				BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
					Id:     b1.Id,
					ViewId: "1",
					Relation: []*pb.EventBlockDataviewViewUpdateRelation{
						{
							Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfMove{
								Move: &pb.EventBlockDataviewViewUpdateRelationMove{
									Ids: []string{"description"},
								},
							},
						},
					},
				},
			},
		), diff)
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
