package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/pb"
)

func (s *Service) AccountConfigUpdate(req *pb.RpcAccountConfigUpdateRequest) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	conf := s.app.MustComponent(config.CName).(*config.Config)
	err := conf.SetCustomFileStorePath(req.IPFSStorageAddr)
	if err != nil {
		return errors.Join(ErrFailedToWriteConfig, err)
	}
	return nil
}

func (s *Service) AccountChangeJsonApiAddr(ctx context.Context, addr string) (*pb.EventAccountJsonApiStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.app == nil {
		return nil, ErrApplicationIsNotRunning
	}
	apiService := app.MustComponent[api.Service](s.app)
	status, err := apiService.ReassignAddress(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("reassign json api address: %w", err)
	}
	return status, nil
}
