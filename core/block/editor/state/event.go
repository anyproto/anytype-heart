package state

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple/latex"
	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
				return f.DeleteView(o.BlockDataviewViewDelete.ViewId)
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewRelationSet:
		if err = apply(o.BlockDataviewRelationSet.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok && o.BlockDataviewRelationSet.Relation != nil {
				if er := f.UpdateRelation(o.BlockDataviewRelationSet.RelationKey, *o.BlockDataviewRelationSet.Relation); er == dataview.ErrRelationNotFound {
					f.AddRelation(*o.BlockDataviewRelationSet.Relation)
				} else {
					return er
				}
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDataviewRelationDelete:
		if err = apply(o.BlockDataviewRelationDelete.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				return f.DeleteRelation(o.BlockDataviewRelationDelete.RelationKey)
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
			if f, ok := b.(latex.Block); ok {
				return f.ApplyEvent(o.BlockSetLatex)
			}
			return fmt.Errorf("not a latex block")
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

func StructDiffIntoEvents(contextId string, diff *types.Struct) (msgs []*pb.EventMessage) {
	return StructDiffIntoEventsWithSubIds(contextId, diff, nil, nil)
}

// StructDiffIntoEvents converts map into events. nil map value converts to Remove event
func StructDiffIntoEventsWithSubIds(contextId string, diff *types.Struct, keys []string, subIds []string) (msgs []*pb.EventMessage) {
	if diff == nil || len(diff.Fields) == 0 {
		return nil
	}
	var (
		removed []string
		details []*pb.EventObjectDetailsAmendKeyValue
	)

	for k, v := range diff.Fields {
		if len(keys) > 0 && slice.FindPos(keys, k) == -1 {
			continue
		}
		if v == nil {
			removed = append(removed, k)
			continue
		}
		details = append(details, &pb.EventObjectDetailsAmendKeyValue{Key: k, Value: v})
	}
	if len(details) > 0 {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfObjectDetailsAmend{
				ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
					Id:      contextId,
					Details: details,
					SubIds: subIds,
				},
			},
		})
	}
	if len(removed) > 0 {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfObjectDetailsUnset{
				ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
					Id:   contextId,
					Keys: removed,
					SubIds: subIds,
				},
			},
		})
	}

	return
}
