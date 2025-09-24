package order

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "core.order.setter"

type OrderSetter interface {
	SetSpaceViewOrder(spaceViewOrder []string) ([]string, error)
	SetOptionsOrder(spaceId string, relationKey domain.RelationKey, order []string) ([]string, error)
	SetObjectTypesOrder(spaceId string, objectIds []string) ([]string, error)
	UnsetOrder(objectId string) error

	app.Component
}

type orderSetter struct {
	objectGetter        cache.ObjectGetter
	store               objectstore.ObjectStore
	techSpaceIdProvider objectstore.TechSpaceIdProvider
}

func New() OrderSetter {
	return &orderSetter{}
}

func (o *orderSetter) Init(a *app.App) (err error) {
	o.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	o.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	o.techSpaceIdProvider = app.MustComponent[objectstore.TechSpaceIdProvider](a)
	return
}

func (o *orderSetter) Name() (name string) {
	return CName
}

// SetSpaceViewOrder sets the order for space views. It ensures all views in spaceViewOrder have lexids.
// spaceViewOrder is the desired final order of all space views
func (o *orderSetter) SetSpaceViewOrder(objectIds []string) ([]string, error) {
	if len(objectIds) == 0 {
		return nil, errors.New("empty objectIds")
	}

	existing, err := o.getCurrentSpaceOrder()
	if err != nil {
		return nil, err
	}

	return o.rebuildIfNeeded(objectIds, existing)
}

// SetOptionsOrder sets the order for relation options of the particular relation. It ensures all options in order have lexids.
// order is the desired final order of all space views
func (o *orderSetter) SetOptionsOrder(spaceId string, relationKey domain.RelationKey, objectIds []string) ([]string, error) {
	if len(objectIds) == 0 {
		return nil, errors.New("empty objectIds")
	}

	existing, err := o.getCurrentOptionsOrder(spaceId, relationKey)
	if err != nil {
		return nil, err
	}

	return o.rebuildIfNeeded(objectIds, existing)
}

func (o *orderSetter) SetObjectTypesOrder(spaceId string, objectIds []string) ([]string, error) {
	if len(objectIds) == 0 {
		return nil, errors.New("empty objectIds")
	}

	existing, err := o.getCurrentTypesOrder(spaceId)
	if err != nil {
		return nil, err
	}

	previousIds := slices.Clone(objectIds)
	sort.Slice(previousIds, func(i, j int) bool {
		orderI, orderJ := existing[previousIds[i]], existing[previousIds[j]]
		return orderI < orderJ
	})

	res := slice.Diff(previousIds, objectIds, slice.StringIdentity, func(s string, s2 string) bool {
		return s == s2
	})

	move := func(id string, afterId string) {
		afterIdx := slices.Index(previousIds, afterId)
		if afterIdx == 0 {
			if len(previousIds) == 1 {
				newId := o.setRank(id, "", "", true)
				existing[id] = newId
			} else {
				next := existing[previousIds[1]]
				newId := o.setRank(id, "", next, true)
				existing[id] = newId
			}
		} else if afterIdx == len(previousIds)-1 {
			last := existing[previousIds[len(previousIds)-1]]
			newId := o.setRank(id, last, "", true)
			existing[id] = newId
		} else {
			left := existing[previousIds[afterIdx]]
			right := existing[previousIds[afterIdx+1]]
			newId := o.setRank(id, left, right, false)
			existing[id] = newId
		}
	}

	for _, ch := range res {
		// if add := ch.Add(); add != nil {
		// 	afterId := add.AfterID
		// 	for _, id := range add.Items {
		// 		move(id, afterId)
		// 		afterIdx := slices.Index(previousIds, afterId)
		// 		previousIds = slices.Insert(previousIds, afterIdx+1, id)
		// 		afterId = id
		// 	}
		// }
		if mv := ch.Move(); mv != nil {
			afterId := mv.AfterID
			for _, id := range mv.IDs {
				move(id, afterId)
				afterId = id
			}
		}
	}

	newOrderIds := make([]string, len(objectIds))
	for i, id := range objectIds {
		newOrderIds[i] = existing[id]
	}

	return newOrderIds, nil

	// return o.rebuildIfNeeded(objectIds, existing)
}

func (o *orderSetter) UnsetOrder(objectId string) error {
	return cache.Do[order.OrderSettable](o.objectGetter, objectId, func(os order.OrderSettable) error {
		return os.UnsetOrder()
	})
}

