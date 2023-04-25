package filesync

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v3"
)

const (
	keyPrefix = "/filesyncindex/"
)

var (
	errQueueIsEmpty = errors.New("queue is empty")

	sepByte = []byte("/")[0]

	uploadKeyPrefix    = []byte(keyPrefix + "queue/upload/")
	removeKeyPrefix    = []byte(keyPrefix + "queue/remove/")
	discardedKeyPrefix = []byte(keyPrefix + "queue/discarded/")
)

type fileSyncStore struct {
	db *badger.DB
}

func (s *fileSyncStore) QueueUpload(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(uploadKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) QueueDiscarded(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(uploadKey(spaceId, fileId)); err != nil {
			return err
		}
		return txn.Set(discardedKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) QueueRemove(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(removeKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) DoneUpload(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(uploadKey(spaceId, fileId)); err != nil {
			return err
		}
		if err = txn.Delete(discardedKey(spaceId, fileId)); err != nil {
			return err
		}
		return txn.Set(doneKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) DoneDiscarded(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(discardedKey(spaceId, fileId)); err != nil {
			return err
		}
		return txn.Set(doneKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) DoneRemove(spaceId, fileId string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Delete(removeKey(spaceId, fileId)); err != nil {
			return err
		}
		return txn.Set(doneKey(spaceId, fileId), binTime())
	})
}

func (s *fileSyncStore) GetUpload() (spaceId, fileId string, err error) {
	return s.getOne(uploadKeyPrefix)
}

func (s *fileSyncStore) GetDiscardedUpload() (spaceId, fileId string, err error) {
	return s.getOne(discardedKeyPrefix)
}

func (s *fileSyncStore) HasUpload(spaceId, fileId string) (ok bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(uploadKey(spaceId, fileId))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		ok = true
		return nil
	})
	return
}

func (s *fileSyncStore) GetRemove() (spaceId, fileId string, err error) {
	return s.getOne(removeKeyPrefix)
}

func (s *fileSyncStore) getOne(prefix []byte) (spaceId, fileId string, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         prefix,
		})
		defer it.Close()

		var earliest uint64
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			timestamp, err := getTimestamp(item)
			{
				fid, sid := extractFileAndSpaceID(item)
				fmt.Printf("fileId: %s, spaceId: %s, timestamp: %d\n", fid, sid, timestamp)
			}
			if err != nil {
				return fmt.Errorf("get timestamp: %w", err)
			}
			if earliest == 0 || timestamp < earliest {
				earliest = timestamp
				fileId, spaceId = extractFileAndSpaceID(item)
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	if fileId == "" {
		return "", "", errQueueIsEmpty
	}
	return
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

func (s *fileSyncStore) IsDone(spaceId, fileId string) (done bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		_, e := txn.Get(doneKey(spaceId, fileId))
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

func doneKey(spaceId, fileId string) (key []byte) {
	return []byte(keyPrefix + "done/" + spaceId + "/" + fileId)
}

func binTime() []byte {
	return binary.LittleEndian.AppendUint64(nil, uint64(time.Now().UnixMilli()))
}

func getTimestamp(item *badger.Item) (uint64, error) {
	var ts uint64
	err := item.Value(func(raw []byte) error {
		ts = binary.LittleEndian.Uint64(raw)
		return nil
	})
	return ts, err
}
