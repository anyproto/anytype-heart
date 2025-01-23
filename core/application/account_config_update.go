package application

import (
	"context"
	"errors"

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
	cfg := config.ConfigRequired{}
	cfg.CustomFileStorePath = req.IPFSStorageAddr
	err := config.WriteJsonConfig(conf.GetConfigPath(), cfg)
	if err != nil {
		return errors.Join(ErrFailedToWriteConfig, err)
	}
	return nil
}

func (s *Service) AccountChangeJsonApiAddr(ctx context.Context, addr string) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.app == nil {
		return ErrApplicationIsNotRunning
	}
	apiService := app.MustComponent[api.Service](s.app)
	return apiService.ReassignAddress(ctx, addr)
}
