package subscription

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func newDependencyService(s *spaceSubscriptions) *dependencyService {
	return &dependencyService{
		s:                s,
		isRelationObjMap: map[domain.RelationKey]bool{},
	}
}

type dependencyService struct {
	s *spaceSubscriptions

	isRelationObjMap map[domain.RelationKey]bool
}

func (ds *dependencyService) makeSubscriptionByEntries(subId string, spaceId string, allEntries, activeEntries []*entry, keys, depKeys []domain.RelationKey, filterDepIds []string) *simpleSub {
	depSub := ds.s.newSimpleSub(subId, spaceId, keys, true)
	depSub.forceIds = filterDepIds
	depEntries := ds.depEntriesByEntries(&opCtx{entries: allEntries, spaceId: spaceId}, ds.depIdsByEntries(activeEntries, depKeys, depSub.forceIds))
	depSub.init(depEntries)
	return depSub
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, sub *simpleSub, entries []*entry, depKeys []domain.RelationKey) {
	depIds := ds.depIdsByEntries(entries, depKeys, sub.forceIds)
	if !sub.isEqualIds(depIds) {
		depEntries := ds.depEntriesByEntries(ctx, depIds)
		sub.refill(ctx, depEntries)
	}
	return
}

func (ds *dependencyService) depIdsByEntries(entries []*entry, depKeys []domain.RelationKey, forceIds []string) (depIds []string) {
	depIds = forceIds
	for _, e := range entries {
		for _, k := range depKeys {
			for _, depId := range e.data.GetStringList(k) {
				if depId != "" && slice.FindPos(depIds, depId) == -1 && depId != e.id {
					depIds = append(depIds, depId)
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

var ignoredKeys = map[domain.RelationKey]struct{}{
	bundle.RelationKeyId:                {},
	bundle.RelationKeySpaceId:           {}, // relation format for spaceId has mistakenly set to Object instead of shorttext
	bundle.RelationKeyFeaturedRelations: {}, // relation format for featuredRelations has mistakenly set to Object instead of shorttext
}

func (ds *dependencyService) isRelationObject(spaceId string, key domain.RelationKey) bool {
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
	if err != nil {
		log.Errorf("can't get relation %s: %v", key, err)
		return false
	}
	isObj := relFormat == model.RelationFormat_object || relFormat == model.RelationFormat_file || relFormat == model.RelationFormat_tag || relFormat == model.RelationFormat_status
	ds.isRelationObjMap[key] = isObj
	return isObj
}

func (ds *dependencyService) depKeys(spaceId string, keys []domain.RelationKey) (depKeys []domain.RelationKey) {
	for _, key := range keys {
		if ds.isRelationObject(spaceId, key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}
