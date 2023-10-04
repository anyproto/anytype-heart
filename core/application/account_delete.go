package application

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

var (
	ErrAccountIsAlreadyDeleted = errors.New("account is already deleted")
	ErrAccountIsActive         = errors.New("account is active")
)

func (s *Service) AccountDelete(ctx context.Context, req *pb.RpcAccountDeleteRequest) (*model.AccountStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	convErr := func(err error) error {
		switch err {
		case spacecore.ErrSpaceIsDeleted:
			return ErrAccountIsAlreadyDeleted
		case spacecore.ErrSpaceDeletionPending:
			return ErrAccountIsAlreadyDeleted
		case spacecore.ErrSpaceIsCreated:
			return ErrAccountIsActive
		default:
			return err
		}
	}
	var (
		accountService = s.app.MustComponent(account.CName).(account.Service)
		status         *model.AccountStatus
	)
	if !req.Revert {
		networkStatus, err := accountService.Delete(ctx)
		if err != nil {
			return nil, convErr(err)
		}
		status = &model.AccountStatus{
			StatusType:   model.AccountStatusType(networkStatus.Status),
			DeletionDate: networkStatus.DeletionDate.Unix(),
		}
	} else {
		err := accountService.RevertDeletion(ctx)
		if err != nil {
			return nil, convErr(err)
		}
		status = &model.AccountStatus{
			StatusType: model.AccountStatusType(spacecore.SpaceStatusCreated),
		}
	}

	// so we will receive updated account status
	s.refreshRemoteAccountState()
	return status, nil
}

func (s *Service) refreshRemoteAccountState() {
	fetcher := s.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	fetcher.Refetch()
}
