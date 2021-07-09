package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
)

type Relation struct {
	*Set
}

func NewRelation(m meta.Service, dbCtrl database.Ctrl) *Relation {
	return &Relation{
		Set: NewSet(m, dbCtrl),
	}
}

func (p *Relation) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	return template.ApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyRelation.URL()}),
	)
}
