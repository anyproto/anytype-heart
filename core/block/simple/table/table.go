package table

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func init() {
	simple.RegisterCreator(NewTable)
}

func NewTable(b *model.Block) simple.Block {
	if c := b.GetTable(); c != nil {
		return &table{
			Base:    base.NewBase(b).(*base.Base),
			content: c,
		}
	}
	return nil
}

type Table interface {
	simple.Block
}

type table struct {
	*base.Base
	content *model.BlockContentTable
}
