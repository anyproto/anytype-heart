package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
)

type ObjectType struct {
	smartblock.SmartBlock
}

func NewObjectType(m meta.Service) *ObjectType {
	sb := smartblock.New(m)
	return &ObjectType{
		SmartBlock: sb,
	}
}

func (p *ObjectType) Init(s source.Source, _ bool, _ []string) (err error) {
	if err = p.SmartBlock.Init(s, true, nil); err != nil {
		return
	}
	return template.ApplyTemplate(p, nil,
		template.WithEmpty,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyObjectType.URL()}))
}
