package storeobject

import "github.com/anyproto/anytype-heart/core/block/editor/smartblock"

type StoreObject interface {
	smartblock.SmartBlock
	NewPState()
}
