package core

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/anyproto/any-sync/util/crypto"
	cp "github.com/otiai10/copy"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/core/filestorage"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// we cannot check the constant error from badger because they hardcoded it there
const errSubstringMultipleAnytypeInstance = "Cannot acquire directory lock"

func (mw *Middleware) refreshRemoteAccountState() {
	fetcher := mw.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	fetcher.Refetch()
}

func (mw *Middleware) getAnalyticsId(bs *block.Service, accountId string) (string, error) {
	conf := mw.app.MustComponent(config.CName).(*config.Config)
	if conf.AnalyticsId != "" {
		return conf.AnalyticsId, nil
	}
	var analyticsId string
	sb, err := bs.PickBlock(context.Background(), accountId)
	if err != nil {
		return "", err
	}
	s := sb.NewState().GetSetting(state.SettingsAnalyticsId)
	if s == nil {
		log.Errorf("analytics id not found")
	} else {
		analyticsId = s.GetStringValue()
	}

	return analyticsId, err
}

func (mw *Middleware) getInfo(bs *block.Service) *model.AccountInfo {
	at := mw.app.MustComponent(core.CName).(core.Service)
	gwAddr := mw.app.MustComponent(gateway.CName).(gateway.Gateway).Addr()
	wallet := mw.app.MustComponent(walletComp.CName).(walletComp.Wallet)
	deviceKey := wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().Account()

	analyticsId, err := mw.getAnalyticsId(bs, at.PredefinedBlocks().Account)
	if err != nil {
		log.Errorf("failed to get analytics id: %s", err.Error())
	}

	if gwAddr != "" {
		gwAddr = "http://" + gwAddr
	}

	cfg := config.ConfigRequired{}
	err = config.GetFileConfig(filepath.Join(wallet.RepoPath(), config.ConfigFileName), &cfg)
	if err != nil || cfg.CustomFileStorePath == "" {
		cfg.CustomFileStorePath = wallet.RepoPath()
	}

	pBlocks := at.PredefinedBlocks()
	return &model.AccountInfo{
		HomeObjectId:           pBlocks.Home,
		ArchiveObjectId:        pBlocks.Archive,
		ProfileObjectId:        pBlocks.Profile,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		AccountSpaceId:         pBlocks.Account,
		WidgetsId:              pBlocks.Widgets,
		GatewayUrl:             gwAddr,
		DeviceId:               deviceId,
		LocalStoragePath:       cfg.CustomFileStorePath,
		TimeZone:               cfg.TimeZone,
		AnalyticsId:            analyticsId,
	}
}

func (mw *Middleware) AccountCreate(cctx context.Context, req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	mw.accountSearchCancel()
	mw.m.Lock()

	defer mw.m.Unlock()
	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		var clientConfig *pb.RpcAccountConfig
		m := &pb.RpcAccountCreateResponse{Config: clientConfig, Account: account, Error: &pb.RpcAccountCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if err := mw.stop(); err != nil {
		response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE, err)
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", true)

	derivationResult, err := core.WalletAccountAt(mw.mnemonic, 0)
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}
	address := derivationResult.Identity.GetPublic().Account()

	if err = core.WalletInitRepo(mw.rootPath, derivationResult.Identity); err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}
	if req.StorePath != "" && req.StorePath != mw.rootPath {
		configPath := filepath.Join(mw.rootPath, address, config.ConfigFileName)
		storePath := filepath.Join(req.StorePath, address)
		err := os.MkdirAll(storePath, 0700)
		if err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
		if err := config.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: storePath}); err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG, err)
		}
	}

	newAcc := &model.Account{Id: address}

	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, derivationResult),
		mw.EventSender,
	}

	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create"), mw.clientVersion, comps...); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	bs := mw.app.MustComponent(block.CName).(*block.Service)
	commonDetails := []*pb.RpcObjectSetDetailsDetail{
		{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(req.Name),
		},
		{
			Key:   bundle.RelationKeyIconOption.String(),
			Value: pbtypes.Int64(req.Icon),
		},
	}
	profileDetails := make([]*pb.RpcObjectSetDetailsDetail, 0)
	profileDetails = append(profileDetails, commonDetails...)

	if req.GetAvatarLocalPath() != "" {
		hash, err := bs.UploadFile(pb.RpcFileUploadRequest{
			LocalPath: req.GetAvatarLocalPath(),
			Type:      model.BlockContentFile_Image,
		})
		if err != nil {
			log.Warnf("can't add avatar: %v", err)
		} else {
			newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{Image: &model.BlockContentFile{Hash: hash}}}
			profileDetails = append(profileDetails, &pb.RpcObjectSetDetailsDetail{
				Key:   bundle.RelationKeyIconImage.String(),
				Value: pbtypes.String(hash),
			})
		}
	}

	newAcc.Name = req.Name
	newAcc.Info = mw.getInfo(bs)

	coreService := mw.app.MustComponent(core.CName).(core.Service)
	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   profileDetails,
	}); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Account,
		Details:   commonDetails,
	}); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	return response(newAcc, pb.RpcAccountCreateResponseError_NULL, nil)
}

