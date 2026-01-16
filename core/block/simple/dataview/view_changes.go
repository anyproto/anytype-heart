package dataview

import (
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func diffViewFields(a, b *model.BlockContentDataviewView) *pb.EventBlockDataviewViewUpdateFields {
	isEqual := a.Type == b.Type &&
		a.Name == b.Name &&
		a.CoverRelationKey == b.CoverRelationKey &&
		a.HideIcon == b.HideIcon &&
		a.CardSize == b.CardSize &&
		a.CoverFit == b.CoverFit &&
		a.GroupRelationKey == b.GroupRelationKey &&
		a.EndRelationKey == b.EndRelationKey &&
		a.GroupBackgroundColors == b.GroupBackgroundColors &&
		a.PageLimit == b.PageLimit &&
		a.DefaultTemplateId == b.DefaultTemplateId &&
		a.DefaultObjectTypeId == b.DefaultObjectTypeId &&
		a.WrapContent == b.WrapContent

	if isEqual {
		return nil
	}
	return &pb.EventBlockDataviewViewUpdateFields{
		Type:                  b.Type,
		Name:                  b.Name,
		CoverRelationKey:      b.CoverRelationKey,
		HideIcon:              b.HideIcon,
		CardSize:              b.CardSize,
		CoverFit:              b.CoverFit,
		GroupRelationKey:      b.GroupRelationKey,
		GroupBackgroundColors: b.GroupBackgroundColors,
		PageLimit:             b.PageLimit,
		DefaultTemplateId:     b.DefaultTemplateId,
		DefaultObjectTypeId:   b.DefaultObjectTypeId,
		EndRelationKey:        b.EndRelationKey,
		WrapContent:           b.WrapContent,
	}
}

func getViewFilterID(f *model.BlockContentDataviewFilter) string {
	return f.Id
}

func isViewFiltersEqual(a, b *model.BlockContentDataviewFilter) bool {
	return proto.Equal(a, b)
}

// nolint:dupl
func diffViewFilters(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateFilter {
	diff := slice.Diff(a.Filters, b.Filters, getViewFilterID, isViewFiltersEqual)
	if len(diff) == 0 {
		return nil
	}

	return slice.UnwrapChanges(
		diff,
		func(afterID string, items []*model.BlockContentDataviewFilter) *pb.EventBlockDataviewViewUpdateFilter {
			return &pb.EventBlockDataviewViewUpdateFilter{
				Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfAdd{
					Add: &pb.EventBlockDataviewViewUpdateFilterAdd{
						AfterId: afterID,
						Items:   items,
					},
				},
			}
		},
		func(ids []string) *pb.EventBlockDataviewViewUpdateFilter {
			return &pb.EventBlockDataviewViewUpdateFilter{
				Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfRemove{
					Remove: &pb.EventBlockDataviewViewUpdateFilterRemove{
						Ids: ids,
					},
				},
			}
		},
		func(afterID string, ids []string) *pb.EventBlockDataviewViewUpdateFilter {
			return &pb.EventBlockDataviewViewUpdateFilter{
				Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfMove{
					Move: &pb.EventBlockDataviewViewUpdateFilterMove{
						AfterId: afterID,
						Ids:     ids,
					},
				},
			}
		},
		func(id string, item *model.BlockContentDataviewFilter) *pb.EventBlockDataviewViewUpdateFilter {
			return &pb.EventBlockDataviewViewUpdateFilter{
				Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate{
					Update: &pb.EventBlockDataviewViewUpdateFilterUpdate{
						Id:   id,
						Item: item,
					},
				},
			}
		})
}

func getViewRelationID(f *model.BlockContentDataviewRelation) string {
	return f.Key
}

func isViewRelationsEqual(a, b *model.BlockContentDataviewRelation) bool {
	return proto.Equal(a, b)
}

// nolint:dupl
func diffViewRelations(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateRelation {
	diff := slice.Diff(a.Relations, b.Relations, getViewRelationID, isViewRelationsEqual)
	if len(diff) == 0 {
		return nil
	}

	return slice.UnwrapChanges(
		diff,
		func(afterID string, items []*model.BlockContentDataviewRelation) *pb.EventBlockDataviewViewUpdateRelation {
			return &pb.EventBlockDataviewViewUpdateRelation{
				Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfAdd{
					Add: &pb.EventBlockDataviewViewUpdateRelationAdd{
						AfterId: afterID,
						Items:   items,
					},
				},
			}
		},
		func(ids []string) *pb.EventBlockDataviewViewUpdateRelation {
			return &pb.EventBlockDataviewViewUpdateRelation{
				Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfRemove{
					Remove: &pb.EventBlockDataviewViewUpdateRelationRemove{
						Ids: ids,
					},
				},
			}
		},
		func(afterID string, ids []string) *pb.EventBlockDataviewViewUpdateRelation {
			return &pb.EventBlockDataviewViewUpdateRelation{
				Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfMove{
					Move: &pb.EventBlockDataviewViewUpdateRelationMove{
						AfterId: afterID,
						Ids:     ids,
					},
				},
			}
		},
		func(id string, item *model.BlockContentDataviewRelation) *pb.EventBlockDataviewViewUpdateRelation {
			return &pb.EventBlockDataviewViewUpdateRelation{
				Operation: &pb.EventBlockDataviewViewUpdateRelationOperationOfUpdate{
					Update: &pb.EventBlockDataviewViewUpdateRelationUpdate{
						Id:   id,
						Item: item,
					},
				},
			}
		})
}

func getViewSortID(f *model.BlockContentDataviewSort) string {
	return f.Id
}

func isViewSortsEqual(a, b *model.BlockContentDataviewSort) bool {
	return proto.Equal(a, b)
}

// nolint:dupl
func diffViewSorts(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateSort {
	diff := slice.Diff(a.Sorts, b.Sorts, getViewSortID, isViewSortsEqual)
	if len(diff) == 0 {
		return nil
	}

	return slice.UnwrapChanges(
		diff,
		func(afterID string, items []*model.BlockContentDataviewSort) *pb.EventBlockDataviewViewUpdateSort {
			return &pb.EventBlockDataviewViewUpdateSort{
				Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfAdd{
					Add: &pb.EventBlockDataviewViewUpdateSortAdd{
						AfterId: afterID,
						Items:   items,
					},
				},
			}
		},
		func(ids []string) *pb.EventBlockDataviewViewUpdateSort {
			return &pb.EventBlockDataviewViewUpdateSort{
				Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfRemove{
					Remove: &pb.EventBlockDataviewViewUpdateSortRemove{
						Ids: ids,
					},
				},
			}
		},
		func(afterID string, ids []string) *pb.EventBlockDataviewViewUpdateSort {
			return &pb.EventBlockDataviewViewUpdateSort{
				Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfMove{
					Move: &pb.EventBlockDataviewViewUpdateSortMove{
						AfterId: afterID,
						Ids:     ids,
					},
				},
			}
		},
		func(id string, item *model.BlockContentDataviewSort) *pb.EventBlockDataviewViewUpdateSort {
			return &pb.EventBlockDataviewViewUpdateSort{
				Operation: &pb.EventBlockDataviewViewUpdateSortOperationOfUpdate{
					Update: &pb.EventBlockDataviewViewUpdateSortUpdate{
						Id:   id,
						Item: item,
					},
				},
			}
		})
}

func (d *Dataview) ApplyViewUpdate(upd *pb.EventBlockDataviewViewUpdate) {
	var view *model.BlockContentDataviewView
	for _, v := range d.content.Views {
		if v.Id == upd.ViewId {
			view = v
			break
		}
	}
	if view == nil {
		return
	}

	if f := upd.Fields; f != nil {
		view.Type = f.Type
		view.Name = f.Name
		view.CoverRelationKey = f.CoverRelationKey
		view.HideIcon = f.HideIcon
		view.CardSize = f.CardSize
		view.CoverFit = f.CoverFit
		view.GroupRelationKey = f.GroupRelationKey
		view.GroupBackgroundColors = f.GroupBackgroundColors
		view.PageLimit = f.PageLimit
		view.DefaultTemplateId = f.DefaultTemplateId
		view.DefaultObjectTypeId = f.DefaultObjectTypeId
		view.EndRelationKey = f.EndRelationKey
		view.WrapContent = f.WrapContent
	}

	{
		var changes []slice.Change[*model.BlockContentDataviewRelation]
		for _, r := range upd.Relation {
			if v := r.GetUpdate(); v != nil {
				changes = append(changes, slice.MakeChangeReplace(v.Item, v.Id))
			} else if v := r.GetAdd(); v != nil {
				changes = append(changes, slice.MakeChangeAdd(v.Items, v.AfterId))
			} else if v := r.GetRemove(); v != nil {
				changes = append(changes, slice.MakeChangeRemove[*model.BlockContentDataviewRelation](v.Ids))
			} else if v := r.GetMove(); v != nil {
				changes = append(changes, slice.MakeChangeMove[*model.BlockContentDataviewRelation](v.Ids, v.AfterId))
			}
		}
		view.Relations = slice.ApplyChanges(view.Relations, changes, getViewRelationID)
	}
	{
		var changes []slice.Change[*model.BlockContentDataviewFilter]
		for _, r := range upd.Filter {
			if v := r.GetUpdate(); v != nil {
				changes = append(changes, slice.MakeChangeReplace(v.Item, v.Id))
			} else if v := r.GetAdd(); v != nil {
				changes = append(changes, slice.MakeChangeAdd(v.Items, v.AfterId))
			} else if v := r.GetRemove(); v != nil {
				changes = append(changes, slice.MakeChangeRemove[*model.BlockContentDataviewFilter](v.Ids))
			} else if v := r.GetMove(); v != nil {
				changes = append(changes, slice.MakeChangeMove[*model.BlockContentDataviewFilter](v.Ids, v.AfterId))
			}
		}
		view.Filters = slice.ApplyChanges(view.Filters, changes, getViewFilterID)
	}
	{
		var changes []slice.Change[*model.BlockContentDataviewSort]
		for _, r := range upd.Sort {
			if v := r.GetUpdate(); v != nil {
				changes = append(changes, slice.MakeChangeReplace(v.Item, v.Id))
			} else if v := r.GetAdd(); v != nil {
				changes = append(changes, slice.MakeChangeAdd(v.Items, v.AfterId))
			} else if v := r.GetRemove(); v != nil {
				changes = append(changes, slice.MakeChangeRemove[*model.BlockContentDataviewSort](v.Ids))
			} else if v := r.GetMove(); v != nil {
				changes = append(changes, slice.MakeChangeMove[*model.BlockContentDataviewSort](v.Ids, v.AfterId))
			}
		}
		view.Sorts = slice.ApplyChanges(view.Sorts, changes, getViewSortID)
	}
}

func diffViewObjectOrder(a, b *model.BlockContentDataviewObjectOrder) []*pb.EventBlockDataviewSliceChange {
	diff := slice.Diff(a.ObjectIds, b.ObjectIds, slice.StringIdentity[string], slice.Equal[string])
	if len(diff) == 0 {
		return nil
	}

	var res []*pb.EventBlockDataviewSliceChange
	for _, sliceCh := range diff {
		if add := sliceCh.Add(); add != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:      pb.EventBlockDataview_SliceOperationAdd,
				Ids:     add.Items,
				AfterId: add.AfterID,
			})
		}
		if move := sliceCh.Move(); move != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:      pb.EventBlockDataview_SliceOperationMove,
				Ids:     move.IDs,
				AfterId: move.AfterID,
			})
		}
		if rm := sliceCh.Remove(); rm != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:  pb.EventBlockDataview_SliceOperationRemove,
				Ids: rm.IDs,
			})
		}

		// Replace change is not supported
	}

	return res
}

