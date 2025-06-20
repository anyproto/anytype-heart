package objectstore

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
)

func fileKeysKey(fileId domain.FileId) string {
	return fmt.Sprintf("fileKeys/%s", fileId)
}

func (s *dsObjectStore) AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	txn, err := s.system.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer txn.Commit()

	for _, fk := range fileKeys {
		it, err := keyValueItem(arena, fileKeysKey(fk.FileId), fk.EncryptionKeys)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("create item: %w", err))
		}
		err = s.system.UpsertOne(txn.Context(), it)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("upsert: %w", err))
		}
	}
	return err
}

func (s *dsObjectStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	doc, err := s.system.FindId(s.componentCtx, fileKeysKey(fileId))
	if err != nil {
		return nil, fmt.Errorf("find file keys: %w", err)
	}
	val := doc.Value().GetStringBytes("value")
	keys := map[string]string{}
	err = json.Unmarshal(val, &keys)
	return keys, err
}
