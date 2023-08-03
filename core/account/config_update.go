package account

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"fmt"
)

func (s *Service) AccountConfigUpdate(req *pb.RpcAccountConfigUpdateRequest) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.app == nil {
		return domain.WrapErrorWithCode(fmt.Errorf("anytype node not set"), pb.RpcAccountConfigUpdateResponseError_ACCOUNT_IS_NOT_RUNNING)
	}

	conf := s.app.MustComponent(config.CName).(*config.Config)
	cfg := config.ConfigRequired{}
	cfg.TimeZone = req.TimeZone
	cfg.CustomFileStorePath = req.IPFSStorageAddr
	err := config.WriteJsonConfig(conf.GetConfigPath(), cfg)
	if err != nil {
		return domain.WrapErrorWithCode(err, pb.RpcAccountConfigUpdateResponseError_FAILED_TO_WRITE_CONFIG)
	}
	return nil
}
