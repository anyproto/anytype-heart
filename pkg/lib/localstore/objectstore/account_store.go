package objectstore

import (
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
)

func (s *dsObjectStore) GetCurrentWorkspaceID() (string, error) {
	return getValue(s.db, currentWorkspace.Bytes(), bytesToString)
}

func (s *dsObjectStore) SetCurrentWorkspaceID(workspaceID string) (err error) {
	return setValue(s.db, currentWorkspace.Bytes(), workspaceID)
}

func (s *dsObjectStore) RemoveCurrentWorkspaceID() (err error) {
	return deleteValue(s.db, currentWorkspace.Bytes())
}

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error) {
	return setValue(s.db, accountStatus.Bytes(), status)
}

func (s *dsObjectStore) GetAccountStatus() (*coordinatorproto.SpaceStatusPayload, error) {
	return getValue(s.db, accountStatus.Bytes(), func(raw []byte) (*coordinatorproto.SpaceStatusPayload, error) {
		status := &coordinatorproto.SpaceStatusPayload{}
		return status, proto.Unmarshal(raw, status)
	})
}
