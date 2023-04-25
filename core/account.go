package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anytypeio/any-sync/util/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/treemanager"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	cp "github.com/otiai10/copy"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	walletComp "github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/util/files"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// we cannot check the constant error from badger because they hardcoded it there
const errSubstringMultipleAnytypeInstance = "Cannot acquire directory lock"
const profileFile = "profile"

type AlphaInviteRequest struct {
	Code    string `json:"code"`
	Account string `json:"account"`
}

type AlphaInviteResponse struct {
	Signature string `json:"signature"`
}

type AlphaInviteErrorResponse struct {
	Error string `json:"error"`
}

func checkInviteCode(cfg *config.Config, code string, account string) (errorCode pb.RpcAccountCreateResponseErrorCode, err error) {
	if code == "" {
		return pb.RpcAccountCreateResponseError_BAD_INPUT, fmt.Errorf("invite code is empty")
	}

	jsonStr, err := json.Marshal(AlphaInviteRequest{
		Code:    code,
		Account: account,
	})

	// TODO: here we always using the default cafe address, because we want to check invite code only on our server
	// this code should be removed with a public release
	req, err := http.NewRequest("POST", cfg.CafeUrl()+"/alpha-invite", bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	checkNetError := func(err error) (netOpError bool, dnsError bool, offline bool) {
		if err == nil {
			return false, false, false
		}
		if netErr, ok := err.(*net.OpError); ok {
			if syscallErr, ok := netErr.Err.(*os.SyscallError); ok {
				if syscallErr.Err == syscall.ENETDOWN || syscallErr.Err == syscall.ENETUNREACH {
					return true, false, true
				}
			}
			if _, ok := netErr.Err.(*net.DNSError); ok {
				return true, true, false
			}
			return true, false, false
		}
		return false, false, false
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		var netOpErr, dnsError bool
		if urlErr, ok := err.(*url.Error); ok {
			var offline bool
			if netOpErr, dnsError, offline = checkNetError(urlErr.Err); offline {
				return pb.RpcAccountCreateResponseError_NET_OFFLINE, err
			}
		}
		if dnsError {
			// we can receive DNS error in case device is offline, lets check the SHOULD-BE-ALWAYS-ONLINE OpenDNS IP address on the 80 port
			c, err2 := net.DialTimeout("tcp", "1.1.1.1:80", time.Second*5)
			if c != nil {
				_ = c.Close()
			}
			_, _, offline := checkNetError(err2)
			if offline {
				return pb.RpcAccountCreateResponseError_NET_OFFLINE, err
			}
		}

		if netOpErr {
			return pb.RpcAccountCreateResponseError_NET_CONNECTION_REFUSED, err
		}

		return pb.RpcAccountCreateResponseError_NET_ERROR, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return pb.RpcAccountCreateResponseError_NET_ERROR, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode != 200 {
		respJson := AlphaInviteErrorResponse{}
		err = json.Unmarshal(body, &respJson)
		return pb.RpcAccountCreateResponseError_BAD_INVITE_CODE, fmt.Errorf(respJson.Error)
	}

	respJson := AlphaInviteResponse{}
	err = json.Unmarshal(body, &respJson)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode response json: %s", err.Error())
	}
	peerID, err := peer.Decode(cfg.CafePeerId)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode cafe pubkey: %s", err.Error())
	}
	pk, err := peerID.ExtractPublicKey()
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode cafe pubkey: %s", err.Error())
	}
	signature, err := base64.RawStdEncoding.DecodeString(respJson.Signature)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode cafe signature: %s", err.Error())
	}
	valid, err := pk.Verify([]byte(code+account), signature)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to verify cafe signature: %s", err.Error())
	}
	if !valid {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("invalid signature")
	}

	return pb.RpcAccountCreateResponseError_NULL, nil
}

func (mw *Middleware) getCafeAccount() *cafePb.AccountState {
	fetcher := mw.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)

	return fetcher.GetAccountState()
}

func (mw *Middleware) refetch() {
	fetcher := mw.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)

	fetcher.Refetch()
}

func (mw *Middleware) getInfo() *model.AccountInfo {
	at := mw.app.MustComponent(core.CName).(core.Service)
	gwAddr := mw.app.MustComponent(gateway.CName).(gateway.Gateway).Addr()
	wallet := mw.app.MustComponent(walletComp.CName).(walletComp.Wallet)
	deviceKey := wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().Account()

	if gwAddr != "" {
		gwAddr = "http://" + gwAddr
	}

	cfg := config.ConfigRequired{}
	err := files.GetFileConfig(filepath.Join(wallet.RepoPath(), config.ConfigFileName), &cfg)
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
	}
}

