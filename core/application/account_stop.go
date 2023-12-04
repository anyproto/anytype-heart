package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/nodeconf"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	oserror "github.com/anyproto/anytype-heart/util/os"
)

var (
	ErrFailedToRemoveAccountData      = errors.New("failed to remove account data")
	ErrNetworkConfigFileDoesNotExist  = errors.New("network config file does not exist")
	ErrNetworkConfigFileInvalid       = errors.New("network config file invalid")
	ErrNetworkConfigNetworkIdMismatch = errors.New("network id mismatch")
)

func (s *Service) AccountStop(req *pb.RpcAccountStopRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	if req.RemoveData {
		err := s.accountRemoveLocalData()
		if err != nil {
			return errors.Join(ErrFailedToRemoveAccountData, oserror.TransformError(err))
		}
	} else {
		err := s.stop()
		if err != nil {
			return ErrFailedToStopApplication
		}
	}
	return nil
}

func (s *Service) AccountChangeNetworkConfigAndRestart(ctx context.Context, req *pb.RpcAccountChangeNetworkConfigAndRestartRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	rootPath := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).RootPath()
	accountId := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account()
	conf := s.app.MustComponent(config.CName).(*config.Config)

	if req.NetworkMode == pb.RpcAccount_CustomConfig {
		// check if file exists at path
		if b, err := os.ReadFile(req.NetworkCustomConfigFilePath); os.IsNotExist(err) {
			return ErrNetworkConfigFileDoesNotExist
		} else {
			var cfg nodeconf.Configuration
			err = yaml.Unmarshal(b, &cfg)
			if err != nil {
				return ErrNetworkConfigFileInvalid
			}
			if conf.NetworkId != "" && conf.NetworkId != cfg.NetworkId {
				return ErrNetworkConfigNetworkIdMismatch
			}
		}
	}

	err := s.stop()
	if err != nil {
		return ErrFailedToStopApplication
	}

	_, err = s.start(ctx, accountId, rootPath, conf.DontStartLocalNetworkSyncAutomatically, req.NetworkMode, req.NetworkCustomConfigFilePath)
	return err
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
