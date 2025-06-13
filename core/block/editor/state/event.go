package state

import (
	"fmt"
	"slices"
	"sort"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/embed"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/simple/table"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/simple/widget"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *State) applyEvent(ev *pb.EventMessage) (err error) {
	var apply = func(id string, f func(b simple.Block) error) (err error) {
		if b := s.Get(id); b != nil {
			return f(b)
		}
		return fmt.Errorf("can't apply change: block not found")
	}
	switch o := ev.Value.(type) {
	case *pb.EventMessageValueOfBlockSetAlign:
		if err = apply(o.BlockSetAlign.Id, func(b simple.Block) error {
			b.Model().Align = o.BlockSetAlign.Align
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetVerticalAlign:
		if err = apply(o.BlockSetVerticalAlign.Id, func(b simple.Block) error {
			b.Model().VerticalAlign = o.BlockSetVerticalAlign.VerticalAlign
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetBackgroundColor:
		if err = apply(o.BlockSetBackgroundColor.Id, func(b simple.Block) error {
			b.Model().BackgroundColor = o.BlockSetBackgroundColor.BackgroundColor
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetBookmark:
		if err = apply(o.BlockSetBookmark.Id, func(b simple.Block) error {
			if bm, ok := b.(bookmark.Block); ok {
				return bm.ApplyEvent(o.BlockSetBookmark)
			}
			return fmt.Errorf("not a bookmark block")
		}); err != nil {
			return
		}

	case *pb.EventMessageValueOfBlockSetTableRow:
		if err = apply(o.BlockSetTableRow.Id, func(b simple.Block) error {
			if tr, ok := b.(table.RowBlock); ok {
				return tr.ApplyEvent(o.BlockSetTableRow)
			}
			return fmt.Errorf("not a table row block")
		}); err != nil {
			return
		}

	case *pb.EventMessageValueOfBlockSetDiv:
		if err = apply(o.BlockSetDiv.Id, func(b simple.Block) error {
			if d, ok := b.(base.DivBlock); ok {
				return d.ApplyEvent(o.BlockSetDiv)
			}
			return fmt.Errorf("not a div block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetText:
		if err = apply(o.BlockSetText.Id, func(b simple.Block) error {
			if t, ok := b.(text.Block); ok {
				return t.ApplyEvent(o.BlockSetText)
			}
			return fmt.Errorf("not a text block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetFields:
		if err = apply(o.BlockSetFields.Id, func(b simple.Block) error {
			b.Model().Fields = o.BlockSetFields.Fields
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetFile:
		if err = apply(o.BlockSetFile.Id, func(b simple.Block) error {
			if f, ok := b.(file.Block); ok {
				return f.ApplyEvent(o.BlockSetFile)
			}
			return fmt.Errorf("not a file block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetLink:
		if err = apply(o.BlockSetLink.Id, func(b simple.Block) error {
			if f, ok := b.(link.Block); ok {
				return f.ApplyEvent(o.BlockSetLink)
			}
			return fmt.Errorf("not a link block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewSourceSet:
		if err = apply(o.BlockDataviewSourceSet.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				return f.SetSource(o.BlockDataviewSourceSet.Source)
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewViewSet:
		if err = apply(o.BlockDataviewViewSet.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok && o.BlockDataviewViewSet.View != nil {
				if f.SetView(o.BlockDataviewViewSet.ViewId, *o.BlockDataviewViewSet.View) != nil {
					f.AddView(*o.BlockDataviewViewSet.View)
				}
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewViewOrder:
		if err = apply(o.BlockDataviewViewOrder.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				f.SetViewOrder(o.BlockDataviewViewOrder.ViewIds)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewViewDelete:
		if err = apply(o.BlockDataviewViewDelete.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				err := f.DeleteView(o.BlockDataviewViewDelete.ViewId)
				if err != nil && err != dataview.ErrViewNotFound {
					return err
				}
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewOldRelationSet:
		if err = apply(o.BlockDataviewOldRelationSet.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok && o.BlockDataviewOldRelationSet.Relation != nil {
				if er := f.UpdateRelationOld(o.BlockDataviewOldRelationSet.RelationKey, *o.BlockDataviewOldRelationSet.Relation); er == dataview.ErrRelationNotFound {
					rel := o.BlockDataviewOldRelationSet.Relation
					f.AddRelationOld(*rel)
					// MIGRATION: reinterpretation of old changes as new changes
					f.AddRelation(&model.RelationLink{
						Key:    rel.Key,
						Format: rel.Format,
					})
				} else {
					return er
				}
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}

	case *pb.EventMessageValueOfBlockDataviewOldRelationDelete:
		if err = apply(o.BlockDataviewOldRelationDelete.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				err = f.DeleteRelationOld(o.BlockDataviewOldRelationDelete.RelationKey)
				if err != nil {
					return err
				}
				// MIGRATION: reinterpretation of old changes as new changes
				f.DeleteRelations(o.BlockDataviewOldRelationDelete.RelationKey)
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewRelationSet:
		if err = apply(o.BlockDataviewRelationSet.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				for _, rel := range o.BlockDataviewRelationSet.RelationLinks {
					f.AddRelation(rel)
				}
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewRelationDelete:
		if err = apply(o.BlockDataviewRelationDelete.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				f.DeleteRelations(o.BlockDataviewRelationDelete.RelationKeys...)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetRelation:
		if err = apply(o.BlockSetRelation.Id, func(b simple.Block) error {
			if f, ok := b.(relation.Block); ok {
				return f.ApplyEvent(o.BlockSetRelation)
			}
			return fmt.Errorf("not a relation block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetLatex:
		if err = apply(o.BlockSetLatex.Id, func(b simple.Block) error {
			if f, ok := b.(embed.Block); ok {
				return f.ApplyEvent(o.BlockSetLatex)
			}
			return fmt.Errorf("not an embed block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataViewGroupOrderUpdate:
		if err = apply(o.BlockDataViewGroupOrderUpdate.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				f.SetViewGroupOrder(o.BlockDataViewGroupOrderUpdate.GroupOrder)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataViewObjectOrderUpdate:
		event := o.BlockDataViewObjectOrderUpdate
		if err = apply(event.Id, func(b simple.Block) error {
			if dvBlock, ok := b.(dataview.Block); ok {
				dvBlock.ApplyObjectOrderUpdate(event)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}

	case *pb.EventMessageValueOfBlockDataviewViewUpdate:
		ev := o.BlockDataviewViewUpdate
		if err = apply(ev.Id, func(b simple.Block) error {
			if dvBlock, ok := b.(dataview.Block); ok {
				dvBlock.ApplyViewUpdate(ev)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}

	case *pb.EventMessageValueOfBlockSetWidget:
		if err = apply(o.BlockSetWidget.Id, func(b simple.Block) error {
			if tr, ok := b.(widget.Block); ok {
				return tr.ApplyEvent(o.BlockSetWidget)
			}
			return fmt.Errorf("not a widget block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewTargetObjectIdSet:
		if err = apply(o.BlockDataviewTargetObjectIdSet.Id, func(b simple.Block) error {
			if dvBlock, ok := b.(dataview.Block); ok {
				dvBlock.SetTargetObjectID(o.BlockDataviewTargetObjectIdSet.TargetObjectId)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewIsCollectionSet:
		if err = apply(o.BlockDataviewIsCollectionSet.Id, func(b simple.Block) error {
			if dvBlock, ok := b.(dataview.Block); ok {
				dvBlock.SetIsCollection(o.BlockDataviewIsCollectionSet.Value)
				return nil
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	}

	return nil
}

func WrapEventMessages(virtual bool, msgs []*pb.EventMessage) []simple.EventMessage {
	var wmsgs []simple.EventMessage
	for i := range msgs {
		wmsgs = append(wmsgs, simple.EventMessage{
			Virtual: virtual,
			Msg:     msgs[i],
		})
	}
	return wmsgs
}

// StructDiffIntoEvents converts diff details and relation keys to unset into events
func StructDiffIntoEvents(spaceId string, contextId string, diff *domain.Details, keysToUnset []domain.RelationKey) (msgs []*pb.EventMessage) {
	return StructDiffIntoEventsWithSubIds(spaceId, contextId, diff, nil, keysToUnset, nil)
}

func StructDiffIntoEventsWithSubIds(
	spaceId, contextId string,
	diff *domain.Details,
	filterKeys, keysToUnset []domain.RelationKey,
	subIds []string,
) (msgs []*pb.EventMessage) {
	if diff.Len() == 0 && len(keysToUnset) == 0 {
		return nil
	}
	var (
		details []*pb.EventObjectDetailsAmendKeyValue
	)

	for k, v := range diff.Iterate() {
		key := string(k)
		if len(filterKeys) > 0 && slice.FindPos(filterKeys, k) == -1 {
			continue
		}
		details = append(details, &pb.EventObjectDetailsAmendKeyValue{Key: key, Value: v.ToProto()})
	}

	if len(details) > 0 {
		msgs = append(msgs, event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsAmend{
			ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
				Id:      contextId,
				Details: details,
				SubIds:  subIds,
			},
		}))
	}

	if len(filterKeys) != 0 {
		keysToUnset = slices.DeleteFunc(keysToUnset, func(key domain.RelationKey) bool {
			return !slices.Contains(filterKeys, key)
		})
	}

	if len(keysToUnset) > 0 {
		msgs = append(msgs, event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsUnset{
			ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
				Id:     contextId,
				Keys:   slice.IntoStrings(keysToUnset),
				SubIds: subIds,
			},
		}))
	}
	return
}

func sortEventMessages(msgs []simple.EventMessage) {
	eventGroup := func(msg simple.EventMessage) int {
		switch msg.Msg.Value.(type) {
		case *pb.EventMessageValueOfBlockAdd:
			return 0
		case *pb.EventMessageValueOfBlockDelete:
			return 1
		case *pb.EventMessageValueOfBlockSetChildrenIds:
			return 2
		case *pb.EventMessageValueOfObjectDetailsSet:
			return 3
		case *pb.EventMessageValueOfObjectDetailsAmend:
			return 4
		case *pb.EventMessageValueOfObjectDetailsUnset:
			return 5
		case *pb.EventMessageValueOfBlockDataviewViewSet:
			return 6
		case *pb.EventMessageValueOfBlockDataviewViewDelete:
			return 7
		default:
			return 1000
		}
	}

	sort.SliceStable(msgs, func(i, j int) bool {
		a, b := msgs[i], msgs[j]
		return eventGroup(a) < eventGroup(b)
	})
}
