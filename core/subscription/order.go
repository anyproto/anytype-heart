package subscription

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type orderManager struct {
	s *spaceSubscriptions

	subs    map[domain.RelationKey]subscription
	orders  database.OrderMap                            // key -> objectId -> orderId
	parents map[domain.RelationKey]map[string]*sortedSub // key -> parentSubId -> sub
}

func newOrderManager(s *spaceSubscriptions) *orderManager {
	return &orderManager{
		s:       s,
		subs:    make(map[domain.RelationKey]subscription),
		orders:  database.OrderMap{},
		parents: make(map[domain.RelationKey]map[string]*sortedSub),
	}
}

func (om *orderManager) initOrderSubscription(relationKey domain.RelationKey, parent *sortedSub) error {
	if relationKey == "" {
		return nil
	}

	_, found := om.subs[relationKey]
	if found {
		om.parents[relationKey][parent.id] = parent
		return nil
	}

	rel, err := om.s.objectStore.FetchRelationByKey(relationKey.String())
	if err != nil {
		return fmt.Errorf("failed to fetch relation from store")
	}

	var sortSub subscription
	switch rel.Format {
	case model.RelationFormat_tag, model.RelationFormat_status:
		sortSub, err = om.buildTagOrderSub(relationKey, parent)
		if err != nil {
			return err
		}
	case model.RelationFormat_object, model.RelationFormat_file:
		sortSub, err = om.buildObjectOrderSub(relationKey, parent)
		if err != nil {
			return err
		}
	}
	if sortSub == nil {
		return nil
	}
	parent.orderRelations = append(parent.orderRelations, model.RelationLink{Key: rel.Key, Format: rel.Format})
	om.s.setSubscription(orderSubId(relationKey), sortSub)
	om.parents[relationKey] = map[string]*sortedSub{parent.id: parent}
	om.subs[relationKey] = sortSub
	return nil
}

func (om *orderManager) buildTagOrderSub(relationKey domain.RelationKey, parent *sortedSub) (subscription, error) {
	f, err := database.MakeFilters([]database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyRelationKey,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(relationKey),
		},
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_relationOption),
		},
	}, om.s.objectStore)
	if err != nil {
		return nil, fmt.Errorf("failed to make filter for sort subscription: %w", err)
	}
	sub := om.s.newSortedSub(orderSubId(relationKey), parent.spaceId, []domain.RelationKey{bundle.RelationKeyId, bundle.RelationKeyOrderId}, f, nil, 0, 0)
	if err = initSubEntries(om.s.objectStore, &database.Filters{FilterObj: f}, sub); err != nil {
		return nil, fmt.Errorf("failed to init sort subscription: %w", err)
	}
	orderIdMap := make(map[string]string, len(sub.entriesBeforeStarted))

	sub.iterateActive(func(e *entry) {
		orderId := e.data.GetString(bundle.RelationKeyOrderId)
		orderIdMap[e.id] = orderId
	})
	om.orders[relationKey] = orderIdMap
	return sub, nil
}

func (om *orderManager) buildObjectOrderSub(relationKey domain.RelationKey, parent *sortedSub) (subscription, error) {
	var (
		targetIds []string
		entries   []*entry
	)
	idsToNames := make(map[string]string)
	if err := om.s.objectStore.QueryIterate(database.Query{}, func(details *domain.Details) {
		targetIds = append(targetIds, details.GetStringList(relationKey)...)
		entryId := details.GetString(bundle.RelationKeyId)
		entries = append(entries, newEntry(entryId, details))
		idsToNames[entryId] = details.GetString(bundle.RelationKeyName)
	}); err != nil {
		return nil, fmt.Errorf("failed to query objects for : %w", err)
	}
	targetIds = lo.Uniq(targetIds)
	entries = slices.DeleteFunc(entries, func(e *entry) bool {
		return !slices.Contains(targetIds, e.id)
	})
	maps.DeleteFunc(idsToNames, func(id, _ string) bool {
		return !slices.Contains(targetIds, id)
	})

	sub := om.s.newIdsSub(orderSubId(relationKey), parent.spaceId, []domain.RelationKey{bundle.RelationKeyId, bundle.RelationKeyName}, false)
	if err := sub.init(entries); err != nil {
		return nil, fmt.Errorf("failed to init sort subscription: %w", err)
	}
	om.orders[relationKey] = idsToNames
	return sub, nil
}

