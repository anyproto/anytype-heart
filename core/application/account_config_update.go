package application

import (
	"context"

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
	return conf.UpdatePersistentConfig(func(cfg *config.ConfigPersistent) (updated bool) {
		if cfg.CustomFileStorePath == req.IPFSStorageAddr {
			return false
		}
		cfg.CustomFileStorePath = req.IPFSStorageAddr
		return true
	})

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
