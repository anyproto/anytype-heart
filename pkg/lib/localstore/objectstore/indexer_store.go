package objectstore

import (
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) AddToIndexQueue(id string) error {
	if id == "index" {
		fmt.Println()
	}
	return setValue(s.db, indexQueueBase.ChildString(id).Bytes(), nil)
}

func (s *dsObjectStore) removeFromIndexQueue(id string) error {
	return deleteValue(s.db, indexQueueBase.ChildString(id).Bytes())
}

func (s *dsObjectStore) ListIDsFromFullTextQueue() ([]string, error) {
	var ids []string
	err := iterateKeysByPrefix(s.db, indexQueueBase.Bytes(), func(key []byte) {
		ids = append(ids, extractIDFromKey(string(key)))
	})
	return ids, err
}

func (s *dsObjectStore) RemoveIDsFromFullTextQueue(ids []string) {
	for _, id := range ids {
		err := s.removeFromIndexQueue(id)
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s from index, will redo the fulltext index: %v", id, err)
		}
	}
}

func (s *dsObjectStore) GetChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	return getValue(s.db, bundledChecksums.Bytes(), func(raw []byte) (*model.ObjectStoreChecksums, error) {
		checksums := &model.ObjectStoreChecksums{}
		return checksums, proto.Unmarshal(raw, checksums)
	})
}

func (s *dsObjectStore) SaveChecksums(checksums *model.ObjectStoreChecksums) (err error) {
	return setValue(s.db, bundledChecksums.Bytes(), checksums)
}

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (s *dsObjectStore) GetLastIndexedHeadsHash(id string) (headsHash string, err error) {
	headsHash, err = getValue(s.db, indexedHeadsState.ChildString(id).Bytes(), bytesToString)
	if err != nil && !isNotFound(err) {
		return "", err
	}
	return headsHash, nil
}

func (s *dsObjectStore) SaveLastIndexedHeadsHash(id string, headsHash string) (err error) {
	return setValue(s.db, indexedHeadsState.ChildString(id).Bytes(), headsHash)
}
