package spaceview

import "github.com/anyproto/anytype-heart/core/block/cache"

type OrderSetter interface {
	SetSpaceViewOrder(spaceViewId string, order []string) error
}

type orderSetter struct {
	og cache.ObjectGetter
}
