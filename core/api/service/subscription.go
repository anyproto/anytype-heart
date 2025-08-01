package service

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/cheggaaa/mb/v3"
)

// subscriptionTracker groups subscription-related data for a single object type
type subscriptionTracker struct {
	subId  string
	queue  *mb.MB[*pb.EventMessage]
	objSub interface{ Close() }
}

func (s *subscriptionTracker) close() {
	if s.objSub != nil {
		s.objSub.Close()
	}
	if s.queue != nil {
		s.queue.Close()
	}
}

// subscriptions groups all subscription trackers
type subscriptions struct {
	properties *subscriptionTracker
	types      *subscriptionTracker
	tags       *subscriptionTracker
}

func newSubscriptions() *subscriptions {
	return &subscriptions{
		properties: &subscriptionTracker{},
		types:      &subscriptionTracker{},
		tags:       &subscriptionTracker{},
	}
}

func (s *subscriptions) close() {
	if s.properties != nil {
		s.properties.close()
	}
	if s.types != nil {
		s.types.close()
	}
	if s.tags != nil {
		s.tags.close()
	}
}
