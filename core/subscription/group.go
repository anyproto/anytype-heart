package subscription

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *spaceSubscriptions) newGroupSub(id string, relKey domain.RelationKey, f *database.Filters, groups []*model.BlockContentDataviewGroup) *groupSub {
	sub := &groupSub{
		id:     id,
		relKey: relKey,
		cache:  s.cache,
		set:    make(map[string]struct{}),
		filter: f,
		groups: groups,
	}
	return sub
}

type groupSub struct {
	id     string
	relKey domain.RelationKey

	cache *cache

	set map[string]struct{}

	filter *database.Filters

	groups []*model.BlockContentDataviewGroup
}

func (gs *groupSub) init(entries []*entry) (err error) {
	for _, e := range entries {
		e = gs.cache.GetOrSet(e)
		e.SetSub(gs.id, true, false)
		gs.set[e.id] = struct{}{}
	}
	return
}

func (gs *groupSub) counters() (prev, next int) {
	return 0, 0
}

func (gs *groupSub) onChange(ctx *opCtx) {
	checkGroups := false
	for _, ctxEntry := range ctx.entries {
		inFilter := gs.filter.FilterObj.FilterObject(ctxEntry.data)
		if _, inSet := gs.set[ctxEntry.id]; inSet {
			cacheEntry := gs.cache.Get(ctxEntry.id)
			if !checkGroups && cacheEntry != nil {
				oldList := cacheEntry.data.GetStringList(gs.relKey)
				newList := ctxEntry.data.GetStringList(gs.relKey)
				checkGroups = !slice.UnsortedEqual(oldList, newList)
			}
			if !inFilter {
				gs.cache.RemoveSubId(ctxEntry.id, gs.id)
				delete(gs.set, ctxEntry.id)
				checkGroups = true
			}
		} else if inFilter {
			// if not in cache but has been added new tags
			gs.cache.Set(ctxEntry)
			gs.set[ctxEntry.id] = struct{}{}
			checkGroups = true
		}
	}

	if checkGroups {
		var records []database.Record
		for id := range gs.set {
			if e := ctx.getEntry(id); e != nil {
				records = append(records, database.Record{Details: e.data})
			} else if rec := gs.cache.Get(id); rec != nil {
				records = append(records, database.Record{Details: rec.data})
			}
		}

		tag := kanban.GroupTag{Key: gs.relKey, Records: records}

		newGroups, err := tag.MakeDataViewGroups()
		if err != nil {
			log.Errorf("fail to make groups for kanban: %s", err)
		}

		oldIds := kanban.GroupsToStrSlice(gs.groups)
		newIds := kanban.GroupsToStrSlice(newGroups)

		removedIds, addedIds := slice.DifferenceRemovedAdded(oldIds, newIds)

		if len(removedIds) > 0 || len(addedIds) > 0 {
			for _, removedGroup := range removedIds {
				for _, g := range gs.groups {
					if removedGroup == g.Id {
						ctx.groups = append(ctx.groups, opGroup{subId: gs.id, group: g, remove: true})
					}
				}
			}

			for _, addGroupId := range addedIds {
				for _, g := range newGroups {
					if addGroupId == g.Id {
						ctx.groups = append(ctx.groups, opGroup{subId: gs.id, group: g})
					}
				}
			}
			gs.groups = newGroups
		}
	}
}

func (gs *groupSub) getActiveRecords() (res []*domain.Details) {
	return
}

func (gs *groupSub) getActiveEntries() (entries []*entry) {
	return
}

func (gs *groupSub) hasDep() bool {
	return false
}

func (gs *groupSub) getDep() subscription {
	return nil
}

func (gs *groupSub) close() {
	for id := range gs.set {
		gs.cache.RemoveSubId(id, gs.id)
	}
	return
}
