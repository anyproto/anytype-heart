package application

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

var (
	ErrAccountIsAlreadyDeleted = errors.New("account is already deleted")
	ErrAccountIsActive         = errors.New("account is active")
)

func (s *Service) AccountDelete(ctx context.Context) (*model.AccountStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.app == nil {
		return nil, ErrApplicationIsNotRunning
	}

	var (
		accountService = s.app.MustComponent(account.CName).(account.Service)
		status         *model.AccountStatus
	)
	toBeDeleted, err := accountService.Delete(ctx)
	if err != nil {
		return nil, err
	}
	status = &model.AccountStatus{
		StatusType:   model.Account_PendingDeletion,
		DeletionDate: toBeDeleted,
	}
	s.refreshRemoteAccountState()
	return status, nil
}

func (s *Service) AccountRevertDeletion(ctx context.Context) (*model.AccountStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	accountService := s.app.MustComponent(account.CName).(account.Service)
	err := accountService.RevertDeletion(ctx)
	if err != nil {
		return nil, err
	}
	status := &model.AccountStatus{
		StatusType: model.AccountStatusType(spacecore.SpaceStatusCreated),
	}
	s.refreshRemoteAccountState()
	return status, nil
}

func (s *Service) refreshRemoteAccountState() {
	fetcher := s.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	fetcher.Refetch()
}
