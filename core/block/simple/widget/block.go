package widget

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
