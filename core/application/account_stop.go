package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/debug"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/pushnotification"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

var (
	ErrFailedToRemoveAccountData = errors.New("failed to remove account data")
)

// cancelStartIfInProcess cancels the start process if it is in progress, otherwise does nothing
func (s *Service) cancelStartIfInProcess() {
	s.appAccountStartInProcessCancelMutex.Lock()
	defer s.appAccountStartInProcessCancelMutex.Unlock()
	if s.appAccountStartInProcessCancel != nil {
		log.Warn("canceling in-process account start")
		s.appAccountStartInProcessCancel()
		s.appAccountStartInProcessCancel = nil
	}
}

func (s *Service) AccountStop(req *pb.RpcAccountStopRequest) error {
	s.cancelStartIfInProcess()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-stopped:
		case <-time.After(app.StopDeadline + time.Second*5):
			// this is extra protection in case we stuck at s.lock
			_, _ = os.Stderr.Write([]byte("AccountStop timeout\n"))
			_, _ = os.Stderr.Write(debug.Stack(true))
			panic("app.Close AccountStop timeout")
		}
	}()
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	// try to revoke push notification token for mobile clients
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		if pushService := s.app.Component(pushnotification.CName).(pushnotification.Service); pushService != nil {
			go func() {
				_ = pushService.RevokeToken(context.Background())
			}()
		}
	}

	if req.RemoveData {
		err := s.accountRemoveLocalData()
		if err != nil {
			return errors.Join(ErrFailedToRemoveAccountData, anyerror.CleanupError(err))
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
	lang := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).FtsPrimaryLang()
	accountId := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account()
	conf := s.app.MustComponent(config.CName).(*config.Config)

	if req.NetworkMode == pb.RpcAccount_CustomConfig {
		// check if file exists at path
		b, err := os.ReadFile(req.NetworkCustomConfigFilePath)
		if os.IsNotExist(err) {
			return config.ErrNetworkFileNotFound
		}
		if err != nil {
			return errors.Join(config.ErrNetworkFileFailedToRead, err)
		}
		var cfg nodeconf.Configuration
		err = yaml.Unmarshal(b, &cfg)
		if err != nil {
			// wrap errors into each other
			return errors.Join(config.ErrNetworkFileFailedToRead, err)
		}
		if conf.NetworkId != "" && conf.NetworkId != cfg.NetworkId {
			return config.ErrNetworkIdMismatch
		}
	}

	err := s.stop()
	if err != nil {
		return ErrFailedToStopApplication
	}

	_, err = s.start(ctx, accountId, rootPath, conf.DontStartLocalNetworkSyncAutomatically, conf.JsonApiListenAddr,
		conf.PeferYamuxTransport, req.NetworkMode, req.NetworkCustomConfigFilePath, lang, "", conf.EnableMembershipV2)
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
