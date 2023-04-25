package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

type Collection struct {
	*Set

	collectionService CollectionService
	objectStore       objectstore.ObjectStore
}

type CollectionService interface {
	RegisterCollection(sb smartblock.SmartBlock)
}

func NewCollection(
	anytype core.Service,
	objectStore objectstore.ObjectStore,
	relationService relation.Service,
	collectionService CollectionService,
	sbtProvider typeprovider.SmartBlockTypeProvider,
) *Collection {
	return &Collection{
		Set:               NewSet(anytype, objectStore, relationService, sbtProvider),
		collectionService: collectionService,
		objectStore:       objectStore,
	}
}

func (c *Collection) Init(ctx *smartblock.InitContext) (err error) {
	err = c.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	c.collectionService.RegisterCollection(c.SmartBlock)

	templates := []template.StateTransformer{
		template.WithDefaultFeaturedRelations,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyCollection.URL()}, model.ObjectType_collection),
		template.WithBlockEditRestricted(c.Id()),
		template.WithTitle,
		template.WithForcedDescription,
	}

	return smartblock.ObjectApplyTemplate(c, ctx.State, templates...)
}
