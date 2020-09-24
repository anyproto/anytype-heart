package editor

import (
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func NewObjectType(m meta.Service, importServices _import.Services) *ObjectType {
	sb := smartblock.New(m)
	return &ObjectType{
		SmartBlock: sb,
	}
}

type ObjectType struct {
	smartblock.SmartBlock
}

func (p *ObjectType) Init(s source.Source, _ bool) (err error) {
	return p.SmartBlock.Init(s, true)
}
