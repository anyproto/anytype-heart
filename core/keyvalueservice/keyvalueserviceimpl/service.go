package keyvalueserviceimpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/keyvalueservice"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const CName = "core.keyvalueservice"

var log = logging.Logger(CName).Desugar()

type subscription struct {
	name         string
	observerFunc keyvalueservice.ObserverFunc
}

type derivedKey string

type service struct {
	lock          sync.RWMutex
	subscriptions map[derivedKey]map[string]subscription

	spaceService space.Service
	techSpace    *clientspace.TechSpace

	techSpaceSalt []byte

	keysLock        sync.Mutex
	keyToDerivedKey map[string]derivedKey
	derivedKeyToKey map[derivedKey]string
}

func New() keyvalueservice.Service {
	return &service{
		subscriptions:   make(map[derivedKey]map[string]subscription),
		keyToDerivedKey: make(map[string]derivedKey),
		derivedKeyToKey: make(map[derivedKey]string),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	s.techSpace = s.spaceService.TechSpace()

	err := s.initTechSpaceSalt()
	if err != nil {
		return fmt.Errorf("init tech salt: %w", err)
	}

	s.techSpace.KeyValueObserver().SetObserver(s.observeChanges)

	return nil
}

func (s *service) initTechSpaceSalt() error {
	commonSpace := s.techSpace.CommonSpace()
	records := commonSpace.Acl().Records()
	if len(records) == 0 {
		return fmt.Errorf("empty acl")
	}
	first := records[0]

	readKeyId, err := commonSpace.Acl().AclState().ReadKeyForAclId(first.Id)
	if err != nil {
		return fmt.Errorf("find read key id: %w", err)
	}

	readKeys := commonSpace.Acl().AclState().Keys()
	key, ok := readKeys[readKeyId]
	if !ok {
		return fmt.Errorf("read key not found")
	}

	rawReadKey, err := key.ReadKey.Raw()
	if err != nil {
		return fmt.Errorf("get raw bytes: %w", err)
	}

	s.techSpaceSalt = rawReadKey
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

			sub.observerFunc(key, keyvalueservice.Value{Data: data, TimestampMilli: kv.TimestampMilli})
		}
		s.lock.RUnlock()

	}
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}

func (s *service) GetUserScopedKey(ctx context.Context, key string) ([]keyvalueservice.Value, error) {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return nil, fmt.Errorf("getDerivedKey: %w", err)
	}
	var result []keyvalueservice.Value
	err = s.techSpace.KeyValueStore().GetAll(ctx, string(derived), func(decryptor keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue) error {
		result = make([]keyvalueservice.Value, 0, len(kvs))
		for _, kv := range kvs {
			data, err := decryptor(kv)
			if err != nil {
				return fmt.Errorf("decrypt: %w", err)
			}
			result = append(result, keyvalueservice.Value{
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

func (s *service) SetUserScopedKey(ctx context.Context, key string, value []byte) error {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return fmt.Errorf("getDerivedKey: %w", err)
	}
	return s.techSpace.KeyValueStore().Set(ctx, string(derived), value)
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
	hasher.Write(s.techSpaceSalt)
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

func (s *service) SubscribeForUserScopedKey(key string, name string, observerFunc keyvalueservice.ObserverFunc) error {
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

	byKey[name] = subscription{
		name:         name,
		observerFunc: observerFunc,
	}
	return nil
}

func (s *service) UnsubscribeFromUserScopedKey(key string, name string) error {
	derived, err := s.getDerivedKey(key)
	if err != nil {
		return fmt.Errorf("getDerivedKey: %w", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[derived]
	if ok {
		delete(byKey, name)
	}
	return nil
}
