package objectstore

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
)

const (
	accountStatusKey = "account_status"
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

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	it, err := keyValueItem(arena, accountStatusKey, status)
	if err != nil {
		return fmt.Errorf("create item: %w", err)
	}
	err = s.system.UpsertOne(s.componentCtx, it)
	return err
}

func (s *dsObjectStore) GetAccountStatus() (*coordinatorproto.SpaceStatusPayload, error) {
	doc, err := s.system.FindId(s.componentCtx, accountStatusKey)
	if err != nil {
		return nil, fmt.Errorf("find account status: %w", err)
	}
	val := doc.Value().GetStringBytes("value")
	var status coordinatorproto.SpaceStatusPayload
	err = json.Unmarshal(val, &status)
	return &status, err
}
