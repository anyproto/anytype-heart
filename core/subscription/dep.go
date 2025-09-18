package subscription

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func newDependencyService(s *spaceSubscriptions) *dependencyService {
	return &dependencyService{
		s:                s,
		isRelationObjMap: map[domain.RelationKey]bool{},
		orders:           database.OrderMap{},
		sorts:            map[string][]sortKey{},
		depOrderObjects:  map[string]map[string]struct{}{},
	}
}

type dependencyService struct {
	s *spaceSubscriptions

	isRelationObjMap map[domain.RelationKey]bool
	orders           database.OrderMap              // key -> objectId -> orderId
	sorts            map[string][]sortKey           // subId -> sortRelationKeys
	depOrderObjects  map[string]map[string]struct{} // objectId -> subIds
}

func (ds *dependencyService) makeSubscriptionByEntries(subId string, allEntries, activeEntries []*entry, keys, depKeys []domain.RelationKey, filterDepIds []string) *simpleSub {
	depSub := ds.s.newSimpleSub(subId, keys, true)
	depSub.forceIds = filterDepIds
	depIds, sortDepIds := ds.depIdsByEntries(activeEntries, depKeys, ds.sorts[subId], depSub.forceIds)
	depEntries := ds.depEntriesByEntries(&opCtx{entries: allEntries}, depIds)
	ds.updateOrders(subId, depEntries, sortDepIds)
	depSub.init(depEntries)
	return depSub
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, subId string, depSub *simpleSub, entries []*entry, depKeys []domain.RelationKey) {
	depIds, sortDepIds := ds.depIdsByEntries(entries, depKeys, ds.sorts[subId], depSub.forceIds)
	if !depSub.isEqualIds(depIds) {
		depEntries := ds.depEntriesByEntries(ctx, depIds)
		ds.updateOrders(subId, depEntries, sortDepIds)
		depSub.refill(ctx, depEntries)
	}
	return
}

func (ds *dependencyService) depIdsByEntries(
	entries []*entry, depKeys []domain.RelationKey, sortKeys []sortKey, forceIds []string,
) (depIds []string, sortDepIds map[sortKey][]string) {
	depIds = forceIds
	sortDepIds = make(map[sortKey][]string, len(sortKeys))
	for _, e := range entries {
		for _, k := range depKeys {
			var currentSortKey *sortKey = nil
			for _, sk := range sortKeys {
				if sk.key == k {
					currentSortKey = &sk
					sortDepIds[sk] = make([]string, 0)
					break
				}
			}
			for _, depId := range e.data.WrapToStringList(k) {
				if depId != "" && slice.FindPos(depIds, depId) == -1 && depId != e.id {
					depIds = append(depIds, depId)
					if currentSortKey != nil {
						sortDepIds[*currentSortKey] = append(sortDepIds[*currentSortKey], depId)
					}
				}
			}
		}
	}
	return
}

func (ds *dependencyService) depEntriesByEntries(ctx *opCtx, depIds []string) (depEntries []*entry) {
	if len(depIds) == 0 {
		return
	}
	var missIds []string
	for _, id := range depIds {
		var e *entry

		// priority: ctx.entries, cache, objectStore
		if e = ctx.getEntry(id); e == nil {
			if e = ds.s.cache.Get(id); e != nil {
				e = e.Copy()
			} else {
				missIds = append(missIds, id)
			}
			if e != nil {
				ctx.entries = append(ctx.entries, e)
			}
		}
		if e != nil {
			depEntries = append(depEntries, e)
		}
	}
	if len(missIds) > 0 {
		records, err := ds.s.objectStore.QueryByIds(missIds)
		if err != nil {
			log.Errorf("can't query by id: %v", err)
		}
		for _, r := range records {
			e := newEntry(r.Details.GetString(bundle.RelationKeyId), r.Details)
			ctx.entries = append(ctx.entries, e)
			depEntries = append(depEntries, e)
		}
	}
	return
}

