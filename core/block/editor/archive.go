package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewArchive(m meta.Service) *Archive {
	sb := smartblock.New(m)
	return &Archive{
		SmartBlock: sb,
		Collection: collection.NewCollection(sb),
	}
}

type Archive struct {
	smartblock.SmartBlock
	collection.Collection
}

func (p *Archive) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.SmartBlock.DisableLayouts()
	return smartblock.ApplyTemplate(p, ctx.State, template.WithEmpty, template.WithNoDuplicateLinks(), template.WithNoObjectTypes(), template.WithDetailName("Archive"), template.WithDetailIconEmoji("🗑"))
}

func (p *Archive) Relations() []*model.Relation {
	return nil
}
