package storeobject

import "github.com/anyproto/anytype-heart/core/block/editor/smartblock"

type StoreObject interface {
	smartblock.SmartBlock
}

type storeObject struct {
	smartblock.SmartBlock
}

func New(sb smartblock.SmartBlock) StoreObject {
	return &storeObject{SmartBlock: sb}
}
