package syncsubscriptions

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
)

const CName = "client.syncstatus.syncsubscriptions"

type SyncSubscription interface {
	Run() error
	Close()
	GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}]
	NotSyncedFilesCount() int 
	SyncingObjectsCount(missing []string) int
}

type SyncSubscriptions interface {
	app.ComponentRunnable
	GetSubscription(id string) (SyncSubscription, error)
}

func New() SyncSubscriptions {
	return &syncSubscriptions{
		subs: make(map[string]SyncSubscription),
	}
}

type syncSubscriptions struct {
	sync.Mutex
	service subscription.Service
	subs    map[string]SyncSubscription
}

func (s *syncSubscriptions) Init(a *app.App) (err error) {
	s.service = app.MustComponent[subscription.Service](a)
	return
}

func (s *syncSubscriptions) Name() (name string) {
	return CName
}

func (s *syncSubscriptions) Run(ctx context.Context) (err error) {
	return nil
}

func (s *syncSubscriptions) GetSubscription(id string) (SyncSubscription, error) {
	s.Lock()
	curSub := s.subs[id]
	s.Unlock()
	if curSub != nil {
		return curSub, nil
	}
	sub := newSyncingObjects(id, s.service)
	err := sub.Run()
	if err != nil {
		return nil, err
	}
	s.Lock()
	s.subs[id] = sub
	s.Unlock()
	return sub, nil
}

func (s *syncSubscriptions) Close(ctx context.Context) (err error) {
	s.Lock()
	subs := lo.Values(s.subs)
	s.Unlock()
	for _, sub := range subs {
		sub.Close()
	}
	return nil
}
