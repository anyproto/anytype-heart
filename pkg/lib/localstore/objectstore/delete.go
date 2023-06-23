package objectstore

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) DeleteDetails(id string) error {
	key := pagesDetailsBase.ChildString(id).Bytes()
	return s.updateTxn(func(txn *badger.Txn) error {
		s.cache.Del(key)

		for _, k := range []ds.Key{
			pagesSnippetBase.ChildString(id),
			pagesDetailsBase.ChildString(id),
		} {
			if err := txn.Delete(k.Bytes()); err != nil {
				return fmt.Errorf("delete key %s: %w", k, err)
			}
		}

		return txn.Delete(key)
	})
}

// DeleteObject removes all details, leaving only id and isDeleted
func (s *dsObjectStore) DeleteObject(id string) error {
	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err := s.UpdateObjectDetails(id, &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():        pbtypes.String(id),
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true), // maybe we can store the date instead?
		},
	})
	if err != nil && !errors.Is(err, ErrDetailsNotChanged) {
		return fmt.Errorf("failed to overwrite details and relations: %w", err)
	}

	return retryOnConflict(func() error {
		txn := s.db.NewTransaction(true)
		defer txn.Discard()

		for _, k := range []ds.Key{
			pagesSnippetBase.ChildString(id),
			indexQueueBase.ChildString(id),
			indexedHeadsState.ChildString(id),
		} {
			if err = txn.Delete(k.Bytes()); err != nil {
				return err
			}
		}

		txn, _, err = s.removeByPrefixInTx(txn, pagesInboundLinksBase.String()+"/"+id+"/")
		if err != nil {
			return err
		}
		txn, _, err = s.removeByPrefixInTx(txn, pagesOutboundLinksBase.String()+"/"+id+"/")
		if err != nil {
			return err
		}
		err = txn.Commit()
		if err != nil {
			return fmt.Errorf("delete object info: %w", err)
		}

		if s.fts != nil {
			err = s.removeFromIndexQueue(id)
			if err != nil {
				log.Errorf("error removing %s from index queue: %s", id, err)
			}
			if err := s.fts.Delete(id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *dsObjectStore) removeByPrefixInTx(txn *badger.Txn, prefix string) (*badger.Txn, int, error) {
	var toDelete [][]byte
	err := iterateKeysByPrefixTx(txn, []byte(prefix), func(key []byte) {
		toDelete = append(toDelete, key)
	})
	if err != nil {
		return txn, 0, fmt.Errorf("iterate keys: %w", err)
	}

	var removed int
	for _, key := range toDelete {
		err = txn.Delete(key)
		if err == badger.ErrTxnTooBig {
			err = txn.Commit()
			if err != nil {
				return txn, removed, fmt.Errorf("commit big transaction: %w", err)
			}
			txn = s.db.NewTransaction(true)
			err = txn.Delete(key)
		}
		if err != nil {
			return txn, removed, fmt.Errorf("delete key %s: %w", key, err)
		}
		removed++
	}
	return txn, removed, nil
}
