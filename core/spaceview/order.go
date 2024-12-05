package spaceview

import (
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "core.spaceview.ordersetter"

type OrderSetter interface {
	SetOrder(spaceViewId string, spaceViewOrder []string) error
	UnsetOrder(spaceViewId string) error
	app.Component
}

type orderSetter struct {
	objectGetter cache.ObjectGetter
}

func New() OrderSetter {
	return &orderSetter{}
}

func (o *orderSetter) Init(a *app.App) (err error) {
	o.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	return
}

func (o *orderSetter) Name() (name string) {
	return CName
}

func (o *orderSetter) SetOrder(spaceViewId string, spaceViewOrder []string) error {
	if len(spaceViewOrder) < 1 {
		return fmt.Errorf("insufficient space views for reordering")
	}
	if len(spaceViewOrder) == 1 {
		return o.setInitOrder(spaceViewId)
	}
	// given space view is the first view in the order
	if spaceViewOrder[0] == spaceViewId {
		return o.setViewAtBeginning(spaceViewId, spaceViewOrder[1])
	}
	// given space view is the last view in the order
	if spaceViewOrder[len(spaceViewOrder)-1] == spaceViewId {
		return o.setViewAtEnd(spaceViewOrder, spaceViewId, spaceViewOrder[len(spaceViewOrder)-2])
	}
	return o.setBetween(spaceViewOrder, spaceViewId)
}

func (o *orderSetter) setInitOrder(spaceViewId string) error {
	return cache.Do[*editor.SpaceView](o.objectGetter, spaceViewId, func(sv *editor.SpaceView) error {
		_, err := sv.SetOrder("")
		return err
	})
}

func (o *orderSetter) UnsetOrder(spaceViewId string) error {
	return cache.Do(o.objectGetter, spaceViewId, func(sb smartblock.SmartBlock) error {
		state := sb.NewState()
		state.RemoveDetail(bundle.RelationKeySpaceOrder.String())
		return sb.Apply(state)
	})
}

func (o *orderSetter) setViewAtBeginning(spaceViewId, afterViewId string) error {
	var (
		nextOrderID string
		err         error
	)
	err = cache.Do[*editor.SpaceView](o.objectGetter, afterViewId, func(sv *editor.SpaceView) error {
		nextOrderID = pbtypes.GetString(sv.Details(), bundle.RelationKeySpaceOrder.String())
		return nil
	})
	if err != nil {
		return err
	}
	return cache.Do[*editor.SpaceView](o.objectGetter, spaceViewId, func(view *editor.SpaceView) error {
		if nextOrderID == "" {
			_, err = view.SetOrder("")
			return err
		}
		return view.SetBetweenViews("", nextOrderID)
	})
}

func (o *orderSetter) setViewAtEnd(order []string, spaceViewId, afterSpaceView string) error {
	var lastOrderId string
	// get the order for the previous view in the list.
	cacheErr := cache.Do[*editor.SpaceView](o.objectGetter, afterSpaceView, func(sv *editor.SpaceView) error {
		lastOrderId = pbtypes.GetString(sv.Details(), bundle.RelationKeySpaceOrder.String())
		return nil
	})
	if cacheErr != nil {
		return cacheErr
	}
	// if view doesn't have order in details, then set it for all previous ids
	if lastOrderId == "" {
		return o.setOrderForPreviousViews(order, spaceViewId)
	}
	return cache.Do[*editor.SpaceView](o.objectGetter, spaceViewId, func(sv *editor.SpaceView) error {
		return sv.SetAfterGivenView(lastOrderId)
	})
}

func (o *orderSetter) setBetween(order []string, spaceViewId string) error {
	prevViewId, nextViewId := o.findNeighborViews(order, spaceViewId)
	var prevOrderId, nextOrderId string
	cacheErr := cache.Do[*editor.SpaceView](o.objectGetter, prevViewId, func(sv *editor.SpaceView) error {
		prevOrderId = pbtypes.GetString(sv.Details(), bundle.RelationKeySpaceOrder.String())
		return nil
	})
	if cacheErr != nil {
		return cacheErr
	}
	if prevOrderId == "" {
		return o.setOrderForPreviousViews(order, spaceViewId)
	}
	cacheErr = cache.Do[*editor.SpaceView](o.objectGetter, nextViewId, func(sv *editor.SpaceView) error {
		nextOrderId = pbtypes.GetString(sv.Details(), bundle.RelationKeySpaceOrder.String())
		return nil
	})
	if cacheErr != nil {
		return cacheErr
	}
	return cache.Do[*editor.SpaceView](o.objectGetter, spaceViewId, func(view *editor.SpaceView) error {
		if nextOrderId == "" {
			return view.SetAfterGivenView(prevOrderId)
		}
		return view.SetBetweenViews(prevOrderId, nextOrderId)
	})
}

func (o *orderSetter) findNeighborViews(order []string, spaceViewId string) (string, string) {
	var prevViewId, nextViewId string
	for i, id := range order {
		if id == spaceViewId {
			prevViewId = order[i-1]
			nextViewId = order[i+1]
			break
		}
	}
	return prevViewId, nextViewId
}

func (o *orderSetter) setOrderForPreviousViews(order []string, spaceViewId string) error {
	var (
		prevOrderId string
		err         error
	)
	for _, id := range order {
		cacheErr := cache.Do[*editor.SpaceView](o.objectGetter, id, func(sv *editor.SpaceView) error {
			prevOrderId, err = sv.SetOrder(prevOrderId)
			if err != nil {
				return err
			}
			return nil
		})
		if cacheErr != nil {
			return cacheErr
		}
		if id == spaceViewId {
			break
		}
	}
	return nil
}