func (o *orderSetter) getCurrentSpaceOrder() (map[string]string, error) {
	// Get the current order of space views
	techSpaceId := o.techSpaceIdProvider.TechSpaceId()

	viewIdToLexId := make(map[string]string)
	err := o.store.SpaceIndex(techSpaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_spaceView),
		},
	}}, func(details *domain.Details) {
		orderId := details.GetString(bundle.RelationKeySpaceOrder)
		viewIdToLexId[details.GetString(bundle.RelationKeyId)] = orderId
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get current space order: %w", err)
	}

	return viewIdToLexId, nil
}

func (o *orderSetter) getCurrentOptionsOrder(spaceId string, relationKey domain.RelationKey) (map[string]string, error) {
	optionIdToOrderId := make(map[string]string)
	err := o.store.SpaceIndex(spaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_relationOption),
		},
		{
			RelationKey: bundle.RelationKeyRelationKey,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(relationKey.String()),
		},
	}}, func(details *domain.Details) {
		orderId := details.GetString(bundle.RelationKeyOrderId)
		optionIdToOrderId[details.GetString(bundle.RelationKeyId)] = orderId
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get current space order: %w", err)
	}

	return optionIdToOrderId, nil
}

func (o *orderSetter) getCurrentTypesOrder(spaceId string) (map[string]string, error) {
	objectIdToOrderId := make(map[string]string)
	err := o.store.SpaceIndex(spaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_objectType),
		},
	}}, func(details *domain.Details) {
		id := details.GetString(bundle.RelationKeyId)
		orderId := details.GetString(bundle.RelationKeyOrderId)
		objectIdToOrderId[id] = orderId
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get current space order: %w", err)
	}

	return objectIdToOrderId, nil
}

// rebuildIfNeeded processes the order in a single pass, updating lexids as needed
func (o *orderSetter) rebuildIfNeeded(objectIds []string, existing map[string]string) ([]string, error) {
	nextExisting := o.precalcNext(existing, objectIds) // O(n)
	prev := ""
	out := make([]string, len(objectIds))

	for i, id := range objectIds {
		curr := existing[id]
		next := nextExisting[i]

		switch {
		case curr != "" && (prev == "" || curr > prev) && (next == "" || curr < next):
			// rank already valid - no change needed
			out[i] = curr
		case i == 0:
			curr = o.setRank(id, "", next, true)
		default:
			// Insert between prev and next
			curr = o.setRank(id, prev, next, false)
		}

		if curr == "" {
			// setRank failed → full rebuild
			return o.rebuildAllLexIds(objectIds)
		}
		out[i] = curr
		prev = curr
	}
	return out, nil
}

// setRank sets the lexid for a view, handling all positioning cases
func (o *orderSetter) setRank(objectId, before, after string, isFirst bool) string {
	var newOrderId string
	err := cache.Do[order.OrderSettable](o.objectGetter, objectId, func(os order.OrderSettable) error {
		var err error
		switch {
		case isFirst && before == "" && after == "":
			// First element with no constraints - add padding
			newOrderId, err = os.SetOrder("")

		case before == "" && after == "":
			// Not first, but no constraints
			newOrderId, err = os.SetOrder("")

		case before == "" && after != "":
			// Insert before the first existing element
			newOrderId, err = os.SetBetweenOrders("", after)

		case before != "" && after == "":
			// Insert after the last element
			newOrderId, err = os.SetOrder(before)

		default:
			// Insert between two elements
			newOrderId, err = os.SetBetweenOrders(before, after)
		}
		return err
	})
	if err != nil {
		// Log error for debugging but return empty string to trigger rebuild
		return ""
	}
	return newOrderId
}

// precalcNext builds a slice where next[i] is the lexid of the next
// element *to the right* that already has a rank.
func (o *orderSetter) precalcNext(existing map[string]string, order []string) []string {
	res := make([]string, len(order))

	for i := 0; i < len(order)-1; i++ {
		next := order[i+1]
		res[i] = existing[next]
	}
	return res
}

// rebuildAllLexIds rebuilds all lexids from scratch
func (o *orderSetter) rebuildAllLexIds(objectIds []string) ([]string, error) {
	finalOrder := make([]string, len(objectIds))
	// Now assign new lexids in order
	previousLexId := ""
	for i, objectId := range objectIds {
		var newLexId string
		err := cache.Do[order.OrderSettable](o.objectGetter, objectId, func(os order.OrderSettable) error {
			var err error
			if i == 0 {
				// First element with padding
				newLexId, err = os.SetOrder("")
			} else {
				// Subsequent elements
				newLexId, err = os.SetOrder(previousLexId)
			}
			if err == nil && newLexId == "" {
				newLexId = os.GetOrder()
			}
			return err
		})

		if err != nil {
			return nil, fmt.Errorf("failed to set lexid for object %s at position %d: %w", objectId, i, err)
		}

		finalOrder[i] = newLexId
		previousLexId = newLexId
	}
	return finalOrder, nil
}
