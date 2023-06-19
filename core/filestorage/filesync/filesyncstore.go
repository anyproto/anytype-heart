package filesync

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"
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
}

func newFileSyncStore(db *badger.DB) (*fileSyncStore, error) {
	s := &fileSyncStore{
		db: db,
	}
	err := s.migrateQueue()
	if err != nil {
		return nil, fmt.Errorf("migrate queue: %w", err)
	}
	return s, nil
}

type QueueItem struct {
	SpaceID     string
	FileID      string
	Timestamp   int64
	AddedByUser bool
}

func (it *QueueItem) less(other *QueueItem) bool {
	return it.Timestamp < other.Timestamp
}

const queueSchemaVersion = 1

func (s *fileSyncStore) migrateQueue() error {
	return s.db.Update(func(txn *badger.Txn) error {
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

func (s *fileSyncStore) QueueUpload(spaceId string, fileId string, addedByUser bool) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		logger := log.With(zap.String("fileID", fileId), zap.String("addedByUser", strconv.FormatBool(addedByUser)))
		ok, err := isKeyExists(txn, discardedKey(spaceId, fileId))
		if err != nil {
			return fmt.Errorf("check discarded key: %w", err)
		}
		if ok {
			logger.Info("add file to upload queue: file is in discarded queue")
			return nil
		}
		ok, err = isKeyExists(txn, uploadKey(spaceId, fileId))
		if err != nil {
			return fmt.Errorf("check upload key: %w", err)
		}
		if ok {
			logger.Info("add file to upload queue: file is already in queue, update timestamp")
		} else {
			logger.Info("add file to upload queue")
		}
		raw, err := createQueueItem(addedByUser)
		if err != nil {
			return fmt.Errorf("create queue item: %w", err)
		}
		return txn.Set(uploadKey(spaceId, fileId), raw)
	})
}

func createQueueItem(addedByUser bool) ([]byte, error) {
	return json.Marshal(QueueItem{
		Timestamp:   time.Now().UnixMilli(),
		AddedByUser: addedByUser,
	})
}

func (s *fileSyncStore) QueueDiscarded(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(uploadKey(spaceId, fileId)); err != nil {
			return err
		}
		raw, err := createQueueItem(false)
		if err != nil {
			return fmt.Errorf("create queue item: %w", err)
		}
		return txn.Set(discardedKey(spaceId, fileId), raw)
	})
}

func (s *fileSyncStore) QueueRemove(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = removeFromUploadingQueue(txn, spaceId, fileId); err != nil {
			return err
		}
		raw, err := createQueueItem(false)
		if err != nil {
			return fmt.Errorf("create queue item: %w", err)
		}
		return txn.Set(removeKey(spaceId, fileId), raw)
	})
}

func (s *fileSyncStore) DoneUpload(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = removeFromUploadingQueue(txn, spaceId, fileId); err != nil {
			return err
		}
		return txn.Set(doneUploadKey(spaceId, fileId), binTime(time.Now().UnixMilli()))
	})
}

func removeFromUploadingQueue(txn *badger.Txn, spaceID string, fileID string) error {
	if err := txn.Delete(uploadKey(spaceID, fileID)); err != nil {
		return fmt.Errorf("remove from uploading queue: %w", err)
	}
	if err := txn.Delete(discardedKey(spaceID, fileID)); err != nil {
		return fmt.Errorf("remove from discarded uploading queue: %w", err)
	}
	return nil
}

func (s *fileSyncStore) DoneRemove(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(removeKey(spaceId, fileId)); err != nil {
			return err
		}
		if err = txn.Delete(doneUploadKey(spaceId, fileId)); err != nil {
			return err
		}
		return txn.Set(doneRemoveKey(spaceId, fileId), binTime(time.Now().UnixMilli()))
	})
}

func (s *fileSyncStore) GetUpload() (it *QueueItem, err error) {
	return s.getOne(uploadKeyPrefix)
}