func (mw *Middleware) AccountRecover(cctx context.Context, _ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(code pb.RpcAccountRecoverResponseErrorCode, err error) *pb.RpcAccountRecoverResponse {
		m := &pb.RpcAccountRecoverResponse{Error: &pb.RpcAccountRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	sendAccountAddEvent := func(index int, account *model.Account) {
		m := &pb.Event{Messages: []*pb.EventMessage{{&pb.EventMessageValueOfAccountShow{AccountShow: &pb.EventAccountShow{Index: int32(index), Account: account}}}}}
		mw.EventSender.Send(m)
	}

	if mw.mnemonic == "" {
		return response(pb.RpcAccountRecoverResponseError_NEED_TO_RECOVER_WALLET_FIRST, nil)
	}

	res, err := core.WalletAccountAt(mw.mnemonic, 0)
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_BAD_INPUT, err)
	}
	sendAccountAddEvent(0, &model.Account{Id: res.Identity.GetPublic().Account(), Name: ""})
	return response(pb.RpcAccountRecoverResponseError_NULL, nil)
}

func (mw *Middleware) AccountSelect(cctx context.Context, req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		var clientConfig *pb.RpcAccountConfig
		m := &pb.RpcAccountSelectResponse{Config: clientConfig, Account: account, Error: &pb.RpcAccountSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if req.Id == "" {
		return response(&model.Account{Id: req.Id}, pb.RpcAccountSelectResponseError_BAD_INPUT, fmt.Errorf("account id is empty"))
	}

	// cancel pending account searches and it will release the mutex
	mw.accountSearchCancel()

	mw.m.Lock()
	defer mw.m.Unlock()

	// we already have this account running, lets just stop events
	if mw.app != nil && req.Id == mw.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account() {
		bs := mw.app.MustComponent(treemanager.CName).(*block.Service)
		bs.CloseBlocks()
		acc := &model.Account{Id: req.Id}
		acc.Info = mw.getInfo(bs)
		return response(acc, pb.RpcAccountSelectResponseError_NULL, nil)
	}

	// in case user selected account other than the first one(used to perform search)
	// or this is the first time in this session we run the Anytype node
	if err := mw.stop(); err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE, err)
	}
	if req.RootPath != "" {
		mw.rootPath = req.RootPath
	}
	if mw.mnemonic == "" {
		return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET, fmt.Errorf("no mnemonic provided"))
	}
	res, err := core.WalletAccountAt(mw.mnemonic, 0)
	if err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
	}
	var repoWasMissing bool
	if _, err := os.Stat(filepath.Join(mw.rootPath, req.Id)); os.IsNotExist(err) {
		repoWasMissing = true
		if err = core.WalletInitRepo(mw.rootPath, res.Identity); err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
	}

	comps := []app.Component{
		anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1", false),
		anytype.BootstrapWallet(mw.rootPath, res),
		mw.EventSender,
	}

	request := "account_select"
	if repoWasMissing {
		// if we have created the repo, we need to highlight that we are recovering the account
		request = request + "_recover"
	}
	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, request), mw.clientVersion, comps...); err != nil {
		if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}
		if strings.Contains(err.Error(), errSubstringMultipleAnytypeInstance) {
			return response(nil, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING, err)
		}
		if errors.Is(err, handshake.ErrIncompatibleVersion) {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_FETCH_REMOTE_NODE_HAS_INCOMPATIBLE_PROTO_VERSION, fmt.Errorf("can't fetch account's data because remote nodes have incompatible protocol version. Please update anytype to the latest version"))
		}
		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE, err)
	}

	acc := &model.Account{Id: req.Id}
	acc.Info = mw.getInfo(mw.app.MustComponent(block.CName).(*block.Service))
	return response(acc, pb.RpcAccountSelectResponseError_NULL, nil)
}

