package order

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/anyproto/any-sync/app"
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

	return o.rebuildIfNeeded(objectIds, existing, false)
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

	return o.rebuildIfNeeded(objectIds, existing, false)
}

func (o *orderSetter) SetObjectTypesOrder(spaceId string, objectIds []string) ([]string, error) {
	if len(objectIds) == 0 {
		return nil, errors.New("empty objectIds")
	}

	existing, err := o.getCurrentTypesOrder(spaceId)
	if err != nil {
		return nil, err
	}

	return o.rebuildIfNeeded(objectIds, existing, true)
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

func (o *orderSetter) reorder(objectIds []string, originalOrderIds map[string]string, needFullList bool) ([]string, []reorderOp, error) {
	// Save the original list
	inputObjectIds := objectIds

	originalIds := getAllOriginalIds(originalOrderIds)
	if needFullList {
		objectIds = calculateFullList(objectIds, originalIds, originalOrderIds)
	}

	nextExisting := o.precalcNext(originalOrderIds, objectIds)
	out := map[string]string{}

	var prev string
	var ops []reorderOp
	var err error

	for i, id := range objectIds {
		curr := originalOrderIds[id]
		next := nextExisting[i]

		if curr == "" || prev >= curr || (next != "" && curr >= next && prev < next) {
			if prev >= next {
				next = ""
			}
			curr, err = o.getNewOrderId(prev, next, i == 0)
			if err != nil {
				return o.rebuildAllLexIds(objectIds, inputObjectIds)
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

func getAllOriginalIds(originalOrderIds map[string]string) []string {
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

func getIdsInOriginalOrder(objectIdsSet map[string]struct{}, fullOriginalIds []string) []string {
	originalIds := make([]string, 0, len(objectIdsSet))
	for _, id := range fullOriginalIds {
		if _, ok := objectIdsSet[id]; ok {
			originalIds = append(originalIds, id)
		}
	}
	return originalIds
}

func hasItemNotInSet[T comparable](items []T, set map[T]struct{}) bool {
	for _, id := range items {
		if _, ok := set[id]; !ok {
			return true
		}
	}

	return false
}

// calculateFullList return the full list of ids, that a client is expected. To do it, we
// compare a list of ids provided by the client with the corresponding part of the full list of ids.
// Then we apply changes to the original full list.
// For example,
// - The full list is [1 2 3 4 5]
// - Client sends us [3 1 5]
// - We compare [1 3 5] with [3 1 5] and get a change "move 1 after 3"
// - Apply this change and get the list: [2 3 1 4 5]
func calculateFullList(objectIds []string, fullOriginalIds []string, originalOrderIds map[string]string) []string {
	objectIdsSet := make(map[string]struct{})
	for _, id := range objectIds {
		objectIdsSet[id] = struct{}{}
	}

	if !hasItemNotInSet(fullOriginalIds, objectIdsSet) {
		return objectIds
	}

	originalIds := getIdsInOriginalOrder(objectIdsSet, fullOriginalIds)
	ops := slice.Diff(originalIds, objectIds, func(s string) string {
		return s
	}, func(s string, s2 string) bool {
		return s < s2
	})

	for _, ch := range ops {
		if mv := ch.Move(); mv != nil {
			// Substitute an empty AfterId with the previous element in the original list, if any
			if mv.AfterID == "" {
				origIdx := slices.Index(fullOriginalIds, originalIds[0])
				if origIdx > 0 {
					mv.AfterID = fullOriginalIds[origIdx-1]
				}
			}
		}
	}

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
func (o *orderSetter) rebuildIfNeeded(objectIds []string, existing map[string]string, needFullList bool) ([]string, error) {
	newOrder, ops, err := o.reorder(objectIds, existing, needFullList)
	if err != nil {
		return nil, fmt.Errorf("recalculate order: %w", err)
	}

	err = o.applyReorder(ops)
	if err != nil {
		return nil, fmt.Errorf("apply reorder: %w", err)
	}
	return newOrder, nil
}

func (o *orderSetter) getNewOrderId(before string, after string, isFirst bool) (string, error) {
	switch {
	case isFirst && before == "" && after == "":
		// First element with no constraints - add padding
		return o.getNextOrderId(""), nil

	case before == "" && after == "":
		// Not first, but no constraints
		return o.getNextOrderId(""), nil

	case before == "" && after != "":
		// Insert before the first existing element
		return o.getInBetweenOrderId("", after)

	case before != "" && after == "":
		// Insert after the last element
		return o.getNextOrderId(before), nil

	default:
		// Insert between two elements
		return o.getInBetweenOrderId(before, after)
	}
}

func (o *orderSetter) getNextOrderId(previousOrderId string) string {
	if previousOrderId == "" {
		// For the first element, use a lexid with huge padding
		return order.LexId.Middle()
	} else {
		return order.LexId.Next(previousOrderId)
	}
}

func (o *orderSetter) getInBetweenOrderId(left string, right string) (string, error) {
	if left == "" {
		// Insert before the first existing element
		return order.LexId.Prev(right), nil
	} else {
		// Insert between two existing elements
		return order.LexId.NextBefore(left, right)
	}
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
func (o *orderSetter) rebuildAllLexIds(objectIds []string, inputObjectIds []string) ([]string, []reorderOp, error) {
	ops := make([]reorderOp, len(objectIds))
	opsSet := map[string]string{}
	previousLexId := ""
	for i, objectId := range objectIds {
		newLexId := o.getNextOrderId(previousLexId)
		ops[i] = reorderOp{id: objectId, newOrderId: newLexId}
		opsSet[objectId] = newLexId
		previousLexId = newLexId
	}
	finalOrder := make([]string, len(inputObjectIds))
	for i, id := range inputObjectIds {
		finalOrder[i] = opsSet[id]
	}
	return finalOrder, ops, nil
}
