package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

const defaultObjectTypeForObjectType = objects.BundledObjectTypeURLPrefix + "objectType"

type ObjectType struct {
	smartblock.SmartBlock
}

func (p *ObjectType) Init(s source.Source, _ bool, _ []string) (err error) {
	if err = p.SmartBlock.Init(s, true, []string{defaultObjectTypeForObjectType}); err != nil {
		return
	}
	return template.ApplyTemplate(p, nil, template.WithEmpty, template.WithObjectTypes([]string{defaultObjectTypeForObjectType}))
}
