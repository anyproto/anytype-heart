package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	trace2 "runtime/trace"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/secureservice/handshake"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

var (
	ErrEmptyAccountID      = errors.New("empty account id")
	ErrNoMnemonicProvided  = errors.New("no mnemonic provided")
	ErrIncompatibleVersion = errors.New("can't fetch account's data because remote nodes have incompatible protocol version. Please update anytype to the latest version")

	ErrAnotherProcessIsRunning   = errors.New("another anytype process is running")
	ErrFailedToFindAccountInfo   = errors.New("failed to find account info")
	ErrAccountIsDeleted          = errors.New("account is deleted")
	ErrAccountStoreIsNotMigrated = errors.New("account store is not migrated")
)

func (s *Service) AccountSelect(ctx context.Context, req *pb.RpcAccountSelectRequest) (*model.Account, error) {
	if req.Id == "" {
		return nil, ErrEmptyAccountID
	}
	if err := s.validateAccountID(req.Id); err != nil {
		return nil, err
	}
	rootPath := req.RootPath
	if rootPath == "" {
		rootPath = s.rootPath
	}
	curMigration, err := s.migrationManager.getOrCreateMigration(rootPath, req.Id, req.FulltextPrimaryLanguage)
	if err != nil {
		return nil, errors.Join(ErrFailedToStartApplication, err)
	}
	if !curMigration.successful() {
		return nil, ErrAccountStoreIsNotMigrated
	}
	if s.migrationManager.isRunning() {
		return nil, ErrMigrationRunning
	}

	if startupTraceEnabled && runtime.GOOS != "android" && runtime.GOOS != "ios" {
		s.traceRecorder.start()
		defer s.traceRecorder.stop()
	}
	s.cancelStartIfInProcess()
	s.lock.Lock()
	defer s.lock.Unlock()

	s.requireClientWithVersion()

	// we already have this account running, lets just stop events
	if s.app != nil &&
		req.Id == s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account() &&
		s.accountLease != nil &&
		s.accountLease.Matches(rootPath, req.Id) &&
		s.accountLease.Usable() {
		// TODO What should we do?
		// objectCache := app.MustComponent[objectcache.Cache](s.app)
		// objectCache.CloseBlocks()

		acc := &model.Account{Id: req.Id}
		var err error
		acc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx)
		if err != nil {
			return nil, err
		}
		go s.refreshRemoteAccountState()
		return acc, nil
	}

	// in case user selected account other than the first one(used to perform search)
	// or this is the first time in this session we run the Anytype node
	if err := s.switchAccountLease(ctx, rootPath, req.Id); err != nil {
		return nil, err
	}
	metrics.Service.SetWorkingDir(rootPath, req.Id)

	return s.start(ctx, req.Id, rootPath, req.DisableLocalNetworkSync, req.JsonApiListenAddr,
		req.PreferYamuxTransport, req.NetworkMode, req.NetworkCustomConfigFilePath, req.FulltextPrimaryLanguage, req.JoinStreamURL, req.EnableMembershipV2, req.PreferredSpaceId)
}

func (s *Service) start(
	ctx context.Context,
	id string,
	rootPath string,
	disableLocalNetworkSync bool,
	jsonApiListenAddr string,
	preferYamux bool,
	networkMode pb.RpcAccountNetworkMode,
	networkConfigFilePath string,
	lang string,
	joinStreamUrl string,
	enableMembershipV2 bool,
	preferredSpaceId string,
) (acc *model.Account, err error) {
	ctx, task := trace2.NewTask(ctx, "application.start")
	defer task.End()

	if rootPath != "" {
		s.rootPath = rootPath
	}
	if lang != "" {
		s.fulltextPrimaryLanguage = lang
	}

	if s.derivedKeys == nil {
		return nil, ErrWalletNotInitialized
	}
	if err = s.acquireAccountLease(ctx, s.rootPath, id); err != nil {
		return nil, err
	}
	appStarted := false
	defer func() {
		if !appStarted && err != nil {
			err = errors.Join(err, s.releaseAccountLease())
		}
	}()

	var repoWasMissing bool
	if _, statErr := os.Stat(filepath.Join(s.rootPath, id)); os.IsNotExist(statErr) {
		repoWasMissing = true
		if err = core.WalletInitRepo(s.rootPath, s.derivedKeys.Identity); err != nil {
			return nil, errors.Join(ErrFailedToCreateLocalRepo, err)
		}
	}

	defer func() {
		if repoWasMissing && !appStarted && err != nil {
			if removeErr := os.RemoveAll(filepath.Join(s.rootPath, id)); removeErr != nil {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	cfg := anytype.BootstrapConfig(false, joinStreamUrl)
	if disableLocalNetworkSync {
		cfg.DontStartLocalNetworkSyncAutomatically = true
	}

	if jsonApiListenAddr != "" {
		cfg.JsonApiListenAddr = jsonApiListenAddr
	}
	if preferYamux {
		cfg.PeferYamuxTransport = true
	}
	if enableMembershipV2 {
		cfg.EnableMembershipV2 = true
	}
	cfg.PreferredSpaceId = preferredSpaceId
	if networkMode > 0 {
		cfg.NetworkMode = networkMode
		cfg.NetworkCustomConfigFilePath = networkConfigFilePath
	}
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, *s.derivedKeys, s.fulltextPrimaryLanguage),
		s.eventSender,
	}

	request := "account_select"
	if repoWasMissing {
		// if we have created the repo, we need to highlight that we are recovering the account
		request = request + "_recover"
	}

	ctx, cancel := context.WithCancel(context.WithValue(ctx, metrics.CtxKeyEntrypoint, request))
	// save the cancel function to be able to stop the app in case of account stop or other select/create operation is called
	s.appAccountStartInProcessCancelMutex.Lock()
	s.appAccountStartInProcessCancel = cancel
	s.appAccountStartInProcessCancelMutex.Unlock()
	newApp, startErr := anytype.StartNewApp(
		ctx,
		s.clientWithVersion,
		comps...,
	)
	s.appAccountStartInProcessCancelMutex.Lock()
	s.appAccountStartInProcessCancel = nil
	s.appAccountStartInProcessCancelMutex.Unlock()

	if startErr != nil {
		if errors.Is(startErr, spacesyncproto.ErrSpaceIsDeleted) {
			return nil, errors.Join(ErrAccountIsDeleted, startErr)
		}
		if errors.Is(startErr, space.ErrSpaceNotExists) {
			return nil, errors.Join(ErrFailedToFindAccountInfo, startErr)
		}
		if errors.Is(startErr, handshake.ErrIncompatibleVersion) {
			return nil, ErrIncompatibleVersion
		}
		return nil, errors.Join(ErrFailedToStartApplication, startErr)
	}
	s.app = newApp
	appStarted = true

	acc = &model.Account{Id: id}
	acc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx)

	return acc, err
}
