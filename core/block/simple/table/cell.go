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
	simple.RegisterCreator(NewCell)
}

func NewCell(b *model.Block) simple.Block {
	if c := b.GetTableCell(); c != nil {
		return &cell{
			Base:    base.NewBase(b).(*base.Base),
			content: c,
		}
	}
	return nil
}

type Cell interface {
	simple.Block
	ApplyEvent(e *pb.EventBlockSetTableCell) (err error)
	SetVerticalAlign(v model.BlockContentTableCellAlign)
}

type cell struct {
	*base.Base
	content *model.BlockContentTableCell
}

func (c *cell) SetVerticalAlign(v model.BlockContentTableCellAlign) {
	c.content.VerticalAlign = v
}

func (c *cell) Copy() simple.Block {
	return NewCell(pbtypes.CopyBlock(c.Model()))
}

// Validate TODO: add validation rules
func (c *cell) Validate() error {
	return nil
}

func (c *cell) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	other, ok := b.(*cell)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = c.Base.Diff(other); err != nil {
		return
	}
	changes := &pb.EventBlockSetTableCell{
		Id: other.Id,
	}
	hasChanges := false

	if c.content.VerticalAlign != other.content.VerticalAlign {
		hasChanges = true
		changes.VerticalAlign = &pb.EventBlockSetTableCellAlign{Value: other.content.VerticalAlign}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetTableCell{BlockSetTableCell: changes}}})
	}
	return
}

func (c *cell) ApplyEvent(e *pb.EventBlockSetTableCell) (err error) {
	if e.VerticalAlign != nil {
		c.content.VerticalAlign = e.VerticalAlign.Value
	}
	return nil
}
