package syncsubscriptions

/*
AI generated

Name: Sync Status Subscriptions Manager
Scope: global

## Responsibility
- Manages per-space subscriptions that track objects with pending sync (syncing/queued/error status)
- Tracks limited files (files that hit backup limits) and uploading files per space
- Creates subscriptions lazily on first access per space
*/

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
)

const CName = "client.syncstatus.syncsubscriptions"

type SyncSubscription interface {
	Run() error
	Close()
	GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}]
	LimitedFilesCount() int
	SyncingObjectsCount(missing []string) int
	UploadingFilesCount() int
}

type SyncSubscriptions interface {
	app.ComponentRunnable
	GetSubscription(id string) (SyncSubscription, error)
}

func New() SyncSubscriptions {
	return &syncSubscriptions{
		subs: make(map[string]*subscriptionEntry),
	}
}

type syncSubscriptions struct {
	sync.Mutex
	subscriptionService subscription.Service
	accountService      account.Service
	subs                map[string]*subscriptionEntry
	// newSubscription builds a space's subscription; a field so a test can
	// control how long building takes
	newSubscription func(spaceId string) SyncSubscription
}

// subscriptionEntry is one space's subscription, built once under its own
// lock, so building one space's never holds up another's
type subscriptionEntry struct {
	mu  sync.Mutex
	sub SyncSubscription
}

func (s *syncSubscriptions) Init(a *app.App) (err error) {
	s.subscriptionService = app.MustComponent[subscription.Service](a)
	s.accountService = app.MustComponent[account.Service](a)
	if s.newSubscription == nil {
		s.newSubscription = func(spaceId string) SyncSubscription {
			return newSyncingObjects(spaceId, s.subscriptionService, domain.NewParticipantId(spaceId, s.accountService.AccountID()))
		}
	}
	return
}

func (s *syncSubscriptions) Name() (name string) {
	return CName
}

func (s *syncSubscriptions) Run(ctx context.Context) (err error) {
	return nil
}

// GetSubscription returns the space's subscription, building it on first
// use. A space's check and build happen under that space's lock: two first
// calls racing would build two subscriptions with the same ids, and the
// second would tear down the first's, which could be the one kept, freezing
// the space's counts. The registry lock is held only to find the entry, so a
// slow build (an index that opens slowly) never holds up another space.
func (s *syncSubscriptions) GetSubscription(id string) (SyncSubscription, error) {
	s.Lock()
	entry := s.subs[id]
	if entry == nil {
		entry = &subscriptionEntry{}
		s.subs[id] = entry
	}
	s.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.sub != nil {
		return entry.sub, nil
	}
	sub := s.newSubscription(id)
	if err := sub.Run(); err != nil {
		return nil, fmt.Errorf("run sync subscriptions of space %s: %w", id, err)
	}
	entry.sub = sub
	return sub, nil
}

func (s *syncSubscriptions) Close(ctx context.Context) (err error) {
	s.Lock()
	entries := lo.Values(s.subs)
	s.Unlock()
	for _, entry := range entries {
		entry.mu.Lock()
		if entry.sub != nil {
			entry.sub.Close()
		}
		entry.mu.Unlock()
	}
	return nil
}
