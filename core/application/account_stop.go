package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/anyproto/any-sync/nodeconf"
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

// AccountStop stops the running app or, when a start is in flight, cancels it
// and returns at once without taking s.lock: the start closes whatever it
// published on its way out (see the protocol in app_start.go). Cancelling a
// pending start is a successful stop even though it leaves no app to close —
// the client needs to tell that apart from "nothing was running" to know its
// select is coming back.
//
// RemoveData never takes that shortcut. Cancelling costs no lock wait, but a
// client that asked for the account to be erased must not be told the erase
// succeeded while the data is still on disk — that is a lie it has no way to
// detect. So a removal request cancels the start and then waits for the lock
// like any other, and if the start unwound first, leaving no wallet to
// resolve the account directory from, it reports the failure rather than
// reporting success.
func (s *Service) AccountStop(req *pb.RpcAccountStopRequest) error {
	cancelled := s.cancelStart()
	if cancelled && !req.RemoveData {
		return nil
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.app == nil {
		if cancelled {
			// The stop happened; the removal did not.
			return ErrFailedToRemoveAccountData
		}
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
	// published only now, under the lock: a restart that superseded the
	// select it was queued behind would find no app left to restart
	ctx, end := s.beginStart(ctx)
	defer end()

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
		if conf.NetworkId() != "" && conf.NetworkId() != cfg.NetworkId {
			return config.ErrNetworkIdMismatch
		}
	}

	err := s.stop()
	if err != nil {
		return ErrFailedToStopApplication
	}

	_, err = s.start(ctx, accountId, rootPath, conf.DontStartLocalNetworkSyncAutomatically, conf.JsonApiListenAddr,
		conf.PeferYamuxTransport, req.NetworkMode, req.NetworkCustomConfigFilePath, lang, "", conf.EnableMembershipV2, conf.PreferredSpaceId)
	return err
}

func (s *Service) accountRemoveLocalData() error {
	conf := s.app.MustComponent(config.CName).(*config.Config)
	address := s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account()

	customFileStorePath := conf.CustomFileStorePath()

	err := s.stop()
	if err != nil {
		return err
	}

	if customFileStorePath != "" {
		if err2 := os.RemoveAll(customFileStorePath); err2 != nil {
			return err2
		}
	}

	err = os.RemoveAll(filepath.Join(s.rootPath, address))
	if err != nil {
		return err
	}

	return nil
}
