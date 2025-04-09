package keyvalueserviceimpl

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/keyvalueservice"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "core.keyvalueservice"

var log = logging.Logger(CName).Desugar()

type subscription struct {
	key          string
	name         string
	observerFunc keyvalueservice.ObserverFunc
}

type service struct {
	lock          sync.RWMutex
	subscriptions map[string]map[string]subscription

	spaceService space.Service
	techSpace    techspace.TechSpace
}

func New() keyvalueservice.Service {
	return &service{subscriptions: make(map[string]map[string]subscription)}
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

func (s *service) observeChanges(decryptFunc keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue) {
	for _, kv := range kvs {
		s.lock.RLock()
		byKey := s.subscriptions[kv.Key]
		for _, sub := range byKey {
			data, err := decryptFunc(kv)
			if err != nil {
				log.Error("can't decrypt value", zap.Error(err))
				continue
			}
			sub.observerFunc(kv.Key, keyvalueservice.Value{Data: data, TimestampMilli: kv.TimestampMilli})
		}
		s.lock.RUnlock()

	}
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}

func (s *service) GetUserScopedKey(ctx context.Context, key string) ([]keyvalueservice.Value, error) {
	kvs, decryptor, err := s.techSpace.KeyValueStore().GetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get all: %w", err)
	}

	result := make([]keyvalueservice.Value, 0, len(kvs))
	for _, kv := range kvs {
		data, err := decryptor(kv)
		if err != nil {
			return nil, fmt.Errorf("decrypt: %w", err)
		}
		result = append(result, keyvalueservice.Value{
			Data:           data,
			TimestampMilli: kv.TimestampMilli,
		})
	}
	return result, nil
}

func (s *service) SetUserScopedKey(ctx context.Context, key string, value []byte) error {
	return s.techSpace.KeyValueStore().Set(ctx, key, value)
}

func (s *service) SubscribeForUserScopedKey(key string, name string, observerFunc keyvalueservice.ObserverFunc) error {
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
