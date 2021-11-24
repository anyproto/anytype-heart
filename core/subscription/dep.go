package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

func (ds *dependencyService) makeSubscriptionByEntries(subId string, entries []*entry, keys, depKeys []string) *simpleSub {
	depSub := ds.s.newSimpleSub(subId, keys, true)
	depEntries := ds.depEntriesByEntries(entries, depKeys)
	depSub.init(depEntries)
	return nil
}

func (ds *dependencyService) refillSubscription(ctx *opCtx, sub *simpleSub, entries []*entry, depKeys []string) {
	depEntries := ds.depEntriesByEntries(entries, depKeys)
	sub.refill(ctx, depEntries)
	ctx.depEntries = append(ctx.depEntries, depEntries...)
	return
}

func (ds *dependencyService) depEntriesByEntries(entries []*entry, depKeys []string) (depEntries []*entry) {
	var depIds []string
	for _, e := range entries {
		for _, k := range depKeys {
			for _, depId := range pbtypes.GetStringList(e.data, k) {
				depIds = append(depIds, depId)
			}
		}
	}
	depRecords, err := ds.s.objectStore.QueryById(depIds)
	if err != nil {
		log.Errorf("can't query by id: %v", err)
	}
	depEntries = make([]*entry, 0, len(depRecords))
	for _, r := range depRecords {
		depEntries = append(depEntries, &entry{
			id:   pbtypes.GetString(r.Details, "id"),
			data: r.Details,
		})
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
	}
	isObj := rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file
	ds.isRelationObjMap[key] = isObj
	return isObj
}

func (ds *dependencyService) depKeys(keys []string) (depKeys []string) {
	for _, key := range keys {
		if ds.isRelationObject(key) {
			depKeys = append(depKeys, key)
		}
	}
	return
}
