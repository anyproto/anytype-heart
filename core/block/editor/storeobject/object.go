package storeobject

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
)

type StoreObject interface {
	smartblock.SmartBlock
}

type storeObject struct {
	smartblock.SmartBlock

	store *storestate.StoreState
}

func New(sb smartblock.SmartBlock) StoreObject {
	return &storeObject{SmartBlock: sb}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	err = storeSource.ReadStoreDoc(ctx.Ctx)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	s.store = storeSource.GetStore()

	return nil
}