func (ds *dependencyService) enregisterObjectSorts(subId string, sorts []database.SortRequest) {
	sortRelations := make([]sortKey, 0, len(sorts))

	for _, sort := range sorts {
		if !ds.isRelationObject(sort.RelationKey) {
			continue
		}
		sortRelations = append(sortRelations, sortKey{
			key:   sort.RelationKey,
			isTag: sort.Format == model.RelationFormat_tag || sort.Format == model.RelationFormat_status,
		})
	}

	if len(sortRelations) != 0 {
		ds.sorts[subId] = sortRelations
	}
}

// updateOrders updates orderMap for sorting keys of subscription subId that have object format
func (ds *dependencyService) updateOrders(subId string, entries []*entry, sortDepIds map[sortKey][]string) {
	ctx := opCtx{entries: entries}
	for sort, depIds := range sortDepIds {
		for _, depId := range depIds {
			orderId := ""
			if sortEntry := ctx.getEntry(depId); sortEntry != nil {
				orderId = sortEntry.data.GetString(sort.orderKey())
			}
			ds.orders[sort.key][depId] = orderId
			if ds.depOrderObjects[depId] == nil {
				ds.depOrderObjects[depId] = map[string]struct{}{}
			}
			ds.depOrderObjects[depId][subId] = struct{}{}
		}
	}
}

// reorderParentSubscription checks if orderId has changed
func (ds *dependencyService) reorderParentSubscription(depSubId string, ctx *opCtx) {
	parentSubId := strings.TrimSuffix(depSubId, "/dep")

	sortKeys := ds.sorts[parentSubId]
	if len(sortKeys) == 0 {
		return
	}

	updatedDepObjects := make([]string, 0)
	for _, e := range ctx.entries {
		for _, key := range sortKeys {
			currentOrderId, found := ds.orders[key.key][e.id]
			if !found {
				continue
			}
			if currentOrderId != e.data.GetString(key.orderKey()) {
				updatedDepObjects = append(updatedDepObjects, e.id)
				ds.orders[key.key][e.id] = e.data.GetString(key.orderKey())
			}
		}
	}

	subsToUpdate := make(map[string]struct{})
	for _, objectId := range updatedDepObjects {
		for subId := range ds.depOrderObjects[objectId] {
			subsToUpdate[subId] = struct{}{}
		}
	}

	if len(subsToUpdate) == 0 {
		return
	}

	for subId := range subsToUpdate {
		sub, ok := ds.s.getSubscription(subId)
		if !ok {
			log.Errorf("failed to get subscription %s to reorder objects", subId)
			continue
		}
		activeEntries := sub.getActiveEntries()
		ctx.entries = append(ctx.entries, activeEntries...)
		sub.onChange(ctx)
	}
}

var ignoredKeys = map[domain.RelationKey]struct{}{
	bundle.RelationKeyId:                {},
	bundle.RelationKeySpaceId:           {}, // relation format for spaceId has mistakenly set to Object instead of shorttext
	bundle.RelationKeyFeaturedRelations: {}, // relation format for featuredRelations has mistakenly set to Object instead of shorttext
}

func (ds *dependencyService) isRelationObject(key domain.RelationKey) bool {
	if key == "" {
		return false
	}
	if _, ok := ignoredKeys[key]; ok {
		return false
	}
	if strings.ContainsRune(string(key), '.') {
		// skip nested keys like "assignee.type"
		return false
	}
	if isObj, ok := ds.isRelationObjMap[key]; ok {
		return isObj
	}
	relFormat, err := ds.s.objectStore.GetRelationFormatByKey(key)
	if err != nil && key != "pageCover" {
		log.Errorf("can't get relation %s: %v", key, err)
		return false
	}
	isObj := relFormat == model.RelationFormat_object || relFormat == model.RelationFormat_file || relFormat == model.RelationFormat_tag || relFormat == model.RelationFormat_status
	ds.isRelationObjMap[key] = isObj
	return isObj
}

func (ds *dependencyService) depKeys(keys []domain.RelationKey) (depKeys []domain.RelationKey) {
	for _, key := range keys {
		if ds.isRelationObject(key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}

type sortKey struct {
	key   domain.RelationKey
	isTag bool
}

func (k sortKey) orderKey() domain.RelationKey {
	if k.isTag {
		return bundle.RelationKeyOrderId
	}
	return bundle.RelationKeyName
}
