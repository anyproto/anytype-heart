package objectstore

import (
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

func (s *dsObjectStore) AddToIndexQueue(id string) error {
	return badgerhelper.SetValue(s.db, indexQueueBase.ChildString(id).Bytes(), nil)
}

func (s *dsObjectStore) removeFromIndexQueue(id string) error {
	return badgerhelper.DeleteValue(s.db, indexQueueBase.ChildString(id).Bytes())
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

func (s *dsObjectStore) GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error) {
	return badgerhelper.GetValue(s.db, bundledChecksums.ChildString(spaceID).Bytes(), func(raw []byte) (*model.ObjectStoreChecksums, error) {
		checksums := &model.ObjectStoreChecksums{}
		return checksums, proto.Unmarshal(raw, checksums)
	})
}

func (s *dsObjectStore) SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error) {
	// in case we have global checksums we need to remove them, because it should not be used for any new space
	if spaceID != addr.AnytypeMarketplaceWorkspace {
		_ = badgerhelper.DeleteValue(s.db, bundledChecksums.Bytes())
	}
	return badgerhelper.SetValue(s.db, bundledChecksums.ChildString(spaceID).Bytes(), checksums)
}

// GetGlobalChecksums is a migration method, it returns checksums stored before we started to store them per space
// it will be deleted after the first SaveChecksums() call
func (s *dsObjectStore) GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	return badgerhelper.GetValue(s.db, bundledChecksums.Bytes(), func(raw []byte) (*model.ObjectStoreChecksums, error) {
		checksums := &model.ObjectStoreChecksums{}
		return checksums, proto.Unmarshal(raw, checksums)
	})
}

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (s *dsObjectStore) GetLastIndexedHeadsHash(id string) (headsHash string, err error) {
	headsHash, err = badgerhelper.GetValue(s.db, indexedHeadsState.ChildString(id).Bytes(), bytesToString)
	if err != nil && !badgerhelper.IsNotFound(err) {
		return "", err
	}
	return headsHash, nil
}

func (s *dsObjectStore) SaveLastIndexedHeadsHash(id string, headsHash string) (err error) {
	return badgerhelper.SetValue(s.db, indexedHeadsState.ChildString(id).Bytes(), headsHash)
}
