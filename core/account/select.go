package account

import (
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
	"fmt"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"strings"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/core/anytype"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"os"
	"path/filepath"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"context"
	"github.com/anyproto/anytype-heart/core/block"
	"errors"
)

// we cannot check the constant error from badger because they hardcoded it there
const errSubstringMultipleAnytypeInstance = "Cannot acquire directory lock"

func (s *Service) AccountSelect(ctx context.Context, req *pb.RpcAccountSelectRequest) (*model.Account, error) {
	if req.Id == "" {
		return nil, domain.WrapErrorWithCode(fmt.Errorf("account id is empty"), pb.RpcAccountSelectResponseError_BAD_INPUT)
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
	if err := s.Stop(); err != nil {
		return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE)
	}
	if req.RootPath != "" {
		s.rootPath = req.RootPath
	}
	if s.mnemonic == "" {
		return nil, domain.WrapErrorWithCode(fmt.Errorf("no mnemonic provided"), pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET)
	}
	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return nil, err
	}
	var repoWasMissing bool
	if _, err := os.Stat(filepath.Join(s.rootPath, req.Id)); os.IsNotExist(err) {
		repoWasMissing = true
		if err = core.WalletInitRepo(s.rootPath, res.Identity); err != nil {
			return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO)
		}
	}

	comps := []app.Component{
		anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1", false),
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
		if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
			return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO)
		}
		if err == core.ErrRepoCorrupted {
			return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED)
		}
		if strings.Contains(err.Error(), errSubstringMultipleAnytypeInstance) {
			return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING)
		}
		if errors.Is(err, handshake.ErrIncompatibleVersion) {
			err = fmt.Errorf("can't fetch account's data because remote nodes have incompatible protocol version. Please update anytype to the latest version")
			return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_FAILED_TO_FETCH_REMOTE_NODE_HAS_INCOMPATIBLE_PROTO_VERSION)
		}
		return nil, domain.WrapErrorWithCode(err, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE)
	}

	acc := &model.Account{Id: req.Id}
	spaceID := app.MustComponent[space.Service](s.app).AccountId()
	acc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx, spaceID)
	return acc, nil
}
