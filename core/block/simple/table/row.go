package table

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewRowBlock)
}

func NewRowBlock(b *model.Block) simple.Block {
	if c := b.GetTableRow(); c != nil {
		return &rowBlock{
			Base:    base.NewBase(b).(*base.Base),
			content: c,
		}
	}
	return nil
}

type RowBlock interface {
	simple.Block
	ApplyEvent(e *pb.EventBlockSetTableRow) (err error)
	SetIsHeader(v bool)
}

type rowBlock struct {
	*base.Base
	content *model.BlockContentTableRow
}

func (b *rowBlock) Copy() simple.Block {
	return NewRowBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *rowBlock) SetIsHeader(v bool) {
	b.content.IsHeader = v
}

func (b *rowBlock) Diff(spaceId string, sb simple.Block) (msgs []simple.EventMessage, err error) {
	other, ok := sb.(*rowBlock)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = b.Base.Diff(spaceId, other); err != nil {
		return
	}
	changes := &pb.EventBlockSetTableRow{
		Id: other.Id,
	}
	hasChanges := false

	if b.content.IsHeader != other.content.IsHeader {
		hasChanges = true
		changes.IsHeader = &pb.EventBlockSetTableRowIsHeader{Value: other.content.IsHeader}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetTableRow{BlockSetTableRow: changes})})
	}
	return
}

func (b *rowBlock) ApplyEvent(e *pb.EventBlockSetTableRow) (err error) {
	if e.IsHeader != nil {
		b.content.IsHeader = e.IsHeader.GetValue()
	}
	return
}
