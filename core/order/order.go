package order

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.order.setter"

type orderSettable interface {
	GetOrder() string
	SetOrder(previousOrderId string) (string, error)
	SetAfterOrder(orderId string) error
	SetBetweenOrders(previousOrderId, afterOrderId string) error
	UnsetOrder() error
}

type OrderSetter interface {
	SetSpaceViewOrder(spaceViewOrder []string) ([]string, error)
	UnsetOrder(objectId string) error

	SetOptionsOrder(spaceId string, relationKey domain.RelationKey, order []string) ([]string, error)

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

func (o *orderSetter) getCurrentSpaceOrder() (map[string]string, error) {
	// Get the current order of space views
	techSpaceId := o.techSpaceIdProvider.TechSpaceId()

	viewIdToLexId := make(map[string]string)
	err := o.store.SpaceIndex(techSpaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyLayout,
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
			RelationKey: bundle.RelationKeyLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_relationOption),
		},
		{
			RelationKey: bundle.RelationKeyRelationKey,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(relationKey.String()),
		},
	}}, func(details *domain.Details) {
		orderId := details.GetString(bundle.RelationKeyOptionOrder)
		optionIdToOrderId[details.GetString(bundle.RelationKeyId)] = orderId
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get current space order: %w", err)
	}

	return optionIdToOrderId, nil
}

// SetSpaceViewOrder sets the order for space views. It ensures all views in spaceViewOrder have lexids.
// spaceViewOrder is the desired final order of all space views
func (o *orderSetter) SetSpaceViewOrder(spaceViewOrder []string) ([]string, error) {
	if len(spaceViewOrder) == 0 {
		return nil, errors.New("empty spaceViewOrder")
	}

	existing, err := o.getCurrentSpaceOrder()
	if err != nil {
		return nil, err
	}

	return o.rebuildIfNeeded(spaceViewOrder, existing)
}

// SetOptionsOrder sets the order for relation options of particular relation. It ensures all options in order have lexids.
// order is the desired final order of all space views
func (o *orderSetter) SetOptionsOrder(spaceId string, relationKey domain.RelationKey, order []string) ([]string, error) {
	if len(order) == 0 {
		return nil, errors.New("empty order")
	}

	existing, err := o.getCurrentOptionsOrder(spaceId, relationKey)
	if err != nil {
		return nil, err
	}

	return o.rebuildIfNeeded(order, existing)
}

// rebuildIfNeeded processes the order in a single pass, updating lexids as needed
func (o *orderSetter) rebuildIfNeeded(order []string, existing map[string]string) ([]string, error) {
	nextExisting := o.precalcNext(existing, order) // O(n)
	prev := ""
	out := make([]string, len(order))

	for i, id := range order {
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
			return o.rebuildAllLexIds(order)
		}
		out[i] = curr
		prev = curr
	}
	return out, nil
}

// setRank sets the lexid for a view, handling all positioning cases
func (o *orderSetter) setRank(objectId, before, after string, isFirst bool) string {
	var newID string
	err := cache.Do[orderSettable](o.objectGetter, objectId, func(os orderSettable) error {
		var e error
		switch {
		case isFirst && before == "" && after == "":
			// First element with no constraints - add padding
			newID, e = os.SetOrder("")
		case before == "" && after == "":
			// Not first, but no constraints
			newID, e = os.SetOrder("")
		case before == "" && after != "":
			// Insert before the first existing element
			e = os.SetBetweenOrders("", after)
		case before != "" && after == "":
			// Insert after the last element
			newID, e = os.SetOrder(before)
		default:
			// Insert between two elements
			e = os.SetBetweenOrders(before, after)
		}

		// Read the lexid from details if not returned directly
		if e == nil && newID == "" {
			newID = os.GetOrder()
		}
		return e
	})
	if err != nil {
		// Log error for debugging but return empty string to trigger rebuild
		return ""
	}
	return newID
}

// precalcNext builds a slice where next[i] is the lexid of the next
// element *to the right* that already has a rank.
func (o *orderSetter) precalcNext(existing map[string]string, order []string) []string {
	res := make([]string, len(order))
	next := ""
	for i := len(order) - 1; i >= 0; i-- {
		res[i] = next
		if lex := existing[order[i]]; lex != "" {
			next = lex
		}
	}
	return res
}

func (o *orderSetter) UnsetOrder(objectId string) error {
	return cache.Do[orderSettable](o.objectGetter, objectId, func(os orderSettable) error {
		return os.UnsetOrder()
	})
}

// rebuildAllLexIds rebuilds all lexids from scratch
func (o *orderSetter) rebuildAllLexIds(order []string) ([]string, error) {
	finalOrder := make([]string, len(order))

	// Clear all existing lexids first
	for _, objectId := range order {
		err := cache.Do[orderSettable](o.objectGetter, objectId, func(os orderSettable) error {
			return os.UnsetOrder()
		})
		if err != nil {
			return nil, fmt.Errorf("failed to clear lexid for %s: %w", objectId, err)
		}
	}

	// Now assign new lexids in order
	previousLexId := ""
	for i, objectId := range order {
		var newLexId string
		err := cache.Do[orderSettable](o.objectGetter, objectId, func(os orderSettable) error {
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
