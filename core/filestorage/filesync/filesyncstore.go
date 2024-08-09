package filesync

import (
	"encoding/json"

	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const (
	keyPrefix = "/filesync/"
)

type fileSyncStore struct {
	db *badger.DB
}

func newFileSyncStore(db *badger.DB) (*fileSyncStore, error) {
	s := &fileSyncStore{
		db: db,
	}
	return s, nil
}

func (s *fileSyncStore) setNodeUsage(usage NodeUsage) error {
	data, err := json.Marshal(usage)
	if err != nil {
		return err
	}
	return badgerhelper.SetValue(s.db, nodeUsageKey(), data)
}

func (s *fileSyncStore) getNodeUsage() (NodeUsage, error) {
	return badgerhelper.GetValue(s.db, nodeUsageKey(), func(raw []byte) (NodeUsage, error) {
		var usage NodeUsage
		err := json.Unmarshal(raw, &usage)
		return usage, err
	})
}

func nodeUsageKey() []byte {
	return []byte(keyPrefix + "node_usage/")
}
