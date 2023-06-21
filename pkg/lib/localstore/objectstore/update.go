package objectstore

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/noctxds"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

// func (s *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct) error {
// 	s.l.Lock()
// 	defer s.l.Unlock()
// 	txn, err := s.ds.NewTransaction(false)
// 	if err != nil {
// 		return fmt.Errorf("error creating txn in datastore: %w", err)
// 	}
// 	defer txn.Discard()
// 	var (
// 		before model.ObjectInfo
// 	)
//
// 	if details != nil {
// 		exInfo, err := s.getObjectInfo(txn, id)
// 		if err != nil {
// 			log.Debugf("UpdateObject failed to get ex state for object %s: %s", id, err.Error())
// 		}
//
// 		if exInfo != nil {
// 			before = *exInfo
// 		} else {
// 			// init an empty state to skip nil checks later
// 			before = model.ObjectInfo{
// 				Details: &types.Struct{Fields: map[string]*types.Value{}},
// 			}
// 		}
// 	}
//
// 	err = s.updateObjectDetails(txn, id, before, details)
// 	if err != nil {
// 		return err
// 	}
// 	err = txn.Commit()
// 	if err != nil {
// 		return err
// 	}
//
// 	return nil
// }

func (s *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	return s.db.Update(func(txn *badger.Txn) error {
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

func (s *dsObjectStore) updateObjectDetails(txn noctxds.Txn, id string, before model.ObjectInfo, details *types.Struct) error {
	if details != nil {
		if err := s.updateDetails(txn, id, &model.ObjectDetails{Details: before.Details}, &model.ObjectDetails{Details: details}); err != nil {
			return err
		}
	}

	return nil
}

func (s *dsObjectStore) updateDetails(txn noctxds.Txn, id string, oldDetails *model.ObjectDetails, newDetails *model.ObjectDetails) error {
	metrics.ObjectDetailsUpdatedCounter.Inc()
	detailsKey := pagesDetailsBase.ChildString(id)

	if newDetails.GetDetails().GetFields() == nil {
		return fmt.Errorf("newDetails is nil")
	}

	newDetails.Details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id) // always ensure we have id set
	b, err := proto.Marshal(newDetails)
	if err != nil {
		return err
	}
	err = txn.Put(detailsKey, b)
	if err != nil {
		return err
	}

	if pbtypes.GetString(newDetails.Details, bundle.RelationKeyWorkspaceId.String()) == "" {
		log.With("objectID", id).With("stack", debug.StackCompact(false)).Warnf("workspaceId erased")
	}

	if oldDetails.GetDetails().Equal(newDetails.GetDetails()) {
		return ErrDetailsNotChanged
	}

	if newDetails != nil && newDetails.Details.Fields != nil {
		s.sendUpdatesToSubscriptions(id, newDetails.Details)
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