func (s *fileSyncStore) GetDiscardedUpload() (it *QueueItem, err error) {
	return s.getOne(discardedKeyPrefix)
}

func isKeyExists(txn *badger.Txn, key []byte) (bool, error) {
	_, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *fileSyncStore) isFileQueued(spaceId, fileId string) (ok bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		ok, err = isKeyExists(txn, uploadKey(spaceId, fileId))
		if err != nil {
			return fmt.Errorf("check upload key: %w", err)
		}
		if ok {
			return nil
		}

		ok, err = isKeyExists(txn, discardedKey(spaceId, fileId))
		if err != nil {
			return fmt.Errorf("check discarded key: %w", err)
		}
		return nil
	})
	return
}

func (s *fileSyncStore) HasUpload(spaceId, fileId string) (ok bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		ok, err = isKeyExists(txn, uploadKey(spaceId, fileId))
		return err
	})
	return
}

func (s *fileSyncStore) IsFileUploadLimited(spaceId, fileId string) (ok bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		ok, err = isKeyExists(txn, discardedKey(spaceId, fileId))
		return err
	})
	return
}

func (s *fileSyncStore) GetRemove() (it *QueueItem, err error) {
	return s.getOne(removeKeyPrefix)
}

// getOne returns the oldest key from the queue with given prefix
func (s *fileSyncStore) getOne(prefix []byte) (earliest *QueueItem, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         prefix,
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			qItem, err := getQueueItem(item)
			if err != nil {
				return fmt.Errorf("get queue item %s: %w", item.Key(), err)
			}
			if earliest == nil || qItem.less(earliest) {
				earliest = qItem
				fileId, spaceId := extractFileAndSpaceID(item)
				earliest.FileID = fileId
				earliest.SpaceID = spaceId
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	if earliest == nil {
		return nil, errQueueIsEmpty
	}
	return
}

func (s *fileSyncStore) listItemsByPrefix(prefix []byte) ([]*QueueItem, error) {
	var items []*QueueItem
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         prefix,
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			qItem, err := getQueueItem(item)
			if err != nil {
				return fmt.Errorf("get queue item %s: %w", item.Key(), err)
			}
			fileId, spaceId := extractFileAndSpaceID(item)
			qItem.FileID = fileId
			qItem.SpaceID = spaceId
			items = append(items, qItem)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func extractFileAndSpaceID(item *badger.Item) (string, string) {
	k := item.Key()
	idx := bytes.LastIndexByte(k, sepByte)
	fileId := string(k[idx+1:])
	k = k[:idx]
	idx = bytes.LastIndexByte(k, sepByte)
	spaceId := string(k[idx+1:])
	return fileId, spaceId
}

func (s *fileSyncStore) QueueLen() (l int, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		for _, prefix := range [][]byte{uploadKeyPrefix, removeKeyPrefix} {
			it := txn.NewIterator(badger.IteratorOptions{
				PrefetchSize:   100,
				PrefetchValues: false,
				Prefix:         prefix,
			})
			for it.Rewind(); it.Valid(); it.Next() {
				l++
			}
			it.Close()
		}
		return nil
	})
	return
}

func (s *fileSyncStore) IsAlreadyUploaded(spaceId, fileId string) (done bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		_, e := txn.Get(doneUploadKey(spaceId, fileId))
		if e != nil && e != badger.ErrKeyNotFound {
			return e
		}
		if e != badger.ErrKeyNotFound {
			done = true
		}
		return nil
	})
	return
}

func uploadKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "queue/upload/" + spaceId + "/" + fileId)
}

func discardedKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "queue/discarded/" + spaceId + "/" + fileId)
}

func removeKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "queue/remove/" + spaceId + "/" + fileId)
}

func doneUploadKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "done/upload/" + spaceId + "/" + fileId)
}

func doneRemoveKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "done/remove/" + spaceId + "/" + fileId)
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
