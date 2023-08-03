package account

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"fmt"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"path/filepath"
	"os"
)

func (s *Service) AccountStop(req *pb.RpcAccountStopRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.app == nil {
		return domain.WrapErrorWithCode(fmt.Errorf("anytype node not set"), pb.RpcAccountStopResponseError_ACCOUNT_IS_NOT_RUNNING)
	}

	if req.RemoveData {
		err := s.accountRemoveLocalData()
		if err != nil {
			return domain.WrapErrorWithCode(oserror.TransformError(err), pb.RpcAccountStopResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA)
		}
	} else {
		err := s.stop()
		if err != nil {
			return domain.WrapErrorWithCode(err, pb.RpcAccountStopResponseError_FAILED_TO_STOP_NODE)
		}
	}
	return nil
}

func (s *Service) accountRemoveLocalData() error {
	conf := s.app.MustComponent(config.CName).(*config.Config)
	address := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account()

	configPath := conf.GetConfigPath()
	fileConf := config.ConfigRequired{}
	if err := config.GetFileConfig(configPath, &fileConf); err != nil {
		return err
	}

	err := s.stop()
	if err != nil {
		return err
	}

	if fileConf.CustomFileStorePath != "" {
		if err2 := os.RemoveAll(fileConf.CustomFileStorePath); err2 != nil {
			return err2
		}
	}

	err = os.RemoveAll(filepath.Join(s.rootPath, address))
	if err != nil {
		return err
	}

	return nil
}
