package order

import (
	"errors"
	"fmt"
	"sort"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/lexid"
	"github.com/samber/lo"

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

var lx = lexid.Must(lexid.CharsBase64, 4, 4000)

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

	newOrder, ops, err := o.reorder(objectIds, existing)
	if err != nil {
		return nil, fmt.Errorf("recalculate order: %w", err)
	}

	err = o.applyReorder(ops)
	if err != nil {
		return nil, fmt.Errorf("apply reorder: %w", err)
	}

	return newOrder, nil
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

type reorderOp struct {
	id         string
	newOrderId string
}

type idAndOrderId struct {
	id      string
	orderId string
}

func (o *orderSetter) reorder(objectIds []string, originalOrderIds map[string]string) ([]string, []reorderOp, error) {
	inputObjectIds := objectIds
	objectIdsSet := make(map[string]struct{})
	for _, id := range objectIds {
		objectIdsSet[id] = struct{}{}
	}

	originalIds := calculateOriginalIds(originalOrderIds)
	var remapIds bool
	for _, id := range originalIds {
		if _, ok := objectIdsSet[id]; !ok {
			remapIds = true
		}
	}
	if remapIds {
		objectIds = calculateFullList(objectIds, originalIds, originalOrderIds)
	}

	// TODO Remap ids using Diff

	nextExisting := o.precalcNext(originalOrderIds, objectIds)
	prev := ""
	out := map[string]string{}

	var ops []reorderOp
	var err error

	for i, id := range objectIds {
		curr := originalOrderIds[id]
		next := nextExisting[i]

		if curr != "" && curr > prev {
			// Current lexid is valid - keep it
			out[id] = curr
		} else if i == 0 {
			curr, err = o.getNewOrderId(id, "", next, true)
			if err != nil {
				curr = ""
			}
			ops = append(ops, reorderOp{id: id, newOrderId: curr})
		} else {
			// When inserting, check if next is valid relative to prev
			// If prev >= next, ignore next (treat as unbounded)
			if next != "" && prev >= next {
				next = ""
			}
			curr, err = o.getNewOrderId(id, prev, next, false)
			if err != nil {
				curr = ""
			}
			ops = append(ops, reorderOp{id: id, newOrderId: curr})
		}
		out[id] = curr
		prev = curr
	}

	outList := make([]string, len(inputObjectIds))
	for i := range inputObjectIds {
		outList[i] = out[inputObjectIds[i]]
	}
	return outList, ops, nil
}

func calculateOriginalIds(originalOrderIds map[string]string) []string {
	listWithOrder := make([]idAndOrderId, 0, len(originalOrderIds))
	for id, orderId := range originalOrderIds {
		listWithOrder = append(listWithOrder, idAndOrderId{id: id, orderId: orderId})
	}
	sort.Slice(listWithOrder, func(i, j int) bool {
		return listWithOrder[i].orderId < listWithOrder[j].orderId
	})
	return lo.Map(listWithOrder, func(it idAndOrderId, _ int) string {
		return it.id
	})
}

func calculateOriginalOrder(objectIds []string, originalOrderIds map[string]string) []string {
	listWithOrder := make([]idAndOrderId, 0, len(originalOrderIds))
	for _, id := range objectIds {
		listWithOrder = append(listWithOrder, idAndOrderId{id: id, orderId: originalOrderIds[id]})
	}
	sort.Slice(listWithOrder, func(i, j int) bool {
		return listWithOrder[i].orderId < listWithOrder[j].orderId
	})
	return lo.Map(listWithOrder, func(it idAndOrderId, _ int) string {
		return it.id
	})
}

func calculateFullList(objectIds []string, fullOriginalIds []string, originalOrderIds map[string]string) []string {
	originalIds := calculateOriginalOrder(objectIds, originalOrderIds)
	ops := slice.Diff(originalIds, objectIds, func(s string) string {
		return s
	}, func(s string, s2 string) bool {
		return s < s2
	})
	return slice.ApplyChanges(fullOriginalIds, ops, slice.StringIdentity)
}

func (o *orderSetter) applyReorder(ops []reorderOp) error {
	for _, op := range ops {
		err := cache.Do[order.OrderSettable](o.objectGetter, op.id, func(os order.OrderSettable) error {
			return os.SetOrder(op.newOrderId)
		})
		if err != nil {
			return fmt.Errorf("failed to set order for object %s: %w", op.id, err)
		}
	}
	return nil
}

// rebuildIfNeeded processes the order in a single pass, updating lexids as needed
func (o *orderSetter) rebuildIfNeeded(objectIds []string, existing map[string]string) ([]string, error) {
	nextExisting := o.precalcNext(existing, objectIds)
	prev := ""
	out := make([]string, len(objectIds))

	for i, id := range objectIds {
		curr := existing[id]
		next := nextExisting[i]

		if curr != "" && curr > prev {
			// Current lexid is valid - keep it
			out[i] = curr
		} else if i == 0 {
			curr = o.setRank(id, "", next, true)
			fmt.Println("set rank first", id, curr)
		} else {
			// When inserting, check if next is valid relative to prev
			// If prev >= next, ignore next (treat as unbounded)
			if next != "" && prev >= next {
				next = ""
			}
			curr = o.setRank(id, prev, next, false)
			fmt.Println("set rank between", id, prev, curr, next)
		}

		if curr == "" {
			fmt.Println("rebuild")
			// setRank failed → full rebuild
			return o.rebuildAllLexIds(objectIds)
		}
		out[i] = curr
		prev = curr
	}
	return out, nil
}

func (o *orderSetter) getNewOrderId(id string, before string, after string, isFirst bool) (string, error) {
	switch {
	case isFirst && before == "" && after == "":
		// First element with no constraints - add padding
		return o.setOrder(""), nil

	case before == "" && after == "":
		// Not first, but no constraints
		return o.setOrder(""), nil

	case before == "" && after != "":
		// Insert before the first existing element
		return o.setBetween("", after)

	case before != "" && after == "":
		// Insert after the last element
		return o.setOrder(before), nil

	default:
		// Insert between two elements
		return o.setBetween(before, after)
	}
}

func (o *orderSetter) setOrder(previousOrderId string) string {
	if previousOrderId == "" {
		// For the first element, use a lexid with huge padding
		return lx.Middle()
	} else {
		return lx.Next(previousOrderId)
	}
}

func (o *orderSetter) setBetween(left string, right string) (string, error) {
	if left == "" {
		// Insert before the first existing element
		return lx.Prev(right), nil
	} else {
		// Insert between two existing elements
		return lx.NextBefore(left, right)
	}
}

// setRank sets the lexid for a view, handling all positioning cases
func (o *orderSetter) setRank(objectId, before, after string, isFirst bool) string {
	var newOrderId string
	err := cache.Do[order.OrderSettable](o.objectGetter, objectId, func(os order.OrderSettable) error {
		var err error
		switch {
		case isFirst && before == "" && after == "":
			// First element with no constraints - add padding
			newOrderId, err = os.SetNextOrder("")

		case before == "" && after == "":
			// Not first, but no constraints
			newOrderId, err = os.SetNextOrder("")

		case before == "" && after != "":
			// Insert before the first existing element
			newOrderId, err = os.SetBetweenOrders("", after)

		case before != "" && after == "":
			// Insert after the last element
			newOrderId, err = os.SetNextOrder(before)

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
	next := ""
	for i := len(order) - 1; i >= 0; i-- {
		res[i] = next
		if lex := existing[order[i]]; lex != "" {
			next = lex
		}
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
				newLexId, err = os.SetNextOrder("")
			} else {
				// Subsequent elements
				newLexId, err = os.SetNextOrder(previousLexId)
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
