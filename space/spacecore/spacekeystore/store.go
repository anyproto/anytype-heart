package spacekeystore

import (
	"bytes"
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
)

type Store interface {
	SyncKeysFromAclState(spaceID string, aclRecordID string, firstMetadataKey crypto.PrivKey, readKey crypto.SymKey)
	EncryptionKeyBySpaceId(spaceId string) (crypto.PrivKey, error)
	KeyBySpaceId(spaceId string) (string, error)
	EncryptionKeyByKeyId(keyId string) (crypto.PrivKey, error)
	app.Component
}

type SpaceKeyStore struct {
	spaceIdToKey            map[string]string
	spaceKeyToAclRecordId   map[string]string
	spaceKeyToEncryptionKey map[string]crypto.PrivKey
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
		spaceIdToKey:            make(map[string]string),
		spaceKeyToEncryptionKey: make(map[string]crypto.PrivKey),
		spaceKeyToAclRecordId:   make(map[string]string),
	}
}

func (s *SpaceKeyStore) SyncKeysFromAclState(spaceID string, aclRecordID string, firstMetadataKey crypto.PrivKey, readKey crypto.SymKey) {
	s.Lock()
	defer s.Unlock()

	keyID, exists := s.spaceIdToKey[spaceID]
	if !exists {
		var err error
		keyID, err = s.deriveAndStoreKeyId(spaceID, firstMetadataKey)
		if err != nil {
			log.Errorf("Failed to derive and store key ID for space %s: %v", spaceID, err)
			return
		}
	}

	if storedAclID, ok := s.spaceKeyToAclRecordId[keyID]; ok && storedAclID == aclRecordID {
		return
	}

	privKey, err := s.deriveEncryptionKey(readKey)
	if err != nil {
		log.Errorf("Failed to derive encryption key for space %s: %v", spaceID, err)
		return
	}

	s.spaceKeyToEncryptionKey[keyID] = privKey
	s.spaceKeyToAclRecordId[keyID] = aclRecordID

	if err := s.broadcastKeyUpdate(spaceID, keyID, aclRecordID, privKey); err != nil {
		log.Errorf("Failed to broadcast key update for space %s: %v", spaceID, err)
	}
}

func (s *SpaceKeyStore) deriveAndStoreKeyId(spaceID string, firstMetadataKey crypto.PrivKey) (string, error) {
	rawKey, err := s.deriveKey(firstMetadataKey)
	if err != nil {
		return "", err
	}
	encodedKey, err := strkey.Encode(spaceVersion, rawKey)
	if err != nil {
		return "", err
	}
	s.spaceIdToKey[spaceID] = encodedKey
	return encodedKey, nil
}

func (s *SpaceKeyStore) deriveKey(firstMetadataKey crypto.PrivKey) ([]byte, error) {
	pk, err := privkey.DeriveFromPrivKey(spaceKeyPath, firstMetadataKey)
	if err != nil {
		return nil, err
	}
	raw, err := pk.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *SpaceKeyStore) deriveEncryptionKey(readKey crypto.SymKey) (crypto.PrivKey, error) {
	seed, err := readKey.Raw()
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(seed)
	privKey, _, err := crypto.GenerateEd25519Key(reader)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

func (s *SpaceKeyStore) broadcastKeyUpdate(spaceID, keyID, aclRecordID string, privKey crypto.PrivKey) error {
	rawKey, err := privKey.Raw()
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

func (s *SpaceKeyStore) EncryptionKeyBySpaceId(spaceId string) (crypto.PrivKey, error) {
	s.Lock()
	defer s.Unlock()
	keyId, exists := s.spaceIdToKey[spaceId]
	if !exists {
		return nil, ErrNotFound
	}
	key, exists := s.spaceKeyToEncryptionKey[keyId]
	if !exists {
		return nil, ErrNotFound
	}
	return key, nil
}

func (s *SpaceKeyStore) EncryptionKeyByKeyId(keyId string) (crypto.PrivKey, error) {
	s.Lock()
	defer s.Unlock()
	key, exists := s.spaceKeyToEncryptionKey[keyId]
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
