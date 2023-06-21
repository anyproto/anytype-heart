package objectstore

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct) error {
	if details == nil {
		return nil
	}
	if details.Fields == nil {
		return fmt.Errorf("details fields are nil")
	}

	key := pagesDetailsBase.ChildString(id).Bytes()
	return s.updateTxn(func(txn *badger.Txn) error {
		prev, ok := s.cache.Get(key)
		if !ok {
			it, err := txn.Get(key)
			if err != nil && err != badger.ErrKeyNotFound {
				return fmt.Errorf("get item: %w", err)
			}
			if err != badger.ErrKeyNotFound {
				prev, err = s.unmarshalDetailsFromItem(it)
				if err != nil {
					return fmt.Errorf("extract details: %w", err)
				}
			}
		}
		detailsModel := &model.ObjectDetails{
			Details: details,
		}
		if prev != nil && proto.Equal(prev.(*model.ObjectDetails), detailsModel) {
			return ErrDetailsNotChanged
		}
		// Ensure ID is set
		details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		s.sendUpdatesToSubscriptions(id, details)

		s.cache.Set(key, detailsModel, int64(detailsModel.Size()))
		val, err := proto.Marshal(detailsModel)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		return txn.Set(key, val)
	})
}

func (s *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		return s.updateObjectLinks(txn, id, links)
	})
}

func (s *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	return setValue(s.db, pagesSnippetBase.ChildString(id).Bytes(), snippet)
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		key := pendingDetailsBase.ChildString(id).Bytes()

		objDetails, err := s.getPendingLocalDetails(txn, key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("get pending details: %w", err)
		}

		oldDetails := objDetails.GetDetails()
		if oldDetails == nil {
			oldDetails = &types.Struct{Fields: map[string]*types.Value{}}
		}
		if oldDetails.Fields == nil {
			oldDetails.Fields = map[string]*types.Value{}
		}
		newDetails, err := proc(oldDetails)
		if err != nil {
			return fmt.Errorf("run a modifier: %w", err)
		}
		if newDetails == nil {
			err = txn.Delete(key)
			if err != nil {
				return err
			}
			return nil
		}

		if newDetails.Fields == nil {
			newDetails.Fields = map[string]*types.Value{}
		}
		newDetails.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		err = setValueTxn(txn, key, &model.ObjectDetails{Details: newDetails})
		if err != nil {
			return fmt.Errorf("put pending details: %w", err)
		}
		return nil
	})
}

func (s *dsObjectStore) getPendingLocalDetails(txn *badger.Txn, key []byte) (*model.ObjectDetails, error) {
	return getValueTxn(txn, key, func(raw []byte) (*model.ObjectDetails, error) {
		var res model.ObjectDetails
		err := proto.Unmarshal(raw, &res)
		return &res, err
	})
}

func (s *dsObjectStore) updateObjectLinks(txn *badger.Txn, id string, links []string) error {
	exLinks, err := findOutboundLinks(txn, id)
	if err != nil {
		log.Errorf("error while finding outbound links for %s: %s", id, err)
	}
	var addedLinks, removedLinks []string

	removedLinks, addedLinks = slice.DifferenceRemovedAdded(exLinks, links)
	if len(addedLinks) > 0 {
		for _, k := range pageLinkKeys(id, addedLinks) {
			err := txn.Set(k.Bytes(), nil)
			if err != nil {
				return fmt.Errorf("setting link %s: %w", k, err)
			}
		}
	}
	if len(removedLinks) > 0 {
		for _, k := range pageLinkKeys(id, removedLinks) {
			if err := txn.Delete(k.Bytes()); err != nil {
				return err
			}
		}
	}

	return nil
}

// should be called under the mutex
func (s *dsObjectStore) sendUpdatesToSubscriptions(id string, details *types.Struct) {
	detCopy := pbtypes.CopyStruct(details)
	detCopy.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for i := range s.subscriptions {
		go func(sub database.Subscription) {
			_ = sub.Publish(id, detCopy)
		}(s.subscriptions[i])
	}
}