func (mw *Middleware) AccountStop(cctx context.Context, req *pb.RpcAccountStopRequest) *pb.RpcAccountStopResponse {
	mw.accountSearchCancel()
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(code pb.RpcAccountStopResponseErrorCode, err error) *pb.RpcAccountStopResponse {
		m := &pb.RpcAccountStopResponse{Error: &pb.RpcAccountStopResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		return response(pb.RpcAccountStopResponseError_ACCOUNT_IS_NOT_RUNNING, fmt.Errorf("anytype node not set"))
	}

	if req.RemoveData {
		err := mw.AccountRemoveLocalData()
		if err != nil {
			return response(pb.RpcAccountStopResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA, err)
		}
	} else {
		err := mw.stop()
		if err != nil {
			return response(pb.RpcAccountStopResponseError_FAILED_TO_STOP_NODE, err)
		}
	}

	return response(pb.RpcAccountStopResponseError_NULL, nil)
}

func (mw *Middleware) AccountMove(cctx context.Context, req *pb.RpcAccountMoveRequest) *pb.RpcAccountMoveResponse {
	mw.accountSearchCancel()
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(code pb.RpcAccountMoveResponseErrorCode, err error) *pb.RpcAccountMoveResponse {
		m := &pb.RpcAccountMoveResponse{Error: &pb.RpcAccountMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	removeDirs := func(src string, dirs []string) error {
		for _, dir := range dirs {
			if err := os.RemoveAll(filepath.Join(src, dir)); err != nil {
				return err
			}
		}
		return nil
	}

	dirs := []string{filestorage.FlatfsDirName}
	conf := mw.app.MustComponent(config.CName).(*config.Config)

	configPath := conf.GetConfigPath()
	srcPath := conf.RepoPath
	fileConf := config.ConfigRequired{}
	if err := config.GetFileConfig(configPath, &fileConf); err != nil {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_GET_CONFIG, err)
	}
	if fileConf.CustomFileStorePath != "" {
		srcPath = fileConf.CustomFileStorePath
	}

	parts := strings.Split(srcPath, string(filepath.Separator))
	accountDir := parts[len(parts)-1]
	if accountDir == "" {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_IDENTIFY_ACCOUNT_DIR, errors.New("fail to identify account dir"))
	}

	destination := filepath.Join(req.NewPath, accountDir)
	if srcPath == destination {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_CREATE_LOCAL_REPO, errors.New("source path should not be equal destination path"))
	}

	if _, err := os.Stat(destination); !os.IsNotExist(err) { // if already exist (in case of the previous fail moving)
		if err := removeDirs(destination, dirs); err != nil {
			return response(pb.RpcAccountMoveResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA, err)
		}
	}

	err := os.MkdirAll(destination, 0700)
	if err != nil {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	err = mw.stop()
	if err != nil {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_STOP_NODE, err)
	}

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(srcPath, dir)); !os.IsNotExist(err) { // copy only if exist such dir
			if err := cp.Copy(filepath.Join(srcPath, dir), filepath.Join(destination, dir), cp.Options{PreserveOwner: true}); err != nil {
				return response(pb.RpcAccountMoveResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
			}
		}
	}

	err = config.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: destination})
	if err != nil {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_WRITE_CONFIG, err)
	}

	if err := removeDirs(srcPath, dirs); err != nil {
		return response(pb.RpcAccountMoveResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA, err)
	}

	if srcPath != conf.RepoPath { // remove root account dir, if move not from anytype source dir
		if err := os.RemoveAll(srcPath); err != nil {
			return response(pb.RpcAccountMoveResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA, err)
		}
	}

	return response(pb.RpcAccountMoveResponseError_NULL, nil)
}

