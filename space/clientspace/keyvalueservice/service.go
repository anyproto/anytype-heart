package keyvalueservice

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
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
	Key            string
	Data           []byte
	TimestampMilli int
}

type subscription struct {
	name         string
	observerFunc ObserverFunc
}

type derivedKey string

// Service provides convenient wrapper for using per-space key-value store.
// It automatically hashes keys for security reasons: no one except you can see actual value of a key. How it works:
// - A key (client key) is hashed using a salt. Salt is the first read key from the space. Call it derived key
// - Then we use derived key as actual key for storing the value
// - And we put the original client key inside encrypted value
//
// Finally, key value pair looks like this:
// hash(key) -> (key, value)
//
// Why use hash of keys instead of AES encryption? Because the output of hash function is much more compact,
// and we're still able to get the original key because we already encrypt value.
//
// The maximum length of a key is 65535
type Service interface {
	Get(ctx context.Context, key string) ([]Value, error)
	Set(ctx context.Context, key string, value []byte) error
	SubscribeForKey(key string, subscriptionName string, observerFunc ObserverFunc) error
	UnsubscribeFromKey(key string, subscriptionName string) error
}

type service struct {
	lock          sync.RWMutex
	subscriptions map[string]map[string]subscription

	subscriptionBuf []subscription

	keyValueStore keyvaluestorage.Storage
	spaceCore     commonspace.Space
	observer      keyvalueobserver.Observer

	keysLock        sync.Mutex
	spaceSalt       []byte
	keyToDerivedKey map[string]derivedKey
}

func New(spaceCore commonspace.Space, observer keyvalueobserver.Observer) (Service, error) {
	s := &service{
		spaceCore:       spaceCore,
		observer:        observer,
		keyValueStore:   spaceCore.KeyValue().DefaultStore(),
		subscriptions:   make(map[string]map[string]subscription),
		keyToDerivedKey: make(map[string]derivedKey),
	}
	s.observer.SetObserver(s.observeChanges)
	return s, nil
}

func (s *service) initSpaceSalt() ([]byte, error) {
	records := s.spaceCore.Acl().Records()
	if len(records) == 0 {
		return nil, fmt.Errorf("empty acl")
	}
	first := records[0]

	readKeyId, err := s.spaceCore.Acl().AclState().ReadKeyForAclId(first.Id)
	if err != nil {
		return nil, fmt.Errorf("find read key id: %w", err)
	}

	readKeys := s.spaceCore.Acl().AclState().Keys()
	key, ok := readKeys[readKeyId]
	if !ok {
		return nil, fmt.Errorf("read key not found")
	}

	rawReadKey, err := key.ReadKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get raw bytes: %w", err)
	}
	return rawReadKey, nil
}

func (s *service) getSalt() ([]byte, error) {
	if s.spaceSalt == nil {
		salt, err := s.initSpaceSalt()
		if err != nil {
			return nil, err
		}
		s.spaceSalt = salt
		return s.spaceSalt, nil
	}
	return s.spaceSalt, nil
}

func (s *service) observeChanges(decryptor keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue) {
	for _, kv := range kvs {
		value, err := decodeKeyValue(decryptor, kv)
		if err != nil {
			log.Warn("decode key-value", zap.Error(err))
			continue
		}

		// s.subscriptionBuf is safe to use without a lock because observeChanges runs only in one goroutine, and this buffer
		// isn't used anywhere else
		s.subscriptionBuf = s.subscriptionBuf[:0]

		s.lock.RLock()
		byKey := s.subscriptions[value.Key]
		for _, sub := range byKey {
			s.subscriptionBuf = append(s.subscriptionBuf, sub)
		}
		s.lock.RUnlock()

		for _, sub := range s.subscriptionBuf {
			sub.observerFunc(value.Key, value)
		}
	}
}

func decodeKeyValue(decryptor keyvaluestorage.Decryptor, kv innerstorage.KeyValue) (Value, error) {
	data, err := decryptor(kv)
	if err != nil {
		return Value{}, fmt.Errorf("decrypt value: %w", err)
	}

	clientKey, value, err := decodeKeyValuePair(data)
	if err != nil {
		return Value{}, fmt.Errorf("decode key-value pair: %w", err)
	}
	return Value{
		Key:            clientKey,
		Data:           value,
		TimestampMilli: kv.TimestampMilli,
	}, nil
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
			value, err := decodeKeyValue(decryptor, kv)
			if err != nil {
				return fmt.Errorf("decode key-value pair: %w", err)
			}
			result = append(result, value)
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

	// Encode value as key + value, so we can use hashing for keys and still able to retrieve original key from client code
	encoded, err := encodeKeyValuePair(key, value)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}

	return s.keyValueStore.Set(ctx, string(derived), encoded)
}

func (s *service) getDerivedKey(key string) (derivedKey, error) {
	s.keysLock.Lock()
	defer s.keysLock.Unlock()

	derived, ok := s.keyToDerivedKey[key]
	if ok {
		return derived, nil
	}

	salt, err := s.getSalt()
	if err != nil {
		return derived, fmt.Errorf("get salt: %w", err)
	}
	hasher := sha256.New()
	// Salt
	hasher.Write(salt)
	// User key
	hasher.Write([]byte(key))
	result := hasher.Sum(nil)

	derived = derivedKey(hex.EncodeToString(result))

	s.keyToDerivedKey[key] = derived
	return derived, nil
}

func (s *service) SubscribeForKey(key string, subscriptionName string, observerFunc ObserverFunc) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[key]
	if !ok {
		byKey = make(map[string]subscription)
		s.subscriptions[key] = byKey
	}

	byKey[subscriptionName] = subscription{
		name:         subscriptionName,
		observerFunc: observerFunc,
	}
	return nil
}

func (s *service) UnsubscribeFromKey(key string, subscriptionName string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	byKey, ok := s.subscriptions[key]
	if ok {
		delete(byKey, subscriptionName)
	}
	return nil
}

// use 2 as we use uint16
const sizePrefixLen = 2

func encodeKeyValuePair(key string, value []byte) ([]byte, error) {
	keySize := len(key)
	if keySize > math.MaxUint16 {
		return nil, fmt.Errorf("key is too long: %d", keySize)
	}
	buf := make([]byte, sizePrefixLen+len(key)+len(value))
	binary.BigEndian.PutUint16(buf, uint16(keySize))
	copy(buf[sizePrefixLen:], key)
	copy(buf[sizePrefixLen+len(key):], value)
	return buf, nil
}

func decodeKeyValuePair(raw []byte) (string, []byte, error) {
	if len(raw) < sizePrefixLen {
		return "", nil, fmt.Errorf("raw value is too small: no key size prefix")
	}
	keySize := int(binary.BigEndian.Uint16(raw))
	if len(raw) < sizePrefixLen+keySize {
		return "", nil, fmt.Errorf("raw value is too small: no key")
	}
	key := make([]byte, keySize)
	copy(key, raw[sizePrefixLen:sizePrefixLen+keySize])
	value := raw[sizePrefixLen+keySize:]
	return string(key), value, nil
}
