package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/secureservice/handshake"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// we cannot check the constant error from badger because they hardcoded it there
const errSubstringMultipleAnytypeInstance = "Cannot acquire directory lock"

var (
	ErrEmptyAccountID      = errors.New("empty account id")
	ErrNoMnemonicProvided  = errors.New("no mnemonic provided")
	ErrIncompatibleVersion = errors.New("can't fetch account's data because remote nodes have incompatible protocol version. Please update anytype to the latest version")

	ErrAnotherProcessIsRunning = errors.New("another anytype process is running")
	ErrFailedToFindAccountInfo = errors.New("failed to find account info")
	ErrAccountIsDeleted        = errors.New("account is deleted")
)

func (s *Service) AccountSelect(ctx context.Context, req *pb.RpcAccountSelectRequest) (*model.Account, error) {
	if req.Id == "" {
		return nil, ErrEmptyAccountID
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.requireClientWithVersion()

	// we already have this account running, lets just stop events
	if s.app != nil && req.Id == s.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account() {
		bs := s.app.MustComponent(treemanager.CName).(*block.Service)
		bs.CloseBlocks()

		spaceID := app.MustComponent[space.Service](s.app).AccountId()
		acc := &model.Account{Id: req.Id}
		var err error
		acc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		return acc, nil
	}

	// in case user selected account other than the first one(used to perform search)
	// or this is the first time in this session we run the Anytype node
	if err := s.stop(); err != nil {
		return nil, errors.Join(ErrFailedToStopApplication, err)
	}
	if req.RootPath != "" {
		s.rootPath = req.RootPath
	}
	if s.mnemonic == "" {
		return nil, ErrNoMnemonicProvided
	}
	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return nil, err
	}
	var repoWasMissing bool
	if _, err := os.Stat(filepath.Join(s.rootPath, req.Id)); os.IsNotExist(err) {
		repoWasMissing = true
		if err = core.WalletInitRepo(s.rootPath, res.Identity); err != nil {
			return nil, errors.Join(ErrFailedToCreateLocalRepo, err)
		}
	}

	cfg := anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1")
	if req.DisableLocalNetworkSync {
		cfg.DontStartLocalNetworkSyncAutomatically = true
	}
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, res),
		s.eventSender,
	}

	request := "account_select"
	if repoWasMissing {
		// if we have created the repo, we need to highlight that we are recovering the account
		request = request + "_recover"
	}

	s.app, err = anytype.StartNewApp(
		context.WithValue(context.Background(), metrics.CtxKeyEntrypoint, request),
		s.clientWithVersion,
		comps...,
	)
	if err != nil {
		if errors.Is(err, spacesyncproto.ErrSpaceIsDeleted) {
			return nil, errors.Join(ErrAccountIsDeleted, err)
		}
		if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
			return nil, errors.Join(ErrFailedToFindAccountInfo, err)
		}
		if strings.Contains(err.Error(), errSubstringMultipleAnytypeInstance) {
			return nil, errors.Join(ErrAnotherProcessIsRunning, err)
		}
		if errors.Is(err, handshake.ErrIncompatibleVersion) {
			return nil, ErrIncompatibleVersion
		}
		return nil, errors.Join(ErrFailedToStartApplication, err)
	}

	acc := &model.Account{Id: req.Id}
	spaceID := app.MustComponent[space.Service](s.app).AccountId()
	acc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx, spaceID)
	return acc, nil
}
