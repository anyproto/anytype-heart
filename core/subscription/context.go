package subscription

import (
	"slices"
	"sort"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

type opChange struct {
	id    string
	subId string
	keys  []domain.RelationKey
}

type opRemove struct {
	id    string
	subId string
}

type opPosition struct {
	id      string
	subId   string
	afterId string
	keys    []domain.RelationKey
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
	spaceId string
	outputs map[string][]*pb.EventMessage

	// subIds for remove
	remove   []opRemove
	change   []opChange
	position []opPosition
	counters []opCounter
	entries  []*entry
	groups   []opGroup

	// entriesIdx indexes entries by id; built lazily on the first getEntry call
	// (entries are assigned wholesale at the start of a batch) and kept in sync
	// by appendEntry afterwards
	entriesIdx      map[string]int
	entriesIdxValid bool

	keysBuf []struct {
		id     string
		subIds []string
		keys   []domain.RelationKey
	}
	// keysBufIdx indexes keysBuf by id
	keysBufIdx map[string]int

	c *cache
}

const defaultOutput = "_default"

func (ctx *opCtx) apply() {
	addEvent := func(subId string, ev *pb.EventMessage) {
		_, ok := ctx.outputs[subId]
		if ok {
			ctx.outputs[subId] = append(ctx.outputs[subId], ev)
		} else {
			ctx.outputs[defaultOutput] = append(ctx.outputs[defaultOutput], ev)
		}
	}

	// changes
	for _, ch := range ctx.change {
		ctx.collectKeys(ch.id, ch.subId, ch.keys)
	}

	// details events
	ctx.detailsEvents()

	// adds, positions
	for _, pos := range ctx.position {
		if pos.isAdd {
			ctx.collectKeys(pos.id, pos.subId, pos.keys)
			addEvent(pos.subId, event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					Id:      pos.id,
					AfterId: pos.afterId,
					SubId:   pos.subId,
				},
			},
			))
		} else {
			addEvent(pos.subId, event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfSubscriptionPosition{
				SubscriptionPosition: &pb.EventObjectSubscriptionPosition{
					Id:      pos.id,
					AfterId: pos.afterId,
					SubId:   pos.subId,
				},
			},
			))
		}
	}

	// removes
	for _, rem := range ctx.remove {
		addEvent(rem.subId, event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfSubscriptionRemove{
			SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
				Id:    rem.id,
				SubId: rem.subId,
			},
		},
		))
	}

	// counters
	for _, count := range ctx.counters {
		addEvent(count.subId, event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				Total:     int64(count.total),
				NextCount: int64(count.nextCount),
				PrevCount: int64(count.prevCount),
				SubId:     count.subId,
			},
		},
		))
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
		addEvent(opGroup.subId, event.NewMessage(ctx.spaceId,
			&pb.EventMessageValueOfSubscriptionGroups{
				SubscriptionGroups: &pb.EventObjectSubscriptionGroups{
					SubId:  opGroup.subId,
					Group:  opGroup.group,
					Remove: opGroup.remove,
				},
			},
		))
	}
}

// detailsEvents produces following types of events:
// EventObjectDetailsAmend
// EventObjectDetailsUnset
// EventMessageValueOfObjectDetailsSet
func (ctx *opCtx) detailsEvents() {
	var msgs []*pb.EventMessage
	for _, info := range ctx.keysBuf {
		curr := ctx.getEntry(info.id)
		if curr == nil {
			log.Errorf("entry present in changes but not in list: %v", info.id)
			continue
		}
		prev := ctx.c.Get(info.id)
		msgs = ctx.addDetailsEvents(prev, curr, info, msgs)
		// save info for every sub because we don't want to send the details events again
		for _, sub := range info.subIds {
			curr.SetSub(sub, true, true)
		}
	}

	ctx.groupDetailsEvents(msgs)
}

func (ctx *opCtx) addDetailsEvents(prev, curr *entry, info struct {
	id     string
	subIds []string
	keys   []domain.RelationKey
}, msgs []*pb.EventMessage) []*pb.EventMessage {
	var subIdsToSendAmendDetails, subIdsToSendSetDetails []string
	if prev != nil {
		active := prev.GetActive()
		detailsSent := prev.GetFullDetailsSent()
		subIdsToSendAmendDetails = lo.Intersect(active, detailsSent)
		sort.Strings(subIdsToSendAmendDetails)

		subIdsToSendSetDetails = slice.Difference(info.subIds, subIdsToSendAmendDetails)
		sort.Strings(subIdsToSendSetDetails)
		if len(subIdsToSendAmendDetails) != 0 {
			diff, keysToUnset := domain.StructDiff(prev.data, curr.data)
			msgs = append(msgs, state.StructDiffIntoEventsWithSubIds(ctx.spaceId, info.id, diff, info.keys, keysToUnset, subIdsToSendAmendDetails)...)
		}
		if len(subIdsToSendSetDetails) != 0 {
			msgs = ctx.appendObjectDetailsSetMessage(msgs, curr, subIdsToSendSetDetails, info.keys)
		}
	} else {
		msgs = ctx.appendObjectDetailsSetMessage(msgs, curr, slices.Clone(info.subIds), info.keys)
	}
	return msgs
}

