package dataview

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

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
	// TODO temp
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
