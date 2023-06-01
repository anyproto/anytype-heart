package widget

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBlock)
}

func NewBlock(b *model.Block) simple.Block {
	if w := b.GetWidget(); w != nil {
		return &block{
			Base:    base.NewBase(b).(*base.Base),
			content: w,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	ApplyEvent(e *pb.EventBlockSetWidget) error
}

type block struct {
	*base.Base
	content *model.BlockContentWidget
}

func (b *block) Copy() simple.Block {
	return NewBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *block) Diff(ob simple.Block) (msgs []simple.EventMessage, err error) {
	other, ok := ob.(*block)
	if !ok {
		return nil, fmt.Errorf("can't make diff with incompatible block")
	}
	if msgs, err = b.Base.Diff(other); err != nil {
		return
	}

	var hasChanges bool
	changes := &pb.EventBlockSetWidget{
		Id: other.Id,
	}

	if b.content.Layout != other.content.Layout {
		hasChanges = true
		changes.Layout = &pb.EventBlockSetWidgetLayout{Value: other.content.Layout}
	}

	if b.content.Limit != other.content.Limit {
		hasChanges = true
		changes.Limit = &pb.EventBlockSetWidgetLimit{Value: other.content.Limit}
	}

	if b.content.ViewId != other.content.ViewId {
		hasChanges = true
		changes.ViewId = &pb.EventBlockSetWidgetViewId{Value: other.content.ViewId}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetWidget{BlockSetWidget: changes}}})
	}
	return
}

func (b *block) ApplyEvent(e *pb.EventBlockSetWidget) error {
	if e.Layout != nil {
		b.content.Layout = e.Layout.GetValue()
	}
	return nil
}
