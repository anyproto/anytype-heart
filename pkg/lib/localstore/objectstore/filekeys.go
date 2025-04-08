package objectstore

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
)

func keyValueItem(arena *anyenc.Arena, key string, value any) (*anyenc.Value, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	obj := arena.NewObject()
	obj.Set("id", arena.NewString(key))
	obj.Set("value", arena.NewStringBytes(raw))
	return obj, nil
}

func (s *dsObjectStore) AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error {
	txn, err := s.fileKeys.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer txn.Commit()

	for _, fk := range fileKeys {
		err = s.fileKeys.Set(txn.Context(), fk.FileId.String(), fk.EncryptionKeys)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("set: %w", err))
		}
	}
	return err
}

func (s *dsObjectStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	return s.fileKeys.Get(s.componentCtx, fileId.String())
}
