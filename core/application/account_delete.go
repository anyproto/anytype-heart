package application

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"context"
	"github.com/anyproto/anytype-heart/core/configfetcher"
)

func (s *Service) AccountDelete(ctx context.Context, req *pb.RpcAccountDeleteRequest) (*model.AccountStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	spaceService := s.app.MustComponent(space.CName).(space.Service)
	resp, err := spaceService.DeleteAccount(ctx, req.Revert)
	status := &model.AccountStatus{
		StatusType:   model.AccountStatusType(resp.Status),
		DeletionDate: resp.DeletionDate.Unix(),
	}

	// so we will receive updated account status
	s.refreshRemoteAccountState()

	code := pb.RpcAccountDeleteResponseError_UNKNOWN_ERROR
	switch err {
	case space.ErrSpaceIsDeleted:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceDeletionPending:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceIsCreated:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ACTIVE
	}
	return status, domain.WrapErrorWithCode(err, code)
}

func (s *Service) refreshRemoteAccountState() {
	fetcher := s.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	fetcher.Refetch()
}