func (ctx *opCtx) appendObjectDetailsSetMessage(msgs []*pb.EventMessage, curr *entry, subIds []string, keys []domain.RelationKey) []*pb.EventMessage {
	msgs = append(msgs, event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id:      curr.id,
			Details: curr.data.ToProtoOnlyKeys(keys...),
			SubIds:  subIds,
		},
	},
	))
	return msgs
}

func (ctx *opCtx) groupDetailsEvents(msgs []*pb.EventMessage) {
	for _, msg := range msgs {
		if v := msg.GetObjectDetailsAmend(); v != nil {
			ctx.groupEventsDetailsAmend(v)
		} else if v := msg.GetObjectDetailsUnset(); v != nil {
			ctx.groupEventsDetailsUnset(v)
		} else if v := msg.GetObjectDetailsSet(); v != nil {
			ctx.groupEventsDetailsSet(v)
		}
	}
}

func (ctx *opCtx) groupEventsDetailsSet(v *pb.EventObjectDetailsSet) {
	defaultSubIds := v.SubIds[:0]
	for _, subId := range v.SubIds {
		if _, ok := ctx.outputs[subId]; ok {
			ctx.outputs[subId] = append(ctx.outputs[subId], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsSet{
				ObjectDetailsSet: &pb.EventObjectDetailsSet{
					Id:      v.Id,
					Details: v.Details,
					SubIds:  []string{subId},
				},
			},
			))
		} else {
			defaultSubIds = append(defaultSubIds, subId)
		}
	}
	if len(defaultSubIds) > 0 {
		ctx.outputs[defaultOutput] = append(ctx.outputs[defaultOutput], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      v.Id,
				Details: v.Details,
				SubIds:  defaultSubIds,
			},
		},
		))
	}
}

func (ctx *opCtx) groupEventsDetailsUnset(v *pb.EventObjectDetailsUnset) {
	defaultSubIds := v.SubIds[:0]
	for _, subId := range v.SubIds {
		if _, ok := ctx.outputs[subId]; ok {
			ctx.outputs[subId] = append(ctx.outputs[subId], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsUnset{
				ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
					Id:     v.Id,
					Keys:   v.Keys,
					SubIds: []string{subId},
				},
			},
			))
		} else {
			defaultSubIds = append(defaultSubIds, subId)
		}
	}
	if len(defaultSubIds) > 0 {
		ctx.outputs[defaultOutput] = append(ctx.outputs[defaultOutput], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsUnset{
			ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
				Id:     v.Id,
				Keys:   v.Keys,
				SubIds: defaultSubIds,
			},
		},
		))
	}
}

func (ctx *opCtx) groupEventsDetailsAmend(v *pb.EventObjectDetailsAmend) {
	defaultSubIds := v.SubIds[:0]
	for _, subId := range v.SubIds {
		if _, ok := ctx.outputs[subId]; ok {
			ctx.outputs[subId] = append(ctx.outputs[subId], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsAmend{
				ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
					Id:      v.Id,
					Details: v.Details,
					SubIds:  []string{subId},
				},
			},
			))
		} else {
			defaultSubIds = append(defaultSubIds, subId)
		}
	}
	if len(defaultSubIds) > 0 {
		ctx.outputs[defaultOutput] = append(ctx.outputs[defaultOutput], event.NewMessage(ctx.spaceId, &pb.EventMessageValueOfObjectDetailsAmend{
			ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
				Id:      v.Id,
				Details: v.Details,
				SubIds:  defaultSubIds,
			},
		},
		))
	}
}

