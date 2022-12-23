package dataview

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func diffViewFields(a, b *model.BlockContentDataviewView) *pb.EventBlockDataviewViewUpdateFields {
	isEqual := a.Type == b.Type &&
		a.Name == b.Name &&
		a.CoverRelationKey == b.CoverRelationKey &&
		a.HideIcon == b.HideIcon &&
		a.CardSize == b.CardSize &&
		a.CoverFit == b.CoverFit &&
		a.GroupRelationKey == b.GroupRelationKey &&
		a.GroupBackgroundColors == b.GroupBackgroundColors

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
	}
}

func getViewFilterID(f *model.BlockContentDataviewFilter) string {
	// TODO temp
	return f.RelationKey
}

func isViewFiltersEqual(a, b *model.BlockContentDataviewFilter) bool {
	if a.RelationKey != b.RelationKey {
		return false
	}
	if a.Condition != b.Condition {
		return false
	}
	return true
}

func diffViewFilters(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateFilter {
	diff := slice.Diff(a.Filters, b.Filters, getViewFilterID, isViewFiltersEqual)
	if len(diff) == 0 {
		return nil
	}

	return unwrapChanges(
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
					&pb.EventBlockDataviewViewUpdateFilterMove{
						AfterId: afterID,
						Ids:     ids,
					},
				},
			}
		},
		func(id string, item *model.BlockContentDataviewFilter) *pb.EventBlockDataviewViewUpdateFilter {
			return &pb.EventBlockDataviewViewUpdateFilter{
				Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfUpdate{
					&pb.EventBlockDataviewViewUpdateFilterUpdate{
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
	if a.Key != b.Key {
		return false
	}
	if a.IsVisible != b.IsVisible {
		return false
	}
	return true
}

func diffViewRelations(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateRelation {
	diff := slice.Diff(a.Relations, b.Relations, getViewRelationID, isViewRelationsEqual)
	if len(diff) == 0 {
		return nil
	}

	return unwrapChanges(
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
	return f.RelationKey
}

func isViewSortsEqual(a, b *model.BlockContentDataviewSort) bool {
	if a.RelationKey != b.RelationKey {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	return true
}

func diffViewSorts(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateSort {
	diff := slice.Diff(a.Sorts, b.Sorts, getViewSortID, isViewSortsEqual)
	if len(diff) == 0 {
		return nil
	}

	return unwrapChanges(
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

func (l *Dataview) ApplyViewUpdate(upd *pb.EventBlockDataviewViewUpdate) {
	var view *model.BlockContentDataviewView
	for _, v := range l.content.Views {
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

func unwrapChanges[T, R any](
	changes []slice.Change[T],
	add func(afterID string, items []T) R,
	remove func(ids []string) R,
	move func(afterID string, ids []string) R,
	update func(id string, item T) R,
) []R {
	res := make([]R, 0, len(changes))
	for _, c := range changes {
		if v := c.Add(); v != nil {
			res = append(res, add(v.AfterId, v.Items))
		}
		if v := c.Remove(); v != nil {
			res = append(res, remove(v.IDs))
		}
		if v := c.Move(); v != nil {
			res = append(res, move(v.AfterId, v.IDs))
		}
		if v := c.Replace(); v != nil {
			res = append(res, update(v.ID, v.Item))
		}
	}
	return res
}
