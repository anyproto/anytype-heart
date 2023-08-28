package objectstore

import (
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
)

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error) {
	return setValue(s.db, accountStatus.Bytes(), status)
}

func (s *dsObjectStore) GetAccountStatus() (*coordinatorproto.SpaceStatusPayload, error) {
	return getValue(s.db, accountStatus.Bytes(), func(raw []byte) (*coordinatorproto.SpaceStatusPayload, error) {
		status := &coordinatorproto.SpaceStatusPayload{}
		return status, proto.Unmarshal(raw, status)
	})
}
