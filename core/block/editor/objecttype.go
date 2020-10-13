package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func NewObjectType(m meta.Service, importServices _import.Services) *ObjectType {
	sb := smartblock.New(m, objects.BundledObjectTypeURLPrefix+"objectType")
	return &ObjectType{
		SmartBlock: sb,
	}
}

type ObjectType struct {
	smartblock.SmartBlock
}

func (p *ObjectType) Init(s source.Source, allowEmpty bool, objectTypeUrls []string) (err error) {
	if err = p.SmartBlock.Init(s, true, nil); err != nil {
		return
	}
	return template.ApplyTemplate(p, nil, template.WithEmpty, template.WithObjectTypes([]string{p.DefaultObjectTypeUrl()}))
}
