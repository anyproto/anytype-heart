package dataview

import (
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (d *Dataview) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	other, ok := b.(*Dataview)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = d.Base.Diff(spaceId, other); err != nil {
		return
	}

	msgs = d.diffGroupOrders(spaceId, other, msgs)
	msgs = d.diffObjectOrders(spaceId, other, msgs)
	msgs = d.diffRelationLinks(spaceId, other, msgs)
	msgs = d.diffViews(spaceId, other, msgs)
	msgs = d.diffSources(spaceId, other, msgs)
	msgs = d.diffOrderOfViews(spaceId, other, msgs)
	msgs = d.diffTargetObjectIDs(spaceId, other, msgs)
	msgs = d.diffIsCollections(spaceId, other, msgs)

	return
}

func (d *Dataview) diffIsCollections(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	if other.content.IsCollection != d.content.IsCollection {
		msgs = append(msgs,
			simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewIsCollectionSet{
				BlockDataviewIsCollectionSet: &pb.EventBlockDataviewIsCollectionSet{
					Id:    other.Id,
					Value: other.content.IsCollection,
				}},
			)})
	}
	return msgs
}

func (d *Dataview) diffTargetObjectIDs(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	if other.content.TargetObjectId != d.content.TargetObjectId {
		msgs = append(msgs,
			simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
				BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
					Id:             other.Id,
					TargetObjectId: other.content.TargetObjectId,
				}},
			)})
	}
	return msgs
}

func (d *Dataview) diffSources(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	if !slice.UnsortedEqual(other.content.Source, d.content.Source) {
		msgs = append(msgs,
			simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewSourceSet{
				&pb.EventBlockDataviewSourceSet{
					Id:     other.Id,
					Source: other.content.Source,
				}})})
	}
	return msgs
}

func (d *Dataview) diffViews(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	// @TODO: rewrite for optimised compare
	for _, view2 := range other.content.Views {
		var found bool
		var (
			viewFilterChanges   []*pb.EventBlockDataviewViewUpdateFilter
			viewRelationChanges []*pb.EventBlockDataviewViewUpdateRelation
			viewSortChanges     []*pb.EventBlockDataviewViewUpdateSort
			viewFieldsChange    *pb.EventBlockDataviewViewUpdateFields
		)

		for _, view1 := range d.content.Views {
			if view1.Id == view2.Id {
				found = true

				viewFieldsChange = diffViewFields(view1, view2)
				viewFilterChanges = diffViewFilters(view1, view2)
				viewRelationChanges = diffViewRelations(view1, view2)
				viewSortChanges = diffViewSorts(view1, view2)

				break
			}
		}

		if len(viewFilterChanges) > 0 || len(viewRelationChanges) > 0 || len(viewSortChanges) > 0 || viewFieldsChange != nil {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewViewUpdate{
						BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
							Id:       other.Id,
							ViewId:   view2.Id,
							Fields:   viewFieldsChange,
							Filter:   viewFilterChanges,
							Relation: viewRelationChanges,
							Sort:     viewSortChanges,
						},
					})})
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewViewSet{
						&pb.EventBlockDataviewViewSet{
							Id:     other.Id,
							ViewId: view2.Id,
							View:   view2,
						}})})
		}
	}

	for _, view1 := range d.content.Views {
		var found bool
		for _, view2 := range other.content.Views {
			if view1.Id == view2.Id {
				found = true
				break
			}
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewViewDelete{
					&pb.EventBlockDataviewViewDelete{
						Id:     other.Id,
						ViewId: view1.Id,
					}})})
		}
	}
	return msgs
}

func (d *Dataview) diffRelationLinks(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	added, removed := pbtypes.RelationLinks(other.content.RelationLinks).Diff(d.content.RelationLinks)
	if len(removed) > 0 {
		msgs = append(msgs, simple.EventMessage{
			Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewRelationDelete{
				BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
					Id:           other.Id,
					RelationKeys: removed,
				},
			}),
		})
	}
	if len(added) > 0 {
		msgs = append(msgs, simple.EventMessage{
			Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewRelationSet{
				BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
					Id:            other.Id,
					RelationLinks: added,
				},
			}),
		})
	}
	return msgs
}

func (d *Dataview) diffOrderOfViews(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	var viewIds1, viewIds2 []string
	for _, v := range d.content.Views {
		viewIds1 = append(viewIds1, v.Id)
	}
	for _, v := range other.content.Views {
		viewIds2 = append(viewIds2, v.Id)
	}
	if !slice.SortedEquals(viewIds1, viewIds2) {
		msgs = append(msgs,
			simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataviewViewOrder{
				&pb.EventBlockDataviewViewOrder{
					Id:      other.Id,
					ViewIds: viewIds2,
				}})})
	}
	return msgs
}

func (d *Dataview) diffObjectOrders(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	for _, order2 := range other.content.ObjectOrders {
		var found bool
		var changes []*pb.EventBlockDataviewSliceChange
		for _, order1 := range d.content.ObjectOrders {
			if order1.ViewId == order2.ViewId && order1.GroupId == order2.GroupId {
				found = true
				changes = diffViewObjectOrder(order1, order2)
				break
			}
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
						&pb.EventBlockDataviewObjectOrderUpdate{
							Id:           other.Id,
							ViewId:       order2.ViewId,
							GroupId:      order2.GroupId,
							SliceChanges: []*pb.EventBlockDataviewSliceChange{{Op: pb.EventBlockDataview_SliceOperationAdd, Ids: order2.ObjectIds}},
						}})})
		}

		if len(changes) > 0 {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
						&pb.EventBlockDataviewObjectOrderUpdate{
							Id:           other.Id,
							ViewId:       order2.ViewId,
							GroupId:      order2.GroupId,
							SliceChanges: changes,
						}})})
		}
	}
	return msgs
}

func (d *Dataview) diffGroupOrders(spaceId string, other *Dataview, msgs []simple.EventMessage) []simple.EventMessage {
	for _, order2 := range other.content.GroupOrders {
		var found, changed bool
		for _, order1 := range d.content.GroupOrders {
			if order1.ViewId == order2.ViewId {
				found = true
				changed = !proto.Equal(order1, order2)
				break
			}
		}

		if !found || changed {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
						&pb.EventBlockDataviewGroupOrderUpdate{
							Id:         other.Id,
							GroupOrder: order2,
						}})})
		}
	}
	return msgs
}