func (om *orderManager) updateOrders(ctx *opCtx, orderSubId string) {
	key, err := parseOrderSubId(orderSubId)
	if err != nil {
		return
	}
	orders := om.orders[key]
	var orderUpdated bool
	for _, e := range ctx.entries {
		if relKey := e.data.GetString(bundle.RelationKeyRelationKey); relKey == key.String() {
			om.orders[key][e.id] = e.data.GetString(bundle.RelationKeyOrderId)
			orderUpdated = true
			continue
		}
		if _, found := orders[e.id]; found {
			// we should update names of objects that already present in OrderMap
			om.orders[key][e.id] = e.data.GetString(bundle.RelationKeyName)
			orderUpdated = true
		}
	}
	if !orderUpdated {
		return
	}
	for _, parent := range om.parents[key] {
		// call onChange to reorder objects in parent subscriptions
		parentEntries, err := queryEntries(om.s.objectStore, &database.Filters{FilterObj: parent.filter})
		if err != nil {
			panic(err)
		}
		ctx.entries = append(ctx.entries, parentEntries...)
		parent.onChange(ctx)
	}
}

// TODO: optimize algo
func (om *orderManager) addObjectOrderIds(ctx *opCtx, relations ...model.RelationLink) {
	var objectRelationKeys []domain.RelationKey
	for _, relation := range relations {
		// we should analyze only object relations, as idsSub does not have any filter
		if relation.Format == model.RelationFormat_object || relation.Format == model.RelationFormat_file {
			objectRelationKeys = append(objectRelationKeys, domain.RelationKey(relation.Key))
		}
	}

	if len(objectRelationKeys) == 0 {
		return
	}

	newIds := make(map[domain.RelationKey]map[string]string, len(objectRelationKeys))
	for _, e := range ctx.entries {
		for _, key := range objectRelationKeys {
			ids := e.data.GetStringList(key)
			for _, id := range ids {
				if _, found := om.orders[key][id]; !found {
					name := ""
					if newOrderEntry := ctx.c.Get(id); newOrderEntry != nil {
						name = newOrderEntry.data.GetString(bundle.RelationKeyName)
					}
					newIds[key][id] = name
				}
			}
		}
	}

	for key, ids := range newIds {
		idsList := make([]string, 0, len(ids))
		for id, name := range ids {
			om.orders[key][id] = name
			idsList = append(idsList, id)
		}
		om.subs[key].(*idsSub).addIds(idsList)
		for _, parent := range om.parents[key] {
			parentEntries, err := queryEntries(om.s.objectStore, &database.Filters{FilterObj: parent.filter})
			if err != nil {
				panic(err)
			}
			ctx.entries = append(ctx.entries, parentEntries...)
			parent.onChange(ctx)
		}
	}
}

func (om *orderManager) closeSubs(parentSubId string, relations ...model.RelationLink) {
	for _, relation := range relations {
		key := domain.RelationKey(relation.GetKey())
		parents := om.parents[key]
		switch len(parents) {
		case 0:
			panic("check algo")
		case 1:
			sub, ok := om.subs[key]
			if !ok {
				panic("check algo")
			}
			sub.close()
			delete(om.subs, key)
			delete(om.parents, key)
			delete(om.orders, key)
		default:
			delete(parents, parentSubId)
			om.parents[key] = parents
		}
	}
}

func orderSubId(key domain.RelationKey) string {
	return fmt.Sprintf("%s-order-sub", key)
}

func parseOrderSubId(id string) (domain.RelationKey, error) {
	if !strings.HasSuffix(id, "-order-sub") {
		return "", fmt.Errorf("invalid order sub id: %s", id)
	}
	return domain.RelationKey(strings.TrimSuffix(id, "-order-sub")), nil
}
