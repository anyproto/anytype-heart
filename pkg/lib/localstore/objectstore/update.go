package objectstore

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
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
	newDetails := &model.ObjectDetails{
		Details: details,
	}

	key := pagesDetailsBase.ChildString(id).Bytes()
	txErr := s.updateTxn(func(txn *badger.Txn) error {
		oldDetails, err := s.extractDetailsByKey(txn, key)
		if err != nil && !badgerhelper.IsNotFound(err) {
			return fmt.Errorf("extract details: %w", err)
		}
		if oldDetails != nil && oldDetails.Details.Equal(newDetails.Details) {
			return ErrDetailsNotChanged
		}
		// Ensure ID is set
		details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		s.sendUpdatesToSubscriptions(id, details)
		val, err := proto.Marshal(newDetails)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		return txn.Set(key, val)
	})
	if txErr != nil {
		return txErr
	}
	s.cache.Set(key, newDetails, int64(newDetails.Size()))
	return nil
}

func (s *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	var (
		added, removed []string
	)
	err := s.updateTxn(func(txn *badger.Txn) error {
		var err error
		added, removed, err = s.updateObjectLinks(txn, id, links)
		return err
	})
	// todo: too big txn is not handled
	if err != nil {
		return err
	}
	s.RLock()
	defer s.RUnlock()
	if s.onLinksUpdateCallback != nil && len(added)+len(removed) > 0 {
		s.onLinksUpdateCallback(LinksUpdateInfo{
			LinksFromId: id,
			Added:       added,
			Removed:     removed,
		})
	}
	return nil
}

func (s *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	return badgerhelper.SetValue(s.db, pagesSnippetBase.ChildString(id).Bytes(), snippet)
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		key := pendingDetailsBase.ChildString(id).Bytes()

		objDetails, err := s.getPendingLocalDetails(txn, key)
		if err != nil && !badgerhelper.IsNotFound(err) {
			return fmt.Errorf("get pending details: %w", err)
		}

		oldDetails := objDetails.GetDetails()
		if oldDetails == nil || oldDetails.Fields == nil {
			oldDetails = &types.Struct{Fields: map[string]*types.Value{}}
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
		err = badgerhelper.SetValueTxn(txn, key, &model.ObjectDetails{Details: newDetails})
		if err != nil {
			return fmt.Errorf("put pending details: %w", err)
		}
		return nil
	})
}

// ModifyObjectDetails updates existing details in store using modification function `proc`
// `proc` should return ErrDetailsNotChanged in case old details are empty or no changes were made
func (s *dsObjectStore) ModifyObjectDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	if proc == nil {
		return nil
	}
	var payload *model.ObjectDetails
	key := pagesDetailsBase.ChildString(id).Bytes()

	if err := s.updateTxn(func(txn *badger.Txn) error {
		oldDetails, err := s.extractDetailsByKey(txn, key)
		if err != nil && !badgerhelper.IsNotFound(err) {
			return fmt.Errorf("get existing details: %w", err)
		}

		newDetails, err := proc(oldDetails.GetDetails())
		if err != nil {
			return fmt.Errorf("run a modifier: %w", err)
		}

		if newDetails == nil || newDetails.Fields == nil {
			newDetails = &types.Struct{Fields: map[string]*types.Value{}}
		}
		// Ensure ID is set
		newDetails.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		s.sendUpdatesToSubscriptions(id, newDetails)
		payload = &model.ObjectDetails{Details: newDetails}
		err = badgerhelper.SetValueTxn(txn, key, payload)
		if err != nil {
			return fmt.Errorf("put details: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	s.cache.Set(key, payload, int64(payload.Size()))
	return nil
}

func (s *dsObjectStore) extractDetailsByKey(txn *badger.Txn, key []byte) (*model.ObjectDetails, error) {
	it, err := txn.Get(key)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return s.unmarshalDetailsFromItem(it)
}

func (s *dsObjectStore) getPendingLocalDetails(txn *badger.Txn, key []byte) (*model.ObjectDetails, error) {
	return badgerhelper.GetValueTxn(txn, key, func(raw []byte) (*model.ObjectDetails, error) {
		var res model.ObjectDetails
		err := proto.Unmarshal(raw, &res)
		return &res, err
	})
}

func (s *dsObjectStore) updateObjectLinks(txn *badger.Txn, id string, links []string) (added []string, removed []string, err error) {
	exLinks, err := findOutboundLinks(txn, id)
	if err != nil {
		log.Errorf("error while finding outbound links for %s: %s", id, err)
	}

	removed, added = slice.DifferenceRemovedAdded(exLinks, links)
	if len(added) > 0 {
		for _, k := range pageLinkKeys(id, added) {
			err = txn.Set(k.Bytes(), nil)
			if err != nil {
				err = fmt.Errorf("setting link %s: %w", k, err)
				return
			}
		}
	}
	if len(removed) > 0 {
		for _, k := range pageLinkKeys(id, removed) {
			if err = txn.Delete(k.Bytes()); err != nil {
				return
			}
		}
	}
	return
}

func (s *dsObjectStore) sendUpdatesToSubscriptions(id string, details *types.Struct) {
	detCopy := pbtypes.CopyStruct(details, false)
	detCopy.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	s.RLock()
	defer s.RUnlock()
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for _, sub := range s.subscriptions {
		_ = sub.PublishAsync(id, detCopy)
	}
}
