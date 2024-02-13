package subscription

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type opChange struct {
	id    string
	subId string
	keys  []string
}

type opRemove struct {
	id    string
	subId string
}

type opPosition struct {
	id      string
	subId   string
	afterId string
	keys    []string
	isAdd   bool
}

type opCounter struct {
	subId     string
	total     int
	prevCount int
	nextCount int
}

type opGroup struct {
	subId  string
	group  *model.BlockContentDataviewGroup
	remove bool
}

type opCtx struct {
	// subIds for remove
	remove   []opRemove
	change   []opChange
	position []opPosition
	counters []opCounter
	entries  []*entry
	groups   []opGroup

	keysBuf []struct {
		id     string
		subIds []string
		keys   []string
	}

	c *cache
}

func (ctx *opCtx) apply() (event *pb.Event) {
	var subMsgs = make([]*pb.EventMessage, 0, 10)

	// adds, positions
	for _, pos := range ctx.position {
		if pos.isAdd {
			ctx.collectKeys(pos.id, pos.subId, pos.keys)
			subMsgs = append(subMsgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfSubscriptionAdd{
					SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
						Id:      pos.id,
						AfterId: pos.afterId,
						SubId:   pos.subId,
					},
				},
			})
		} else {
			subMsgs = append(subMsgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfSubscriptionPosition{
					SubscriptionPosition: &pb.EventObjectSubscriptionPosition{
						Id:      pos.id,
						AfterId: pos.afterId,
						SubId:   pos.subId,
					},
				},
			})
		}
	}

	// changes
	for _, ch := range ctx.change {
		ctx.collectKeys(ch.id, ch.subId, ch.keys)
	}

	// details events
	eventMsgs := ctx.detailsEvents()

	// removes
	for _, rem := range ctx.remove {
		subMsgs = append(subMsgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					Id:    rem.id,
					SubId: rem.subId,
				},
			},
		})
	}

	// counters
	for _, count := range ctx.counters {
		subMsgs = append(subMsgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					Total:     int64(count.total),
					NextCount: int64(count.nextCount),
					PrevCount: int64(count.prevCount),
					SubId:     count.subId,
				},
			},
		})
	}

	// apply to cache
	for _, e := range ctx.entries {
		if len(e.SubIds()) > 0 {
			ctx.c.Set(e)
		} else {
			ctx.c.Remove(e.id)
		}
	}

	for _, opGroup := range ctx.groups {
		subMsgs = append(subMsgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionGroups{
				SubscriptionGroups: &pb.EventObjectSubscriptionGroups{
					SubId:  opGroup.subId,
					Group:  opGroup.group,
					Remove: opGroup.remove,
				},
			},
		})
	}

	return &pb.Event{
		Messages: append(eventMsgs, subMsgs...),
	}
}

func (ctx *opCtx) detailsEvents() (msgs []*pb.EventMessage) {
	var getEntry = func(id string) *entry {
		for _, e := range ctx.entries {
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
		prev := ctx.c.Get(info.id)
		var prevData *types.Struct
		if prev != nil && prev.IsActive(info.subIds...) && prev.IsFullDetailsSent(info.subIds...) {
			prevData = prev.data
			diff := pbtypes.StructDiff(prevData, curr.data)
			msgs = append(msgs, state.StructDiffIntoEventsWithSubIds(info.id, diff, info.keys, info.subIds)...)
		} else {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectDetailsSet{
					ObjectDetailsSet: &pb.EventObjectDetailsSet{
						Id:      curr.id,
						Details: pbtypes.StructFilterKeys(curr.data, info.keys),
						SubIds:  info.subIds,
					},
				},
			})
		}
		// save info for every sub because we don't want to send the details events again
		for _, sub := range info.subIds {
			curr.SetSub(sub, true, true)
		}
	}
	return
}

func (ctx *opCtx) collectKeys(id string, subId string, keys []string) {
	var found bool
	for i, kb := range ctx.keysBuf {
		if kb.id == id {
			found = true
			for _, k := range keys {
				if slice.FindPos(ctx.keysBuf[i].keys, k) == -1 {
					ctx.keysBuf[i].keys = append(ctx.keysBuf[i].keys, k)
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

func (ctx *opCtx) getEntry(id string) *entry {
	for _, e := range ctx.entries {
		if e.id == id {
			return e
		}
	}
	return nil
}

func (ctx *opCtx) reset() {
	ctx.remove = ctx.remove[:0]
	ctx.change = ctx.change[:0]
	ctx.position = ctx.position[:0]
	ctx.counters = ctx.counters[:0]
	ctx.keysBuf = ctx.keysBuf[:0]
	ctx.entries = ctx.entries[:0]
	ctx.groups = ctx.groups[:0]
}
