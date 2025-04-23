package keyvalueservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/keyvalueobserver"
)

const CName = "core.keyvalueservice"

var log = logging.Logger(CName).Desugar()

type ObserverFunc func(key string, val Value)

type Value struct {
	Data           []byte
	TimestampMilli int
}

type subscription struct {
	name         string
	observerFunc ObserverFunc
}

type derivedKey string

type Service interface {
	Get(ctx context.Context, key string) ([]Value, error)
	Set(ctx context.Context, key string, value []byte) error
	SubscribeForKey(key string, subscriptionName string, observerFunc ObserverFunc) error
	UnsubscribeFromKey(key string, subscriptionName string) error
}

type service struct {
	lock          sync.RWMutex
	subscriptions map[derivedKey]map[string]subscription

	keyValueStore keyvaluestorage.Storage
	spaceCore     commonspace.Space
	observer      keyvalueobserver.Observer

	keysLock        sync.Mutex
	spaceSalt       []byte
	keyToDerivedKey map[string]derivedKey
	derivedKeyToKey map[derivedKey]string
}

func New(spaceCore commonspace.Space, observer keyvalueobserver.Observer) (Service, error) {
	s := &service{
		spaceCore:       spaceCore,
		observer:        observer,
		keyValueStore:   spaceCore.KeyValue().DefaultStore(),
		subscriptions:   make(map[derivedKey]map[string]subscription),
		keyToDerivedKey: make(map[string]derivedKey),
		derivedKeyToKey: make(map[derivedKey]string),
	}
	err := s.initSpaceSalt()
	if err != nil {
		return nil, fmt.Errorf("init tech salt: %w", err)
	}

	s.observer.SetObserver(s.observeChanges)
	return s, nil
}

func (s *service) initSpaceSalt() error {
	records := s.spaceCore.Acl().Records()
	if len(records) == 0 {
		return fmt.Errorf("empty acl")
	}
	first := records[0]

	readKeyId, err := s.spaceCore.Acl().AclState().ReadKeyForAclId(first.Id)
	if err != nil {
		return fmt.Errorf("find read key id: %w", err)
	}

	readKeys := s.spaceCore.Acl().AclState().Keys()
	key, ok := readKeys[readKeyId]
	if !ok {
		return fmt.Errorf("read key not found")
	}

	rawReadKey, err := key.ReadKey.Raw()
	if err != nil {
		return fmt.Errorf("get raw bytes: %w", err)
	}

	s.spaceSalt = rawReadKey
	return nil
}

func (s *service) observeChanges(decryptFunc keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue) {
	for _, kv := range kvs {
		s.lock.RLock()
		byKey := s.subscriptions[derivedKey(kv.Key)]
		for _, sub := range byKey {
			data, err := decryptFunc(kv)
			if err != nil {
				log.Error("can't decrypt value", zap.Error(err))
				continue
			}

			key, ok := s.getKeyFromDerived(derivedKey(kv.Key))
			if !ok {
				log.Error("can't get key from derived key", zap.String("subName", sub.name))
				continue
			}

			sub.observerFunc(key, Value{Data: data, TimestampMilli: kv.TimestampMilli})
		}
		s.lock.RUnlock()

	}
}

func (s *service) Get(ctx context.Context, key string) ([]Value, error) {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return nil, fmt.Errorf("getDerivedKey: %w", err)
	}
	var result []Value
	err = s.keyValueStore.GetAll(ctx, string(derived), func(decryptor keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue) error {
		result = make([]Value, 0, len(kvs))
		for _, kv := range kvs {
			data, err := decryptor(kv)
			if err != nil {
				return fmt.Errorf("decrypt: %w", err)
			}
			result = append(result, Value{
				Data:           data,
				TimestampMilli: kv.TimestampMilli,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get all: %w", err)
	}
	return result, nil
}

func (s *service) Set(ctx context.Context, key string, value []byte) error {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return fmt.Errorf("getDerivedKey: %w", err)
	}
	return s.keyValueStore.Set(ctx, string(derived), value)
}

func (s *service) getDerivedKey(key string) (derivedKey, error) {
	s.keysLock.Lock()
	defer s.keysLock.Unlock()

	derived, ok := s.keyToDerivedKey[key]
	if ok {
		return derived, nil
	}

	hasher := sha256.New()
	// Salt
	hasher.Write(s.spaceSalt)
	// User key
	hasher.Write([]byte(key))
	result := hasher.Sum(nil)

	derived = derivedKey(hex.EncodeToString(result))

	s.keyToDerivedKey[key] = derived
	s.derivedKeyToKey[derived] = key
	return derived, nil
}

func (s *service) getKeyFromDerived(derived derivedKey) (string, bool) {
	s.keysLock.Lock()
	defer s.keysLock.Unlock()

	key, ok := s.derivedKeyToKey[derived]
	return key, ok
}

func (s *service) SubscribeForKey(key string, subscriptionName string, observerFunc ObserverFunc) error {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return fmt.Errorf("getDerivedKey: %w", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[derived]
	if !ok {
		byKey = make(map[string]subscription)
		s.subscriptions[derived] = byKey
	}

	byKey[subscriptionName] = subscription{
		name:         subscriptionName,
		observerFunc: observerFunc,
	}
	return nil
}

func (s *service) UnsubscribeFromKey(key string, subscriptionName string) error {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return fmt.Errorf("getDerivedKey: %w", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[derived]
	if ok {
		delete(byKey, subscriptionName)
	}
	return nil
}
