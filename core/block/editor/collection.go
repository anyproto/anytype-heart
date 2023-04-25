package editor

import (
	"github.com/anytypeio/any-sync/app"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Collection struct {
	*Set
}

func NewCollection(
	anytype core.Service,
	objectStore objectstore.ObjectStore,
	relationService relation2.Service,
) *Collection {
	return &Collection{NewSet(anytype, objectStore, relationService)}
}

func (c *Collection) Init(ctx *smartblock.InitContext) (err error) {
	err = c.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	// TODO clean up
	type collectionService interface {
		RegisterCollection(sb smartblock.SmartBlock)
	}
	colService := app.MustComponent[collectionService](ctx.App)
	colService.RegisterCollection(c.SmartBlock)

	templates := []template.StateTransformer{
		template.WithDefaultFeaturedRelations,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyCollection.URL()}, model.ObjectType_collection),
		template.WithBlockEditRestricted(c.Id()),
		template.WithTitle,
		template.WithForcedDescription,
	}

	return smartblock.ObjectApplyTemplate(c, ctx.State, templates...)
}
