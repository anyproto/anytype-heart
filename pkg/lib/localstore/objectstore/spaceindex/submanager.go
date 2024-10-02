package spaceindex

import (
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

type SubscriptionManager struct {
	lock                  sync.RWMutex
	subscriptions         []database.Subscription
	onLinksUpdateCallback func(info LinksUpdateInfo)
	onChangeCallback      func(record database.Record)
}

func (s *SubscriptionManager) SubscribeForAll(callback func(record database.Record)) {
	s.lock.Lock()
	s.onChangeCallback = callback
	s.lock.Unlock()
}

func (s *SubscriptionManager) SubscribeLinksUpdate(callback func(info LinksUpdateInfo)) {
	s.lock.Lock()
	s.onLinksUpdateCallback = callback
	s.lock.Unlock()
}

func (s *SubscriptionManager) updateObjectLinks(fromId string, added []string, removed []string) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.onLinksUpdateCallback != nil && len(added)+len(removed) > 0 {
		s.onLinksUpdateCallback(LinksUpdateInfo{
			LinksFromId: fromId,
			Added:       added,
			Removed:     removed,
		})
	}
}

// unsafe, use under mutex
func (s *SubscriptionManager) addSubscriptionIfNotExists(sub database.Subscription) (existed bool) {
	for _, s := range s.subscriptions {
		if s == sub {
			return true
		}
	}

	s.subscriptions = append(s.subscriptions, sub)
	return false
}

func (s *SubscriptionManager) closeAndRemoveSubscription(subscription database.Subscription) {
	s.lock.Lock()
	defer s.lock.Unlock()
	subscription.Close()

	for i, sub := range s.subscriptions {
		if sub == subscription {
			s.subscriptions = append(s.subscriptions[:i], s.subscriptions[i+1:]...)
			break
		}
	}
}

func (s *SubscriptionManager) sendUpdatesToSubscriptions(id string, details *domain.Details) {
	detCopy := details.Copy()
	detCopy.SetString(bundle.RelationKeyId, id)
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for _, sub := range s.subscriptions {
		_ = sub.PublishAsync(id, detCopy)
	}
}