func (mw *Middleware) AccountCreate(cctx context.Context, req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	mw.accountSearchCancel()
	mw.m.Lock()

	defer mw.m.Unlock()
	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		var clientConfig *pb.RpcAccountConfig
		if account != nil && err == nil {
			cafeAccount := mw.getCafeAccount()

			clientConfig = convertToRpcAccountConfig(cafeAccount.Config) // to support deprecated clients
			enrichWithCafeAccount(account, cafeAccount)
		}
		m := &pb.RpcAccountCreateResponse{Config: clientConfig, Account: account, Error: &pb.RpcAccountCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if err := mw.stop(); err != nil {
		response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE, err)
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", true, true)
	derivationResult, err := core.WalletAccountAt(mw.mnemonic, 0)
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}
	address := derivationResult.Identity.GetPublic().Account()
	if code, err := checkInviteCode(cfg, req.AlphaInviteCode, address); err != nil {
		return response(nil, code, err)
	}

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
		if err := files.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: storePath}); err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG, err)
		}
	}

	newAcc := &model.Account{Id: address}

	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, derivationResult),
		mw.EventSender,
	}

	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create"), comps...); err != nil {
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
	newAcc.Info = mw.getInfo()

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
		if account != nil {
			cafeAccount := mw.getCafeAccount()

			clientConfig = convertToRpcAccountConfig(cafeAccount.Config) // to support deprecated clients
			enrichWithCafeAccount(account, cafeAccount)
		}
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
		mw.app.MustComponent(treemanager.CName).(*block.Service).CloseBlocks()
		acc := &model.Account{Id: req.Id}
		acc.Info = mw.getInfo()
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
		anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1", false, false),
		anytype.BootstrapWallet(mw.rootPath, res),
		mw.EventSender,
	}

	request := "account_select"
	if repoWasMissing {
		// if we have created the repo, we need to highlight that we are recovering the account
		request = request + "_recover"
	}
	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, request), comps...); err != nil {
		if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}
		if strings.Contains(err.Error(), errSubstringMultipleAnytypeInstance) {
			return response(nil, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING, err)
		}

		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE, err)
	}

	acc := &model.Account{Id: req.Id}
	acc.Info = mw.getInfo()
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
	if err := files.GetFileConfig(configPath, &fileConf); err != nil {
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

	err = files.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: destination})
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

	mw.refetch()
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
	err := files.WriteJsonConfig(conf.GetConfigPath(), cfg)
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
	if err := files.GetFileConfig(configPath, &fileConf); err != nil {
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

	f, err := archive.Open(profileFile)
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
	cfg, err := mw.getBootstrapConfig(err, req)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	err = mw.startApp(cfg, res, err)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	err = mw.setDetails(profile, req.Icon, err)
	if err != nil {
		return "", pb.RpcAccountRecoverFromLegacyExportResponseError_UNKNOWN_ERROR, err
	}

	return address, pb.RpcAccountRecoverFromLegacyExportResponseError_NULL, nil
}

func (mw *Middleware) startApp(cfg *config.Config, derivationResult crypto.DerivationResult, err error) error {
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, derivationResult),
		mw.EventSender,
	}

	ctxWithValue := context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create")
	if mw.app, err = anytype.StartNewApp(ctxWithValue, comps...); err != nil {
		return err
	}
	return nil
}

func (mw *Middleware) getBootstrapConfig(err error, req *pb.RpcAccountRecoverFromLegacyExportRequest) (*config.Config, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	oldCfg, err := extractConfig(archive)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config: %w", err)
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", false, false)
	cfg.LegacyFileStorePath = oldCfg.LegacyFileStorePath
	return cfg, nil
}

func (mw *Middleware) setDetails(profile *pb.Profile, icon int64, err error) error {
	profileDetails, accountDetails := buildDetails(profile, icon)
	bs := mw.app.MustComponent(block.CName).(*block.Service)
	coreService := mw.app.MustComponent(core.CName).(core.Service)

	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   profileDetails,
	}); err != nil {
		return err
	}
	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
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
	}, {
		Key:   bundle.RelationKeyIconImage.String(),
		Value: pbtypes.String(profile.Avatar),
	}}
	if profile.Avatar == "" {
		profileDetails = append(profileDetails, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIconOption.String(),
			Value: pbtypes.Int64(icon),
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

func convertToRpcAccountConfig(cfg *cafePb.Config) *pb.RpcAccountConfig {
	return &pb.RpcAccountConfig{
		EnableDataview:          cfg.EnableDataview,
		EnableDebug:             cfg.EnableDebug,
		EnablePrereleaseChannel: cfg.EnablePrereleaseChannel,
		Extra:                   cfg.Extra,
		EnableSpaces:            cfg.EnableSpaces,
	}
}

func enrichWithCafeAccount(acc *model.Account, cafeAcc *cafePb.AccountState) {
	cfg := cafeAcc.Config
	acc.Config = &model.AccountConfig{
		EnableDataview:          cfg.EnableDataview,
		EnableDebug:             cfg.EnableDebug,
		EnablePrereleaseChannel: cfg.EnablePrereleaseChannel,
		Extra:                   cfg.Extra,
		EnableSpaces:            cfg.EnableSpaces,
	}

	st := cafeAcc.Status
	acc.Status = &model.AccountStatus{
		StatusType:   model.AccountStatusType(st.Status),
		DeletionDate: st.DeletionDate,
	}
}
