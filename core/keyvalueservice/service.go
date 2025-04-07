package keyvalueservice

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"

	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "core.keyvalueservice"

type ObserverFunc func(key string, value []byte)

type Service interface {
	app.ComponentRunnable

	GetUserScopedKey(key string) ([]byte, error)
	SetUserScopedKey(key string, value []byte) error
	SubscribeForUserScopedKey(key string, subscriptionName string, observerFunc ObserverFunc) error
	UnsubscribeFromUserScopedKey(key string, subscriptionName string) error
}

type subscription struct {
	key          string
	name         string
	observerFunc ObserverFunc
}

type service struct {
	lock          sync.RWMutex
	subscriptions map[string]map[string]subscription

	spaceService space.Service
	techSpace    techspace.TechSpace
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.techSpace = s.spaceService.TechSpace()

	s.techSpace.KeyValueObserver().SetObserver(s.observeChanges)

	return nil
}

func (s *service) observeChanges(keyValue ...innerstorage.KeyValue) {
	for _, kv := range keyValue {
		s.lock.RLock()
		byKey := s.subscriptions[kv.Key]
		for _, sub := range byKey {
			sub.observerFunc(kv.Key, kv.Value.Value)
		}
		s.lock.RUnlock()

	}
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}

func (s *service) GetUserScopedKey(key string) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (s *service) SetUserScopedKey(key string, value []byte) error {
	// TODO implement me
	panic("implement me")
}

func (s *service) SubscribeForUserScopedKey(key string, name string, observerFunc ObserverFunc) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[key]
	if !ok {
		byKey = make(map[string]subscription)
		s.subscriptions[key] = byKey
	}

	byKey[name] = subscription{
		key:          key,
		name:         name,
		observerFunc: observerFunc,
	}
	return nil
}

func (s *service) UnsubscribeFromUserScopedKey(key string, name string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[key]
	if ok {
		delete(byKey, name)
	}
	return nil
}
