package objectstore

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) DeleteDetails(ids ...string) error {
	for _, chunk := range lo.Chunk(ids, 100) {
		err := s.updateTxn(func(txn *badger.Txn) error {
			for _, id := range chunk {
				err := s.objects.DeleteId(s.componentCtx, id)
				if err != nil {
					return fmt.Errorf("delete object %s: %w", id, err)
				}

				for _, key := range []ds.Key{
					indexedHeadsState.ChildString(id),
				} {
					if err := txn.Delete(key.Bytes()); err != nil {
						return fmt.Errorf("delete key %s: %w", key, err)
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
func (s *dsObjectStore) DeleteObject(id domain.FullID) error {
	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err := s.UpdateObjectDetails(id.ObjectID, &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():        pbtypes.String(id.ObjectID),
			bundle.RelationKeySpaceId.String():   pbtypes.String(id.SpaceID),
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true), // maybe we can store the date instead?
		},
	})
	if err != nil {
		return fmt.Errorf("failed to overwrite details and relations: %w", err)
	}

	return badgerhelper.RetryOnConflict(func() error {
		txn := s.db.NewTransaction(true)
		defer txn.Discard()

		for _, key := range []ds.Key{
			indexQueueBase.ChildString(id.ObjectID),
			indexedHeadsState.ChildString(id.ObjectID),
		} {
			if err = txn.Delete(key.Bytes()); err != nil {
				return err
			}
		}

		txn, err = s.eraseLinksForObject(txn, id.ObjectID)
		if err != nil {
			return err
		}
		err = txn.Commit()
		if err != nil {
			return fmt.Errorf("delete object info: %w", err)
		}

		if s.fts != nil {
			err = s.removeFromIndexQueue(id.ObjectID)
			if err != nil {
				log.Errorf("error removing %s from index queue: %s", id, err)
			}
			if err := s.fts.DeleteObject(id.ObjectID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *dsObjectStore) DeleteLinks(ids ...string) (err error) {
	return badgerhelper.RetryOnConflict(func() error {
		txn := s.db.NewTransaction(true)
		defer txn.Discard()
		for _, id := range ids {
			txn, err = s.eraseLinksForObject(txn, id)
			if err != nil {
				return fmt.Errorf("erase links for object %s: %w", id, err)
			}
		}
		return txn.Commit()
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
		if errors.Is(err, badger.ErrTxnTooBig) {
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
