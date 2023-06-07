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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (m *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	var (
		before model.ObjectInfo
	)

	if details != nil {
		exInfo, err := m.getObjectInfo(txn, id)
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

	err = m.updateObjectDetails(txn, id, before, details)
	if err != nil {
		return err
	}
	err = txn.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (m *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.updateObjectLinks(txn, id, links)
	if err != nil {
		return err
	}
	return txn.Commit()
}

func (m *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if val, err := txn.Get(pagesSnippetBase.ChildString(id)); err == ds.ErrNotFound || string(val) != snippet {
		if err := m.updateSnippet(txn, id, snippet); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (m *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	// todo: review this method. Any other way to do this?
	for {
		err := m.updatePendingLocalDetails(id, proc)
		if errors.Is(err, badger.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}

func (m *dsObjectStore) updatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	key := pendingDetailsBase.ChildString(id)

	objDetails, err := m.getPendingLocalDetails(txn, id)
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

func (m *dsObjectStore) getPendingLocalDetails(txn noctxds.Txn, id string) (*model.ObjectDetails, error) {
	val, err := txn.Get(pendingDetailsBase.ChildString(id))
	if err != nil {
		return nil, err
	}
	return unmarshalDetails(id, val)
}

func (m *dsObjectStore) updateObjectLinks(txn noctxds.Txn, id string, links []string) error {
	exLinks, _ := findOutboundLinks(txn, id)
	var addedLinks, removedLinks []string

	removedLinks, addedLinks = slice.DifferenceRemovedAdded(exLinks, links)
	if len(addedLinks) > 0 {
		for _, k := range pageLinkKeys(id, nil, addedLinks) {
			if err := txn.Put(k, nil); err != nil {
				return err
			}
		}
	}

	if len(removedLinks) > 0 {
		for _, k := range pageLinkKeys(id, nil, removedLinks) {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *dsObjectStore) updateObjectDetails(txn noctxds.Txn, id string, before model.ObjectInfo, details *types.Struct) error {
	if details != nil {
		if err := m.updateDetails(txn, id, &model.ObjectDetails{Details: before.Details}, &model.ObjectDetails{Details: details}); err != nil {
			return err
		}
	}

	return nil
}

func (m *dsObjectStore) updateDetails(txn noctxds.Txn, id string, oldDetails *model.ObjectDetails, newDetails *model.ObjectDetails) error {
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

	err = localstore.UpdateIndexesWithTxn(m, txn, oldDetails, newDetails, id)
	if err != nil {
		return err
	}

	if newDetails != nil && newDetails.Details.Fields != nil {
		m.sendUpdatesToSubscriptions(id, newDetails.Details)
	}

	return nil
}

// should be called under the mutex
func (m *dsObjectStore) sendUpdatesToSubscriptions(id string, details *types.Struct) {
	detCopy := pbtypes.CopyStruct(details)
	detCopy.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	if m.onChangeCallback != nil {
		m.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for i := range m.subscriptions {
		go func(sub database.Subscription) {
			_ = sub.Publish(id, detCopy)
		}(m.subscriptions[i])
	}
}

func (m *dsObjectStore) updateSnippet(txn noctxds.Txn, id string, snippet string) error {
	snippetKey := pagesSnippetBase.ChildString(id)
	return txn.Put(snippetKey, []byte(snippet))
}
