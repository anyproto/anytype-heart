package objectstore

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) DeleteDetails(ids ...string) error {
	for _, chunk := range lo.Chunk(ids, 100) {
		err := s.updateTxn(func(txn *badger.Txn) error {
			for _, id := range chunk {
				s.cache.Del(pagesDetailsBase.ChildString(id).Bytes())

				for _, k := range []ds.Key{
					pagesDetailsBase.ChildString(id),
					pagesSnippetBase.ChildString(id),
					pagesDetailsBase.ChildString(id),
					indexedHeadsState.ChildString(id),
				} {
					if err := txn.Delete(k.Bytes()); err != nil {
						return fmt.Errorf("delete key %s: %w", k, err)
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
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

	return badgerhelper.RetryOnConflict(func() error {
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

		txn, err = s.eraseLinksForObject(txn, id)
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

func getLastPartOfKey(key []byte) string {
	lastSlashIdx := bytes.LastIndexByte(key, '/')
	if lastSlashIdx == -1 {
		return string(key)
	}
	return string(key[lastSlashIdx+1:])
}

func (s *dsObjectStore) eraseLinksForObject(txn *badger.Txn, from string) (*badger.Txn, error) {
	var toDelete [][]byte
	outboundPrefix := pagesOutboundLinksBase.ChildString(from).Bytes()
	err := iterateKeysByPrefixTx(txn, outboundPrefix, func(key []byte) {
		key = slices.Clone(key)
		to := getLastPartOfKey(key)
		toDelete = append(toDelete, key, inboundLinkKey(from, to).Bytes())
	})
	if err != nil {
		return txn, fmt.Errorf("iterate keys: %w", err)
	}

	for _, key := range toDelete {
		err = txn.Delete(key)
		if err == badger.ErrTxnTooBig {
			err = txn.Commit()
			if err != nil {
				return txn, fmt.Errorf("commit big transaction: %w", err)
			}
			txn = s.db.NewTransaction(true)
			err = txn.Delete(key)
		}
		if err != nil {
			return txn, fmt.Errorf("delete key %s: %w", key, err)
		}
	}
	return txn, nil
}
