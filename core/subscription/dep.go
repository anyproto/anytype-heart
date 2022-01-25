package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

func (ds *dependencyService) makeSubscriptionByEntries(subId string, allEntries, activeEntries []*entry, keys, depKeys []string) *simpleSub {
	depSub := ds.s.newSimpleSub(subId, keys, true)
	depEntries := ds.depEntriesByEntries(&opCtx{entries: allEntries}, activeEntries, depKeys)
	depSub.init(depEntries)
	return depSub
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, sub *simpleSub, entries []*entry, depKeys []string) {
	depEntries := ds.depEntriesByEntries(ctx, entries, depKeys)
	sub.refill(ctx, depEntries)
	return
}

func (ds *dependencyService) depEntriesByEntries(ctx *opCtx, entries []*entry, depKeys []string) (depEntries []*entry) {
	var depIds []string
	for _, e := range entries {
		for _, k := range depKeys {
			for _, depId := range pbtypes.GetStringList(e.data, k) {
				if depId != "" && slice.FindPos(depIds, depId) == -1 {
					depIds = append(depIds, depId)
				}
			}
		}
	}
	if len(depIds) == 0 {
		return
	}
	depRecords, err := ds.s.objectStore.QueryById(depIds)
	if err != nil {
		log.Errorf("can't query by id: %v", err)
	}
	depEntries = make([]*entry, 0, len(depRecords))
	for _, r := range depRecords {
		var e *entry
		id := pbtypes.GetString(r.Details, "id")

		// priority: ctx.entries, cache, objectStore
		if e = ctx.getEntry(id); e == nil {
			if e = ds.s.cache.Get(id); e == nil {
				e = &entry{
					id:   id,
					data: r.Details,
				}
			} else {
				e = &entry{
					id:          id,
					data:        e.data,
					subIds:      e.subIds,
					subIsActive: e.subIsActive,
				}
			}
			ctx.entries = append(ctx.entries, e)
		}
		depEntries = append(depEntries, e)
	}
	return
}

func (ds *dependencyService) isRelationObject(key string) bool {
	if isObj, ok := ds.isRelationObjMap[key]; ok {
		return isObj
	}
	rel, err := ds.s.objectStore.GetRelation(key)
	if err != nil {
		log.Errorf("can't get relation: %v", err)
		return false
	}
	isObj := rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file
	ds.isRelationObjMap[key] = isObj
	return isObj
}

func (ds *dependencyService) depKeys(keys []string) (depKeys []string) {
	for _, key := range keys {
		if key == bundle.RelationKeyId.String() {
			continue
		}
		if ds.isRelationObject(key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}
