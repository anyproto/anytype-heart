package subscription

import (
	"github.com/anyproto/anytype-heart/pb"
)

// EventMatcher dispatches subscription event messages to per-type callbacks.
// It is a consumer-side utility used by services reading internal
// subscription queues
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
