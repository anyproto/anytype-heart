package objectstore

import (
	"fmt"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
)

func (m *dsObjectStore) GetCurrentWorkspaceId() (string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return "", fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	val, err := txn.Get(currentWorkspace)
	if err != nil {
		return "", err
	}
	return string(val), nil
}

func (m *dsObjectStore) SetCurrentWorkspaceId(threadId string) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Put(currentWorkspace, []byte(threadId)); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) RemoveCurrentWorkspaceId() (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Delete(currentWorkspace); err != nil {
		return fmt.Errorf("failed to delete from ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	b, err := status.Marshal()
	if err != nil {
		return err
	}

	if err := txn.Put(accountStatus, b); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) GetAccountStatus() (status *coordinatorproto.SpaceStatusPayload, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	status = &coordinatorproto.SpaceStatusPayload{}
	if val, err := txn.Get(accountStatus); err != nil {
		return nil, err
	} else if err := proto.Unmarshal(val, status); err != nil {
		return nil, err
	}

	return status, nil
}