func (mw *Middleware) AccountDelete(cctx context.Context, req *pb.RpcAccountDeleteRequest) *pb.RpcAccountDeleteResponse {
	response := func(status *model.AccountStatus, code pb.RpcAccountDeleteResponseErrorCode, err error) *pb.RpcAccountDeleteResponse {
		m := &pb.RpcAccountDeleteResponse{Error: &pb.RpcAccountDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Status = status
		}

		return m
	}

	var st *model.AccountStatus
	err := mw.doAccountService(func(a space.Service) (err error) {
		resp, err := a.DeleteAccount(cctx, req.Revert)
		if err != nil {
			return
		}
		st = &model.AccountStatus{
			StatusType:   model.AccountStatusType(resp.Status),
			DeletionDate: resp.DeletionDate.Unix(),
		}
		return
	})

	// so we will receive updated account status
	mw.refreshRemoteAccountState()

	if err == nil {
		return response(st, pb.RpcAccountDeleteResponseError_NULL, nil)
	}
	code := pb.RpcAccountDeleteResponseError_UNKNOWN_ERROR
	switch err {
	case space.ErrSpaceIsDeleted:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceDeletionPending:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceIsCreated:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ACTIVE
	default:
		break
	}
	return response(nil, code, err)
}

func (mw *Middleware) AccountConfigUpdate(_ context.Context, req *pb.RpcAccountConfigUpdateRequest) *pb.RpcAccountConfigUpdateResponse {
	response := func(code pb.RpcAccountConfigUpdateResponseErrorCode, err error) *pb.RpcAccountConfigUpdateResponse {
		m := &pb.RpcAccountConfigUpdateResponse{Error: &pb.RpcAccountConfigUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	if mw.app == nil {
		return response(pb.RpcAccountConfigUpdateResponseError_ACCOUNT_IS_NOT_RUNNING, fmt.Errorf("anytype node not set"))
	}

	conf := mw.app.MustComponent(config.CName).(*config.Config)
	cfg := config.ConfigRequired{}
	cfg.TimeZone = req.TimeZone
	cfg.CustomFileStorePath = req.IPFSStorageAddr
	err := config.WriteJsonConfig(conf.GetConfigPath(), cfg)
	if err != nil {
		return response(pb.RpcAccountConfigUpdateResponseError_FAILED_TO_WRITE_CONFIG, err)
	}

	return response(pb.RpcAccountConfigUpdateResponseError_NULL, err)
}

func (mw *Middleware) AccountRemoveLocalData() error {
	conf := mw.app.MustComponent(config.CName).(*config.Config)
	address := mw.app.MustComponent(walletComp.CName).(walletComp.Wallet).GetAccountPrivkey().GetPublic().Account()

	configPath := conf.GetConfigPath()
	fileConf := config.ConfigRequired{}
	if err := config.GetFileConfig(configPath, &fileConf); err != nil {
		return err
	}

	err := mw.stop()
	if err != nil {
		return err
	}

	if fileConf.CustomFileStorePath != "" {
		if err2 := os.RemoveAll(fileConf.CustomFileStorePath); err2 != nil {
			return err2
		}
	}

	err = os.RemoveAll(filepath.Join(mw.rootPath, address))
	if err != nil {
		return err
	}

	return nil
}

func (mw *Middleware) AccountRecoverFromLegacyExport(cctx context.Context,
	req *pb.RpcAccountRecoverFromLegacyExportRequest) *pb.RpcAccountRecoverFromLegacyExportResponse {
	response := func(address string, code pb.RpcAccountRecoverFromLegacyExportResponseErrorCode, err error) *pb.RpcAccountRecoverFromLegacyExportResponse {
		m := &pb.RpcAccountRecoverFromLegacyExportResponse{AccountId: address, Error: &pb.RpcAccountRecoverFromLegacyExportResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	profile, err := getUserProfile(req)
	if err != nil {
		return response("", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err)
	}
	address, code, err := mw.createAccountFromExport(profile, req)
	if err != nil {
		return response("", code, err)
	}

	return response(address, pb.RpcAccountRecoverFromLegacyExportResponseError_NULL, nil)
}

func getUserProfile(req *pb.RpcAccountRecoverFromLegacyExportRequest) (*pb.Profile, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	f, err := archive.Open(constant.ProfileFile)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var profile pb.Profile

	err = profile.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (mw *Middleware) createAccountFromExport(profile *pb.Profile, req *pb.RpcAccountRecoverFromLegacyExportRequest) (accountId string, code pb.RpcAccountRecoverFromLegacyExportResponseErrorCode, err error) {
	mw.m.Lock()
	defer mw.m.Unlock()
	err = mw.stop()
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	res, err := core.WalletAccountAt(mw.mnemonic, 0)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}
	address := res.Identity.GetPublic().Account()
	if profile.Address != res.OldAccountKey.GetPublic().Account() && profile.Address != address {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_DIFFERENT_ACCOUNT, fmt.Errorf("backup was made from different account")
	}
	mw.rootPath = req.RootPath
	err = os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}
	mw.accountSearchCancel()
	if _, statErr := os.Stat(filepath.Join(mw.rootPath, address)); os.IsNotExist(statErr) {
		if walletErr := core.WalletInitRepo(mw.rootPath, res.Identity); walletErr != nil {
			return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, walletErr
		}
	}
	cfg, err := mw.getBootstrapConfig(req)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	if profile.AnalyticsId != "" {
		cfg.AnalyticsId = profile.AnalyticsId
	} else {
		cfg.AnalyticsId = metrics.GenerateAnalyticsId()
	}

	err = mw.startApp(cfg, res)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	err = mw.setDetails(profile, req.Icon)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	if err = mw.app.MustComponent(builtinobjects.CName).(builtinobjects.BuiltinObjects).InjectMigrationDashboard(); err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_BAD_INPUT, err
	}

	return address, pb.RpcAccountRecoverFromLegacyExportResponseError_NULL, nil
}

func (mw *Middleware) startApp(cfg *config.Config, derivationResult crypto.DerivationResult) error {
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, derivationResult),
		mw.EventSender,
	}

	ctxWithValue := context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create")
	var err error
	if mw.app, err = anytype.StartNewApp(ctxWithValue, mw.clientVersion, comps...); err != nil {
		return err
	}
	return nil
}

