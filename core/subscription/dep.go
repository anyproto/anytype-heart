package subscription

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func newDependencyService(s *service) *dependencyService {
	return &dependencyService{
		s:                s,
		isRelationObjMap: map[string]bool{},
	}
}

type dependencyService struct {
	s *service

	isRelationObjMap map[string]bool
}

func (ds *dependencyService) makeSubscriptionByEntries(subId string, allEntries, activeEntries []*entry, keys, depKeys, filterDepIds []string) *simpleSub {
	depSub := ds.s.newSimpleSub(subId, keys, true)
	depSub.forceIds = filterDepIds
	depEntries := ds.depEntriesByEntries(&opCtx{entries: allEntries}, ds.depIdsByEntries(activeEntries, depKeys, depSub.forceIds))
	depSub.init(depEntries)
	return depSub
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, sub *simpleSub, entries []*entry, depKeys []string) {
	depIds := ds.depIdsByEntries(entries, depKeys, sub.forceIds)
	if !sub.isEqualIds(depIds) {
		depEntries := ds.depEntriesByEntries(ctx, depIds)
		sub.refill(ctx, depEntries)
	}
	return
}

func (ds *dependencyService) depIdsByEntries(entries []*entry, depKeys, forceIds []string) (depIds []string) {
	depIds = forceIds
	for _, e := range entries {
		for _, k := range depKeys {
			for _, depId := range pbtypes.GetStringList(e.data, k) {
				if depId != "" && slice.FindPos(depIds, depId) == -1 {
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
				newSubIds := make([]string, len(e.subIds))
				newSubIsActive := make([]bool, len(e.subIsActive))
				newSubFullDetailsSent := make([]bool, len(e.subFullDetailsSent))
				copy(newSubIds, e.subIds)
				copy(newSubIsActive, e.subIsActive)
				copy(newSubFullDetailsSent, e.subFullDetailsSent)
				e = &entry{
					id:                 id,
					data:               e.data,
					subIds:             newSubIds,
					subIsActive:        newSubIsActive,
					subFullDetailsSent: newSubFullDetailsSent,
				}
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
		records, err := ds.s.objectStore.QueryByID(missIds)
		if err != nil {
			log.Errorf("can't query by id: %v", err)
		}
		for _, r := range records {
			e := &entry{
				id:   pbtypes.GetString(r.Details, "id"),
				data: r.Details,
			}
			ctx.entries = append(ctx.entries, e)
			depEntries = append(depEntries, e)
		}
	}
	return
}

func (ds *dependencyService) isRelationObject(key string) bool {
	if isObj, ok := ds.isRelationObjMap[key]; ok {
		return isObj
	}
	rel, err := ds.s.relationService.GetRelationByKey(key)
	if err != nil {
		log.Errorf("can't get relation %s: %v", key, err)
		return false
	}
	isObj := rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file || rel.Format == model.RelationFormat_tag || rel.Format == model.RelationFormat_status
	ds.isRelationObjMap[key] = isObj
	return isObj
}

func (ds *dependencyService) depKeys(keys []string) (depKeys []string) {
	for _, key := range keys {
		if key == bundle.RelationKeyId.String() {
			continue
		}
		if key == bundle.RelationKeyLinks.String() {
			// skip links because it's aggregated from other relations and blocks
			continue
		}
		if ds.isRelationObject(key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}
