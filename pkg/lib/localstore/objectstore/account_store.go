package objectstore

import (
	"fmt"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
)

func (s *dsObjectStore) GetCurrentWorkspaceID() (string, error) {
	txn, err := s.ds.NewTransaction(true)
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

func (s *dsObjectStore) SetCurrentWorkspaceID(workspaceID string) (err error) {
	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Put(currentWorkspace, []byte(workspaceID)); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (s *dsObjectStore) RemoveCurrentWorkspaceID() (err error) {
	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Delete(currentWorkspace); err != nil {
		return fmt.Errorf("failed to delete from ds: %w", err)
	}

	return txn.Commit()
}

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error) {
	txn, err := s.ds.NewTransaction(false)
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

func (s *dsObjectStore) GetAccountStatus() (status *coordinatorproto.SpaceStatusPayload, err error) {
	txn, err := s.ds.NewTransaction(true)
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