func (ctx *opCtx) collectKeys(id string, subId string, keys []domain.RelationKey) {
	if i, ok := ctx.keysBufIdx[id]; ok {
		kb := ctx.keysBuf[i]
		for _, k := range keys {
			if slice.FindPos(ctx.keysBuf[i].keys, k) == -1 {
				ctx.keysBuf[i].keys = append(ctx.keysBuf[i].keys, k)
			}
		}
		if slice.FindPos(kb.subIds, subId) == -1 {
			ctx.keysBuf[i].subIds = append(kb.subIds, subId)
			sort.Strings(ctx.keysBuf[i].subIds)
		}
		return
	}
	keysCopy := make([]domain.RelationKey, len(keys))
	copy(keysCopy, keys)
	ctx.keysBuf = append(ctx.keysBuf, struct {
		id     string
		subIds []string
		keys   []domain.RelationKey
	}{id: id, keys: keysCopy, subIds: []string{subId}})
	if ctx.keysBufIdx == nil {
		ctx.keysBufIdx = make(map[string]int)
	}
	ctx.keysBufIdx[id] = len(ctx.keysBuf) - 1
}

func (ctx *opCtx) getEntry(id string) *entry {
	if !ctx.entriesIdxValid {
		if ctx.entriesIdx == nil {
			ctx.entriesIdx = make(map[string]int, len(ctx.entries))
		} else {
			clear(ctx.entriesIdx)
		}
		for i, e := range ctx.entries {
			ctx.entriesIdx[e.id] = i
		}
		ctx.entriesIdxValid = true
	}
	if i, ok := ctx.entriesIdx[id]; ok {
		return ctx.entries[i]
	}
	return nil
}

// appendEntry must be used to add entries after batch processing has started,
// so that the id index stays in sync
func (ctx *opCtx) appendEntry(e *entry) {
	ctx.entries = append(ctx.entries, e)
	if ctx.entriesIdxValid {
		ctx.entriesIdx[e.id] = len(ctx.entries) - 1
	}
}

func (ctx *opCtx) reset() {
	ctx.remove = ctx.remove[:0]
	ctx.change = ctx.change[:0]
	ctx.position = ctx.position[:0]
	ctx.counters = ctx.counters[:0]
	ctx.keysBuf = ctx.keysBuf[:0]
	ctx.entries = ctx.entries[:0]
	ctx.groups = ctx.groups[:0]
	ctx.entriesIdxValid = false
	clear(ctx.keysBufIdx)
	if ctx.outputs == nil {
		ctx.outputs = map[string][]*pb.EventMessage{
			defaultOutput: nil,
		}
	}
}

type EventMatcher struct {
	OnAdd      func(spaceId string, msg *pb.EventObjectSubscriptionAdd)
	OnRemove   func(spaceId string, msg *pb.EventObjectSubscriptionRemove)
	OnPosition func(spaceId string, msg *pb.EventObjectSubscriptionPosition)
	OnSet      func(spaceId string, msg *pb.EventObjectDetailsSet)
	OnUnset    func(spaceId string, msg *pb.EventObjectDetailsUnset)
	OnAmend    func(spaceId string, msg *pb.EventObjectDetailsAmend)
	OnCounters func(spaceId string, msg *pb.EventObjectSubscriptionCounters)
	OnGroups   func(spaceId string, msg *pb.EventObjectSubscriptionGroups)
}

func (m EventMatcher) Match(msg *pb.EventMessage) {
	if msg == nil || msg.Value == nil {
		return
	}
	switch v := msg.Value.(type) {
	case *pb.EventMessageValueOfSubscriptionAdd:
		if m.OnAdd != nil {
			m.OnAdd(msg.SpaceId, v.SubscriptionAdd)
		}
	case *pb.EventMessageValueOfSubscriptionRemove:
		if m.OnRemove != nil {
			m.OnRemove(msg.SpaceId, v.SubscriptionRemove)
		}
	case *pb.EventMessageValueOfSubscriptionPosition:
		if m.OnPosition != nil {
			m.OnPosition(msg.SpaceId, v.SubscriptionPosition)
		}
	case *pb.EventMessageValueOfObjectDetailsSet:
		if m.OnSet != nil {
			m.OnSet(msg.SpaceId, v.ObjectDetailsSet)
		}
	case *pb.EventMessageValueOfObjectDetailsUnset:
		if m.OnUnset != nil {
			m.OnUnset(msg.SpaceId, v.ObjectDetailsUnset)
		}
	case *pb.EventMessageValueOfObjectDetailsAmend:
		if m.OnAmend != nil {
			m.OnAmend(msg.SpaceId, v.ObjectDetailsAmend)
		}
	case *pb.EventMessageValueOfSubscriptionCounters:
		if m.OnCounters != nil {
			m.OnCounters(msg.SpaceId, v.SubscriptionCounters)
		}
	case *pb.EventMessageValueOfSubscriptionGroups:
		if m.OnGroups != nil {
			m.OnGroups(msg.SpaceId, v.SubscriptionGroups)
		}
	}
}
