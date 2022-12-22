package dataview

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func diffViewFilters(a, b *model.BlockContentDataviewView) []*pb.EventBlockDataviewViewUpdateFilter {
	calcID := func(f *model.BlockContentDataviewFilter) string {
		// TODO temp
		return f.RelationKey
	}
	equal := func(a, b withID[*model.BlockContentDataviewFilter]) bool {
		if a.item.RelationKey != b.item.RelationKey {
			return false
		}
		if a.item.Condition != b.item.Condition {
			return false
		}
		return true
	}

	diff := slice.Diff(wrapWithIDs(a.Filters, calcID), wrapWithIDs(b.Filters, calcID), equal)
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
