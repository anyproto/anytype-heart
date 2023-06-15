package objectstore

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/noctxds"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct) error {
	s.l.Lock()
	defer s.l.Unlock()
	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	var (
		before model.ObjectInfo
	)

	if details != nil {
		exInfo, err := s.getObjectInfo(txn, id)
		if err != nil {
			log.Debugf("UpdateObject failed to get ex state for object %s: %s", id, err.Error())
		}

		if exInfo != nil {
			before = *exInfo
		} else {
			// init an empty state to skip nil checks later
			before = model.ObjectInfo{
				Details: &types.Struct{Fields: map[string]*types.Value{}},
			}
		}
	}

	err = s.updateObjectDetails(txn, id, before, details)
	if err != nil {
		return err
	}
	err = txn.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (s *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return s.updateObjectLinks(txn, id, links)
	})
}

func (s *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	return setValue(s.db, pagesSnippetBase.ChildString(id).Bytes(), snippet)
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	// todo: review this method. Any other way to do this?
	for {
		err := s.updatePendingLocalDetails(id, proc)
		if errors.Is(err, badger.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}

func (s *dsObjectStore) updatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	key := pendingDetailsBase.ChildString(id)

	objDetails, err := s.getPendingLocalDetails(txn, id)
	if err != nil && err != ds.ErrNotFound {
		return fmt.Errorf("get pending details: %w", err)
	}

	details := objDetails.GetDetails()
	if details == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if details.Fields == nil {
		details.Fields = map[string]*types.Value{}
	}
	details, err = proc(details)
	if err != nil {
		return fmt.Errorf("run a modifier: %w", err)
	}
	if details == nil {
		err = txn.Delete(key)
		if err != nil {
			return err
		}
		return txn.Commit()
	}
	b, err := proto.Marshal(&model.ObjectDetails{Details: details})
	if err != nil {
		return err
	}
	err = txn.Put(key, b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (s *dsObjectStore) getPendingLocalDetails(txn noctxds.Txn, id string) (*model.ObjectDetails, error) {
	val, err := txn.Get(pendingDetailsBase.ChildString(id))
	if err != nil {
		return nil, err
	}
	return unmarshalDetails(id, val)
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
