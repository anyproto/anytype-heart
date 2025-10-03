package subscription

import (
	"strings"

	"github.com/samber/lo"

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
		sorts:            sortsMap{},
		depOrderObjects:  map[string]map[string]struct{}{},
	}
}

type dependencyService struct {
	s *spaceSubscriptions

	isRelationObjMap map[domain.RelationKey]bool
	sorts            sortsMap                       // subId -> sortRelationKeys
	depOrderObjects  map[string]map[string]struct{} // objectId -> subIds
}

func (ds *dependencyService) makeSubscriptionByEntries(subId string, allEntries, activeEntries []*entry, keys, depKeys []domain.RelationKey, filterDepIds []string) *simpleSub {
	depSubKeys := ds.depSubKeys(subId, keys)
	depSub := ds.s.newSimpleSub(subId, depSubKeys, true)
	depSub.forceIds = filterDepIds
	parentSubId := strings.TrimSuffix(subId, "/dep")
	depIds := ds.depIdsByEntries(parentSubId, activeEntries, depKeys, depSub.forceIds)
	depEntries := ds.depEntriesByEntries(&opCtx{entries: allEntries}, depIds)
	depSub.init(depEntries)
	return depSub
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, subId string, depSub *simpleSub, entries []*entry, depKeys []domain.RelationKey) {
	depIds := ds.depIdsByEntries(subId, entries, depKeys, depSub.forceIds)
	if !depSub.isEqualIds(depIds) {
		depEntries := ds.depEntriesByEntries(ctx, depIds)
		depSub.refill(ctx, depEntries)
	}
	return
}

func (ds *dependencyService) depIdsByEntries(
	subId string, entries []*entry, depKeys []domain.RelationKey, forceIds []string,
) (depIds []string) {
	depIds = forceIds
	for _, e := range entries {
		for _, k := range depKeys {
			_, isSortKey := ds.sorts.getSortKey(subId, k)
			for _, depId := range e.data.WrapToStringList(k) {
				if depId != "" {
					if slice.FindPos(depIds, depId) == -1 && depId != e.id {
						depIds = append(depIds, depId)
					}
					if isSortKey {
						if ds.depOrderObjects[depId] == nil {
							ds.depOrderObjects[depId] = map[string]struct{}{}
						}
						ds.depOrderObjects[depId][subId] = struct{}{}
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

// reorderParentSubscription collects changed dep objects and triggers parent sub reorder
func (ds *dependencyService) reorderParentSubscription(depSubId string, ctx *opCtx) {
	parentSubId := strings.TrimSuffix(depSubId, "/dep")

	updatedDepObjects := make([]*domain.Details, 0)
	for _, e := range ctx.entries {
		if subIds, isDepOrderObject := ds.depOrderObjects[e.id]; isDepOrderObject {
			if _, found := subIds[parentSubId]; found {
				updatedDepObjects = append(updatedDepObjects, e.data)
			}
		}
	}

	if len(updatedDepObjects) == 0 {
		return
	}

	sub, ok := ds.s.getSortableSubscription(parentSubId)
	if !ok {
		log.Errorf("failed to get subscription %s to reorder objects", parentSubId)
		return
	}
	sub.reorder(ctx, updatedDepObjects)
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

// depKeys returns keys of relations with object/tag format that could handle ids of dependent objects
func (ds *dependencyService) depKeys(keys []domain.RelationKey) (depKeys []domain.RelationKey) {
	for _, key := range keys {
		if ds.isRelationObject(key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}

// depSubKeys returns keys that will be analyzed in objects filtered by dependent subscription
// TODO: maybe we need to exclude some keys from initial list (lastModifiedDate, syncDate)
func (ds *dependencyService) depSubKeys(subId string, keys []domain.RelationKey) []domain.RelationKey {
	sorts, found := ds.sorts[subId]
	if !found {
		return keys
	}
	for _, sort := range sorts {
		keys = append(keys, sort.orderKey())
	}
	return lo.Uniq(keys)
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

type sortsMap map[string][]sortKey // subId -> sortRelationKeys

func (m sortsMap) getSortKey(subId string, key domain.RelationKey) (sortKey, bool) {
	keys, ok := m[subId]
	if !ok {
		return sortKey{}, false
	}

	for _, k := range keys {
		if k.key == key {
			return k, true
		}
	}
	return sortKey{}, false
}
