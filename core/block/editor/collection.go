package editor

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
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
	err = c.Set.Init(ctx)
	if err != nil {
		return
	}

	// TODO clean up
	type collectionService interface {
		RegisterCollection(sb smartblock.SmartBlock)
	}
	colService := app.MustComponent[collectionService](ctx.App)
	colService.RegisterCollection(c.SmartBlock)
	return nil
}
