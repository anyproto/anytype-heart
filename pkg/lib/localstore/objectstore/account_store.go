package objectstore

import (
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
)

const (
	accountStatusKey = "account_status"
)

func (s *dsObjectStore) SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) error {
	return s.accountStatus.Set(s.componentCtx, anystoreprovider.SystemKeys.AccountStatus(), status)
}

func (s *dsObjectStore) GetAccountStatus() (*coordinatorproto.SpaceStatusPayload, error) {
	return s.accountStatus.Get(s.componentCtx, anystoreprovider.SystemKeys.AccountStatus())
}
