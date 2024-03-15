package filesync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var (
	sepByte = []byte("/")[0]

	uploadKeyPrefix    = []byte(keyPrefix + "queue/upload/")
	removeKeyPrefix    = []byte(keyPrefix + "queue/remove/")
	discardedKeyPrefix = []byte(keyPrefix + "queue/discarded/")
)

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

type queue struct {
	db           *badger.DB
	keyFunc      func(id domain.FullFileId) []byte
	badgerPrefix []byte
	batcher      *mb.MB[*QueueItem]

	lock sync.Mutex
	set  map[domain.FullFileId]struct{}
}

func newQueue(db *badger.DB, badgerPrefix []byte, keyFunc func(id domain.FullFileId) []byte) (*queue, error) {
	q := &queue{
		db:           db,
		badgerPrefix: badgerPrefix,
		batcher:      mb.New[*QueueItem](0),
		set:          make(map[domain.FullFileId]struct{}),
		keyFunc:      keyFunc,
	}
	err := q.restore()
	if err != nil {
		return nil, fmt.Errorf("restore queue: %w", err)
	}
	return q, nil
}

func (q *queue) close() error {
	return q.batcher.Close()
}

func (q *queue) restore() error {
	items, err := q.listItemsByPrefix()
	if err != nil {
		return fmt.Errorf("get saved discarded items: %w", err)
	}
	err = q.batcher.Add(context.Background(), items...)
	if err != nil {
		return fmt.Errorf("add to discarded queue: %w", err)
	}
	for _, it := range items {
		q.set[it.FullFileId()] = struct{}{}
	}
	return nil
}

func (q *queue) listItemsByPrefix() ([]*QueueItem, error) {
	var items []*QueueItem
	err := q.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         q.badgerPrefix,
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			qItem, err := getQueueItem(item)
			if err != nil {
				return fmt.Errorf("get queue item %s: %w", item.Key(), err)
			}
			fileId, spaceId := extractFileAndSpaceID(item)
			qItem.FileId = domain.FileId(fileId)
			qItem.SpaceId = spaceId
			items = append(items, qItem)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].less(items[j])
	})
	return items, nil
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

func extractFileAndSpaceID(item *badger.Item) (string, string) {
	k := item.Key()
	idx := bytes.LastIndexByte(k, sepByte)
	fileId := string(k[idx+1:])
	k = k[:idx]
	idx = bytes.LastIndexByte(k, sepByte)
	spaceId := string(k[idx+1:])
	return fileId, spaceId
}

func (q *queue) has(id domain.FullFileId) bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	_, ok := q.set[id]
	return ok
}

func (q *queue) add(ctx context.Context, item *QueueItem) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.set[item.FullFileId()] = struct{}{}
	err := q.batcher.Add(ctx, item)
	if err != nil {
		return err
	}
	return q.store(item)
}

func (q *queue) store(item *QueueItem) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("create queue item: %w", err)
	}
	return badgerhelper.SetValue(q.db, q.keyFunc(item.FullFileId()), raw)
}

func (q *queue) getNext(ctx context.Context) (*QueueItem, error) {
	it, err := q.batcher.WaitOne(ctx)
	if err != nil {
		return nil, err
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	_, ok := q.set[it.FullFileId()]
	if !ok {
		return nil, fmt.Errorf("removed from queue")
	}
	delete(q.set, it.FullFileId())
	return it, nil
}

func (q *queue) remove(id domain.FullFileId) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	delete(q.set, id)
	return badgerhelper.DeleteValue(q.db, q.keyFunc(id))
}

func (q *queue) length() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	return len(q.set)
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
