package subscription

import (
	"sort"
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// groupsSub serves SubscribeGroups (kanban columns): the distinct groups of
// a query, kept current by re-running the kanban grouper and diffing group
// ids whenever something relevant changes — an object entering/leaving the
// match set, a member's grouped value changing, or a relationOption of the
// grouped relation changing (tag/status groups are derived from option
// objects). The core engine knows nothing about groups: this adapter watches
// the feed through spaceState.groupsSubs and recomputes off the space mutex
// (the grouper queries the store).
type groupsSub struct {
	subId       string
	spaceId     string
	relationKey domain.RelationKey
	grouper     kanban.Grouper
	svc         *service
	space       *spaceState

	// compile produces a fresh *database.Filters per recomputation — the
	// kanban groupers mutate the filters they are given
	compile func() (*database.Filters, error)
	// match is a privately compiled filter for relevance checks only
	match database.Filter

	// members maps matching object id → its grouped-key value; options
	// tracks known relationOption ids of the grouped relation — a hard
	// delete tombstones an option down to {id, isDeleted}, dropping the
	// layout and relationKey that would otherwise identify it. Both are
	// maintained under the space mutex by checkItem, so only changes that
	// can affect groups mark the sub dirty.
	members map[string]domain.Value
	options map[string]struct{}
	dirty   bool // guarded by the space mutex

	// recomputeMu serializes recomputations (worker vs initial subscribe);
	// dead stops a recompute that lost a race against teardown from
	// broadcasting groups for a subscription that no longer exists
	recomputeMu sync.Mutex
	dead        bool
	current     map[string]*model.BlockContentDataviewGroup
}

// markDead prevents any further group broadcasts; called by teardown
func (g *groupsSub) markDead() {
	g.recomputeMu.Lock()
	g.dead = true
	g.recomputeMu.Unlock()
}

// checkItem flags relevance of one feed item. Runs under the space mutex.
func (g *groupsSub) checkItem(id string, details *domain.Details) {
	if details == nil {
		if _, ok := g.members[id]; ok {
			delete(g.members, id)
			g.dirty = true
		}
		if _, ok := g.options[id]; ok {
			delete(g.options, id)
			g.dirty = true
		}
		return
	}
	if details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_relationOption) &&
		details.GetString(bundle.RelationKeyRelationKey) == string(g.relationKey) {
		if details.GetBool(bundle.RelationKeyIsDeleted) {
			delete(g.options, id)
		} else {
			g.options[id] = struct{}{}
		}
		g.dirty = true
		return
	}
	if details.GetBool(bundle.RelationKeyIsDeleted) {
		// a hard delete tombstones the object down to {id, isDeleted}: only
		// the tracked option set can still identify a deleted option
		if _, ok := g.options[id]; ok {
			delete(g.options, id)
			g.dirty = true
			return
		}
	}
	matched := g.match != nil && g.match.FilterObject(details)
	oldVal, wasMember := g.members[id]
	switch {
	case matched && !wasMember:
		g.members[id] = details.Get(g.relationKey)
		g.dirty = true
	case !matched && wasMember:
		delete(g.members, id)
		g.dirty = true
	case matched && wasMember:
		newVal := details.Get(g.relationKey)
		if !newVal.Equal(oldVal) {
			g.members[id] = newVal
			g.dirty = true
		}
	}
}

// computeGroups runs the kanban grouper against a fresh filter compilation
func (g *groupsSub) computeGroups() ([]*model.BlockContentDataviewGroup, error) {
	filters, err := g.compile()
	if err != nil {
		return nil, err
	}
	if err = g.grouper.InitGroups(g.spaceId, filters); err != nil {
		return nil, err
	}
	return g.grouper.MakeDataViewGroups()
}

// init computes the initial group set without emitting events
func (g *groupsSub) init() ([]*model.BlockContentDataviewGroup, error) {
	g.recomputeMu.Lock()
	defer g.recomputeMu.Unlock()
	groups, err := g.computeGroups()
	if err != nil {
		return nil, err
	}
	g.current = make(map[string]*model.BlockContentDataviewGroup, len(groups))
	for _, grp := range groups {
		g.current[grp.Id] = grp
	}
	return groups, nil
}

// recompute re-derives the group set and broadcasts the delta as
// SubscriptionGroups events. Runs without engine locks.
func (g *groupsSub) recompute() {
	g.recomputeMu.Lock()
	defer g.recomputeMu.Unlock()
	if g.dead {
		return
	}
	groups, err := g.computeGroups()
	if err != nil {
		log.Errorf("groups subscription %s: recompute: %v", g.subId, err)
		return
	}
	newSet := make(map[string]*model.BlockContentDataviewGroup, len(groups))
	for _, grp := range groups {
		newSet[grp.Id] = grp
	}

	var removedIds, addedIds []string
	for id := range g.current {
		if _, ok := newSet[id]; !ok {
			removedIds = append(removedIds, id)
		}
	}
	for id := range newSet {
		if _, ok := g.current[id]; !ok {
			addedIds = append(addedIds, id)
		}
	}
	sort.Strings(removedIds)
	sort.Strings(addedIds)

	msgs := make([]*pb.EventMessage, 0, len(removedIds)+len(addedIds))
	for _, id := range removedIds {
		msgs = append(msgs, g.groupEvent(g.current[id], true))
	}
	for _, id := range addedIds {
		msgs = append(msgs, g.groupEvent(newSet[id], false))
	}
	g.current = newSet
	if len(msgs) > 0 {
		g.svc.eventSender.Broadcast(&pb.Event{Messages: msgs})
	}
}

func (g *groupsSub) groupEvent(group *model.BlockContentDataviewGroup, remove bool) *pb.EventMessage {
	return event.NewMessage(g.spaceId, &pb.EventMessageValueOfSubscriptionGroups{
		SubscriptionGroups: &pb.EventObjectSubscriptionGroups{
			SubId:  g.subId,
			Group:  group,
			Remove: remove,
		},
	})
}
