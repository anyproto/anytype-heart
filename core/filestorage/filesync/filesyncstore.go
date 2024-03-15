package filesync

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/cheggaaa/mb/v3"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const (
	keyPrefix = "/filesyncindex/"
)

var (
	errQueueIsEmpty = errors.New("queue is empty")

	sepByte = []byte("/")[0]

	uploadKeyPrefix       = []byte(keyPrefix + "queue/upload/")
	removeKeyPrefix       = []byte(keyPrefix + "queue/remove/")
	discardedKeyPrefix    = []byte(keyPrefix + "queue/discarded/")
	queueSchemaVersionKey = []byte(keyPrefix + "queue/schema_version")
)

type fileSyncStore struct {
	db *badger.DB

	inboxQueue     *mb.MB[*QueueItem]
	discardedQueue *mb.MB[*QueueItem]
	removingQueue  *mb.MB[*QueueItem]
}

func newFileSyncStore(db *badger.DB) (*fileSyncStore, error) {
	s := &fileSyncStore{
		db:             db,
		inboxQueue:     mb.New[*QueueItem](0),
		discardedQueue: mb.New[*QueueItem](0),
		removingQueue:  mb.New[*QueueItem](0),
	}
	err := s.migrateQueue()
	if err != nil {
		return nil, fmt.Errorf("migrate queue: %w", err)
	}

	return s, nil
}

type QueueItem struct {
	SpaceId     string
	FileId      domain.FileId
	Timestamp   int64
	AddedByUser bool
	Imported    bool
}

func (it *QueueItem) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
}

func (it *QueueItem) less(other *QueueItem) bool {
	return it.Timestamp < other.Timestamp
}

const queueSchemaVersion = 1

func (s *fileSyncStore) updateTxn(f func(txn *badger.Txn) error) error {
	return badgerhelper.RetryOnConflict(func() error {
		return s.db.Update(f)
	})
}

func (s *fileSyncStore) migrateQueue() error {
	return s.updateTxn(func(txn *badger.Txn) error {
		raw, err := txn.Get(queueSchemaVersionKey)
		if err != nil && err != badger.ErrKeyNotFound {
			return fmt.Errorf("get schema version: %w", err)
		}
		version, err := versionFromItem(raw)
		if err != nil {
			return fmt.Errorf("get schema version from item: %w", err)
		}

		if version < queueSchemaVersion {
			err = runMigrationFromVersion0(txn)
			if err != nil {
				return fmt.Errorf("run migration from version 0: %w", err)
			}
		}

		return txn.Set(queueSchemaVersionKey, []byte(strconv.Itoa(queueSchemaVersion)))
	})
}

func runMigrationFromVersion0(txn *badger.Txn) error {
	for _, prefix := range [][]byte{
		uploadKeyPrefix,
		discardedKeyPrefix,
		removeKeyPrefix,
	} {
		err := migrateByPrefix(txn, prefix)
		if err != nil {
			return fmt.Errorf("migrate by prefix %s: %w", string(prefix), err)
		}
	}
	return nil
}

func migrateByPrefix(txn *badger.Txn, prefix []byte) error {
	it := txn.NewIterator(badger.IteratorOptions{
		PrefetchSize:   100,
		PrefetchValues: true,
		Prefix:         prefix,
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		err := migrateItem(txn, item)
		if err != nil {
			return fmt.Errorf("migrate item %s: %w", item.Key(), err)
		}
		log.Warn("migrate queue item", zap.String("key", string(item.Key())))
	}
	return nil
}

func migrateItem(txn *badger.Txn, item *badger.Item) error {
	timestamp, err := getTimestamp(item)
	if err != nil {
		return fmt.Errorf("get timestamp: %w", err)
	}
	it := QueueItem{
		Timestamp: int64(timestamp),
	}
	raw, err := json.Marshal(it)
	if err != nil {
		return fmt.Errorf("marshal queue item: %w", err)
	}
	return txn.Set(item.Key(), raw)
}

func versionFromItem(it *badger.Item) (int, error) {
	if it == nil {
		return 0, nil
	}
	var res int
	err := it.Value(func(raw []byte) error {
		res, _ = strconv.Atoi(string(raw))
		return nil
	})
	return res, err
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

func uploadKey(fileId domain.FullFileId) (key []byte) {
	return []byte(keyPrefix + "queue/upload/" + fileId.SpaceId + "/" + fileId.FileId.String())
}

func discardedKey(fileId domain.FullFileId) (key []byte) {
	return []byte(keyPrefix + "queue/discarded/" + fileId.SpaceId + "/" + fileId.FileId.String())
}

func removeKey(fileId domain.FullFileId) (key []byte) {
	return []byte(keyPrefix + "queue/remove/" + fileId.SpaceId + "/" + fileId.FileId.String())
}

func binTime(timestamp int64) []byte {
	return binary.LittleEndian.AppendUint64(nil, uint64(timestamp))
}

func getTimestamp(item *badger.Item) (uint64, error) {
	var ts uint64
	err := item.Value(func(raw []byte) error {
		ts = binary.LittleEndian.Uint64(raw)
		return nil
	})
	return ts, err
}

func getQueueItem(item *badger.Item) (*QueueItem, error) {
	var it QueueItem
	err := item.Value(func(raw []byte) error {
		return json.Unmarshal(raw, &it)
	})
	if err != nil {
		return nil, err
	}
	return &it, nil
}
