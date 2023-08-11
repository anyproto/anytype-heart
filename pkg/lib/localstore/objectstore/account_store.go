package objectstore

import (
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

func (s *dsObjectStore) GetCurrentWorkspaceID() (string, error) {
	return badgerhelper.GetValue(s.db, currentWorkspace.Bytes(), bytesToString)
}

func (s *dsObjectStore) SetCurrentWorkspaceID(workspaceID string) (err error) {
	return badgerhelper.SetValue(s.db, currentWorkspace.Bytes(), workspaceID)
}

func (s *dsObjectStore) RemoveCurrentWorkspaceID() (err error) {
	return badgerhelper.DeleteValue(s.db, currentWorkspace.Bytes())
}

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error) {
	return badgerhelper.SetValue(s.db, accountStatus.Bytes(), status)
}

func (s *dsObjectStore) GetAccountStatus() (*coordinatorproto.SpaceStatusPayload, error) {
	return badgerhelper.GetValue(s.db, accountStatus.Bytes(), func(raw []byte) (*coordinatorproto.SpaceStatusPayload, error) {
		status := &coordinatorproto.SpaceStatusPayload{}
		return status, proto.Unmarshal(raw, status)
	})
}
