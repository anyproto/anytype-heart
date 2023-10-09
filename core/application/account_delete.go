package application

import (
	"context"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

func (s *Service) AccountDelete(ctx context.Context) (*model.AccountStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var (
		accountService = s.app.MustComponent(account.CName).(account.Service)
		status         *model.AccountStatus
	)
	networkStatus, err := accountService.Delete(ctx)
	if err != nil {
		return nil, err
	}
	status = &model.AccountStatus{
		StatusType:   model.AccountStatusType(networkStatus.Status),
		DeletionDate: networkStatus.DeletionDate.Unix(),
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
