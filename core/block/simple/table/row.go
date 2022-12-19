package table

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

func (b *rowBlock) Diff(sb simple.Block) (msgs []simple.EventMessage, err error) {
	other, ok := sb.(*rowBlock)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = b.Base.Diff(other); err != nil {
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
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetTableRow{BlockSetTableRow: changes}}})
	}
	return
}

func (b *rowBlock) ApplyEvent(e *pb.EventBlockSetTableRow) (err error) {
	if e.IsHeader != nil {
		b.content.IsHeader = e.IsHeader.GetValue()
	}
	return
}
