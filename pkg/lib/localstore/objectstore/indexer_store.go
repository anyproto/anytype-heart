package objectstore

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (m *dsObjectStore) AddToIndexQueue(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	var buf [8]byte
	size := binary.PutVarint(buf[:], time.Now().Unix())
	if err = txn.Put(indexQueueBase.ChildString(id), buf[:size]); err != nil {
		return err
	}
	return txn.Commit()
}

func (m *dsObjectStore) removeFromIndexQueue(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Delete(indexQueueBase.ChildString(id)); err != nil {
		return fmt.Errorf("failed to remove id from full text index queue: %s", err.Error())
	}

	return txn.Commit()
}

func (m *dsObjectStore) ListIDsFromFullTextQueue() ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	res, err := txn.Query(query.Query{Prefix: indexQueueBase.String()})
	if err != nil {
		return nil, fmt.Errorf("error query txn in datastore: %w", err)
	}

	var ids []string
	for entry := range res.Next() {
		ids = append(ids, extractIdFromKey(entry.Key))
	}

	err = res.Close()
	if err != nil {
		return nil, fmt.Errorf("close query result: %w", err)
	}
	return ids, nil
}

func (m *dsObjectStore) RemoveIDsFromFullTextQueue(ids []string) {
	for _, id := range ids {
		err := m.removeFromIndexQueue(id)
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s from index, will redo the fulltext index: %v", id, err)
		}
	}
}

func (m *dsObjectStore) GetChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	val, err := txn.Get(bundledChecksums)
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get details: %w", err)
	}
	if err == ds.ErrNotFound {
		return nil, err
	}

	var objChecksum model.ObjectStoreChecksums
	if err := proto.Unmarshal(val, &objChecksum); err != nil {
		return nil, err
	}

	return &objChecksum, nil
}

func (m *dsObjectStore) SaveChecksums(checksums *model.ObjectStoreChecksums) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	b, err := checksums.Marshal()
	if err != nil {
		return err
	}

	if err := txn.Put(bundledChecksums, b); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (m *dsObjectStore) GetLastIndexedHeadsHash(id string) (headsHash string, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return "", fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if val, err := txn.Get(indexedHeadsState.ChildString(id)); err != nil && err != ds.ErrNotFound {
		return "", fmt.Errorf("failed to get heads hash: %w", err)
	} else if val == nil {
		return "", nil
	} else {
		return string(val), nil
	}
}

func (m *dsObjectStore) SaveLastIndexedHeadsHash(id string, headsHash string) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Put(indexedHeadsState.ChildString(id), []byte(headsHash)); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}