func (mw *Middleware) getBootstrapConfig(req *pb.RpcAccountRecoverFromLegacyExportRequest) (*config.Config, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	oldCfg, err := extractConfig(archive)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config: %w", err)
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", false)
	cfg.LegacyFileStorePath = oldCfg.LegacyFileStorePath
	return cfg, nil
}

func (mw *Middleware) setDetails(profile *pb.Profile, icon int64) error {
	profileDetails, accountDetails := buildDetails(profile, icon)
	bs := mw.app.MustComponent(block.CName).(*block.Service)
	coreService := mw.app.MustComponent(core.CName).(core.Service)

	if err := bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   profileDetails,
	}); err != nil {
		return err
	}
	if err := bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Account,
		Details:   accountDetails,
	}); err != nil {
		return err
	}
	return nil
}

func buildDetails(profile *pb.Profile, icon int64) (
	profileDetails []*pb.RpcObjectSetDetailsDetail, accountDetails []*pb.RpcObjectSetDetailsDetail,
) {
	profileDetails = []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(profile.Name),
	}}
	if profile.Avatar == "" {
		profileDetails = append(profileDetails, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIconOption.String(),
			Value: pbtypes.Int64(icon),
		})
	} else {
		profileDetails = append(profileDetails, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIconImage.String(),
			Value: pbtypes.String(profile.Avatar),
		})
	}
	accountDetails = []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyIconOption.String(),
		Value: pbtypes.Int64(icon),
	}}
	return
}

func extractConfig(archive *zip.ReadCloser) (*config.Config, error) {
	for _, f := range archive.File {
		if f.Name == config.ConfigFileName {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}

			var conf config.Config
			err = json.NewDecoder(r).Decode(&conf)
			if err != nil {
				return nil, err
			}
			return &conf, nil
		}
	}
	return nil, fmt.Errorf("config.json not found in archive")
}

func (mw *Middleware) isAccountExistsOnDisk(account string) bool {
	if _, err := os.Stat(filepath.Join(mw.rootPath, account)); err == nil {
		return true
	}
	return false
}