func (d *Dataview) ApplyObjectOrderUpdate(upd *pb.EventBlockDataviewObjectOrderUpdate) {
	var existOrder []string
	for _, order := range d.Model().GetDataview().ObjectOrders {
		if order.ViewId == upd.ViewId && order.GroupId == upd.GroupId {
			existOrder = order.ObjectIds
		}
	}

	rawChanges := upd.GetSliceChanges()

	changes := make([]slice.Change[string], 0, len(rawChanges))
	for _, eventCh := range rawChanges {
		var ch slice.Change[string]
		switch eventCh.Op {
		case pb.EventBlockDataview_SliceOperationAdd:
			ch = slice.MakeChangeAdd(eventCh.Ids, eventCh.AfterId)
		case pb.EventBlockDataview_SliceOperationMove:
			ch = slice.MakeChangeMove[string](eventCh.Ids, eventCh.AfterId)
		case pb.EventBlockDataview_SliceOperationRemove:
			ch = slice.MakeChangeRemove[string](eventCh.Ids)
		case pb.EventBlockDataview_SliceOperationReplace:
			// Replace change is not supported
		}
		changes = append(changes, ch)
	}

	changedIds := slice.ApplyChanges(existOrder, changes, slice.StringIdentity[string])

	d.SetViewObjectOrder([]*model.BlockContentDataviewObjectOrder{
		{ViewId: upd.ViewId, GroupId: upd.GroupId, ObjectIds: changedIds},
	})
}
