package spacekeystore

import (
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/strkey"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/privkey"
)

var log = logging.Logger(CName)

var ErrNotFound = errors.New("key not found")

const (
	CName        = "space.core.spaceKeyStore"
	spaceKeyPath = "m/99999'/1'"
	spaceVersion = 0xB5
	spacePath    = "m/99999'/1'"
)

type Store interface {
	SyncKeysFromAclState(spaceID string, aclRecordID string, firstMetadataKey crypto.PrivKey, readKey crypto.SymKey)
	SignKeyBySpaceId(spaceId string) (crypto.PrivKey, error)
	KeyBySpaceId(spaceId string) (string, error)
	SignKeyByKeyId(keyId string) (crypto.PrivKey, error)
	EncryptionKeyBySpaceId(spaceId string) (crypto.SymKey, error)
	app.Component
}

type SpaceKeyStore struct {
	spaceIdToKey            map[string]string
	spaceKeyToAclRecordId   map[string]string
	spaceKeyToSignatureKey  map[string]crypto.PrivKey
	spaceKeyToEncryptionKey map[string]crypto.SymKey
	sync.Mutex

	eventSender event.Sender
}

func (s *SpaceKeyStore) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	return nil
}

func (s *SpaceKeyStore) Name() (name string) {
	return CName
}

func New() Store {
	return &SpaceKeyStore{
		spaceKeyToSignatureKey:  make(map[string]crypto.PrivKey),
		spaceKeyToEncryptionKey: make(map[string]crypto.SymKey),
		spaceKeyToAclRecordId:   make(map[string]string),
		spaceIdToKey:            make(map[string]string),
	}
}

func (s *SpaceKeyStore) SyncKeysFromAclState(spaceID, aclRecordID string, firstMetadataKey crypto.PrivKey, readKey crypto.SymKey) {
	s.Lock()
	defer s.Unlock()

	keyID, exists := s.spaceIdToKey[spaceID]
	if !exists {
		var err error
		keyID, err = s.deriveAndStoreSpaceKey(spaceID, firstMetadataKey)
		if err != nil {
			log.Errorf("Failed to derive and store key ID for space %s: %v", spaceID, err)
			return
		}
	}

	if storedAclID, ok := s.spaceKeyToAclRecordId[keyID]; ok && storedAclID == aclRecordID {
		return
	}

	symKey, err := s.deriveEncryptionKey(readKey)
	if err != nil {
		log.Errorf("Failed to derive encryption key for space %s: %v", spaceID, err)
		return
	}
	s.spaceKeyToAclRecordId[keyID] = aclRecordID
	s.spaceKeyToEncryptionKey[keyID] = symKey
	if err := s.broadcastKeyUpdate(spaceID, keyID, aclRecordID, symKey); err != nil {
		log.Errorf("Failed to broadcast key update for space %s: %v", spaceID, err)
	}
}

func (s *SpaceKeyStore) deriveAndStoreSpaceKey(spaceID string, firstMetadataKey crypto.PrivKey) (string, error) {
	key, err := privkey.DeriveFromPrivKey(spaceKeyPath, firstMetadataKey)
	if err != nil {
		return "", err
	}
	rawKey, err := key.GetPublic().Raw()
	if err != nil {
		return "", err
	}
	encodedKey, err := strkey.Encode(spaceVersion, rawKey)
	if err != nil {
		return "", err
	}
	s.spaceIdToKey[spaceID] = encodedKey
	s.spaceKeyToSignatureKey[encodedKey] = key
	return encodedKey, nil
}

func (s *SpaceKeyStore) deriveEncryptionKey(readKey crypto.SymKey) (crypto.SymKey, error) {
	raw, err := readKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, spacePath)
}

func (s *SpaceKeyStore) broadcastKeyUpdate(spaceID, keyID, aclRecordID string, symKey crypto.SymKey) error {
	rawKey, err := symKey.Raw()
	if err != nil {
		return err
	}
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				SpaceId: spaceID,
				Value: &pb.EventMessageValueOfKeyUpdate{KeyUpdate: &pb.EventKeyUpdate{
					SpaceKeyId:      keyID,
					EncryptionKeyId: aclRecordID,
					EncryptionKey:   rawKey,
				}},
			},
		},
	})
	return nil
}

func (s *SpaceKeyStore) SignKeyBySpaceId(spaceId string) (crypto.PrivKey, error) {
	s.Lock()
	defer s.Unlock()
	keyId, exists := s.spaceIdToKey[spaceId]
	if !exists {
		return nil, ErrNotFound
	}
	key, exists := s.spaceKeyToSignatureKey[keyId]
	if !exists {
		return nil, ErrNotFound
	}
	return key, nil
}

func (s *SpaceKeyStore) EncryptionKeyBySpaceId(spaceId string) (crypto.SymKey, error) {
	s.Lock()
	defer s.Unlock()
	key, exists := s.spaceKeyToEncryptionKey[spaceId]
	if !exists {
		return nil, ErrNotFound
	}
	return key, nil
}

func (s *SpaceKeyStore) SignKeyByKeyId(keyId string) (crypto.PrivKey, error) {
	s.Lock()
	defer s.Unlock()
	key, exists := s.spaceKeyToSignatureKey[keyId]
	if !exists {
		return nil, ErrNotFound
	}
	return key, nil
}

func (s *SpaceKeyStore) KeyBySpaceId(spaceId string) (string, error) {
	s.Lock()
	defer s.Unlock()
	keyId, exists := s.spaceIdToKey[spaceId]
	if !exists {
		return "", ErrNotFound
	}
	return keyId, nil
}
