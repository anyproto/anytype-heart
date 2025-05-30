package spaceindex

import (
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
)

type SubscriptionManager struct {
	lock                  sync.RWMutex
	onLinksUpdateCallback func(info LinksUpdateInfo)
}

func (s *SubscriptionManager) SubscribeLinksUpdate(callback func(info LinksUpdateInfo)) {
	s.lock.Lock()
	s.onLinksUpdateCallback = callback
	s.lock.Unlock()
}

func (s *SubscriptionManager) updateObjectLinks(fromId domain.FullID, added []string, removed []string) {
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
