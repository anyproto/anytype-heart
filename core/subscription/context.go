package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

type opChange struct {
	id      string
	subId   string
	keys    []string
	afterId string
}

type opRemove struct {
	id    string
	subId string
}

type opPosition struct {
	id      string
	subId   string
	afterId string
}

type opCounter struct {
	subId     string
	total     int
	prevCount int
	nextCount int
}

type opCtx struct {
	// subIds for remove
	remove     []opRemove
	change     []opChange
	add        []opChange
	position   []opPosition
	counters   []opCounter
	depEntries []*entry

	keysBuf []struct {
		id     string
		subIds []string
		keys   []string
	}
}

func (ctx *opCtx) apply(c *cache, entries []*entry) (events []*pb.Event) {
	var byEventsContext = make(map[string][]*pb.EventMessage)
	var appendToContext = func(contextId string, msg ...*pb.EventMessage) {
		msgs, ok := byEventsContext[contextId]
		if ok {
			byEventsContext[contextId] = append(msgs, msg...)
		} else {
			byEventsContext[contextId] = msg
		}
	}

	// adds
	for _, add := range ctx.add {
		ctx.collectKeys(add.id, add.subId, add.keys)
		appendToContext(add.subId, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					Id:      add.id,
					AfterId: add.afterId,
				},
			},
		})
	}

	// changes
	for _, ch := range ctx.change {
		ctx.collectKeys(ch.id, ch.subId, ch.keys)
	}

	// details events
	appendToContext("", ctx.detailsEvents(c, entries)...)

	// positions
	for _, pos := range ctx.position {
		appendToContext(pos.subId, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionPosition{
				SubscriptionPosition: &pb.EventObjectSubscriptionPosition{
					Id:      pos.id,
					AfterId: pos.afterId,
				},
			},
		})
	}

	// removes
	for _, rem := range ctx.remove {
		appendToContext(rem.subId, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					Id: rem.id,
				},
			},
		})
	}

	// counters
	for _, count := range ctx.counters {
		appendToContext(count.subId, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					Total:     int64(count.total),
					NextCount: int64(count.nextCount),
					PrevCount: int64(count.prevCount),
				},
			},
		})
	}

	// apply to cache
	for _, e := range entries {
		c.set(e)
	}
	for _, e := range ctx.depEntries {
		if c.pick(e.id) == nil {
			c.set(e)
		}
	}

	events = make([]*pb.Event, 0, len(byEventsContext))

	// event with details must be first to send
	if messages, ok := byEventsContext[""]; ok {
		events = append(events, &pb.Event{
			Messages: messages,
		})
		delete(byEventsContext, "")
	}

	for contextId, messages := range byEventsContext {
		events = append(events, &pb.Event{
			Messages:  messages,
			ContextId: contextId,
		})
	}
	return
}

func (ctx *opCtx) detailsEvents(c *cache, entries []*entry) (msgs []*pb.EventMessage) {
	var getEntry = func(id string) *entry {
		for _, e := range entries {
			if e.id == id {
				return e
			}
		}
		for _, e := range ctx.depEntries {
			if e.id == id {
				return e
			}
		}
		return nil
	}
	for _, info := range ctx.keysBuf {
		curr := getEntry(info.id)
		if curr == nil {
			log.Errorf("entry present in changes but not in list: %v", info.id)
			continue
		}
		prev := c.pick(info.id)
		var prevData *types.Struct
		if prev != nil {
			prevData = prev.data
		}
		diff := pbtypes.StructDiff(prevData, curr.data)
		msgs = append(msgs, state.StructDiffIntoEventsWithSubIds(info.id, diff, info.keys, info.subIds)...)
	}
	return
}

func (ctx *opCtx) collectKeys(id string, subId string, keys []string) {
	var found bool
	for i, kb := range ctx.keysBuf {
		if kb.id == id {
			found = true
			for _, k := range keys {
				if slice.FindPos(kb.keys, k) == -1 {
					ctx.keysBuf[i].keys = append(kb.keys, k)
				}
			}
			if slice.FindPos(kb.subIds, subId) == -1 {
				ctx.keysBuf[i].subIds = append(kb.subIds, subId)
			}
			break
		}
	}
	if !found {
		keysCopy := make([]string, len(keys))
		copy(keysCopy, keys)
		ctx.keysBuf = append(ctx.keysBuf, struct {
			id     string
			subIds []string
			keys   []string
		}{id: id, keys: keysCopy, subIds: []string{subId}})
	}
}

func (ctx *opCtx) reset() {
	ctx.remove = ctx.remove[:0]
	ctx.change = ctx.change[:0]
	ctx.add = ctx.add[:0]
	ctx.position = ctx.position[:0]
	ctx.counters = ctx.counters[:0]
	ctx.keysBuf = ctx.keysBuf[:0]
	ctx.depEntries = ctx.depEntries[:0]
}
