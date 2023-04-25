package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gogo/status"
	cp "github.com/otiai10/copy"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/treegetter"

	"github.com/anytypeio/go-anytype-middleware/core/account"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	walletComp "github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-anytype-middleware/util/files"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// we cannot check the constant error from badger because they hardcoded it there
const errSubstringMultipleAnytypeInstance = "Cannot acquire directory lock"

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

	pubk, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeDevice, cfg.CafePeerId)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode cafe pubkey: %s", err.Error())
	}

	signature, err := base64.RawStdEncoding.DecodeString(respJson.Signature)
	if err != nil {
		return pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to decode cafe signature: %s", err.Error())
	}

	valid, err := pubk.Verify([]byte(code+account), signature)
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
	var deviceId string
	deviceKey, err := wallet.GetDevicePrivkey()
	if err == nil {
		deviceId = deviceKey.Address()
	}

	if gwAddr != "" {
		gwAddr = "http://" + gwAddr
	}

	cfg := config.ConfigRequired{}
	files.GetFileConfig(filepath.Join(wallet.RepoPath(), config.ConfigFileName), &cfg)
	if cfg.IPFSStorageAddr == "" {
		cfg.IPFSStorageAddr = wallet.RepoPath()
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
		LocalStoragePath:       cfg.IPFSStorageAddr,
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

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1")
	index := len(mw.foundAccounts)
	var account wallet.Keypair
	for {
		var err error
		account, err = core.WalletAccountAt(mw.mnemonic, index, "")
		if err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
		}
		path := filepath.Join(mw.rootPath, account.Address())
		// additional check if we found the repo already exists on local disk
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}

		log.Warnf("Account already exists locally, but doesn't exist in the foundAccounts list")
		index++
		continue
	}

	if code, err := checkInviteCode(cfg, req.AlphaInviteCode, account.Address()); err != nil {
		return response(nil, code, err)
	}

	seedRaw, err := account.Raw()
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	if err = core.WalletInitRepo(mw.rootPath, seedRaw); err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	if req.StorePath != "" && req.StorePath != mw.rootPath {
		configPath := filepath.Join(mw.rootPath, account.Address(), config.ConfigFileName)

		storePath := filepath.Join(req.StorePath, account.Address())

		err := os.MkdirAll(storePath, 0700)
		if err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}

		if err := files.WriteJsonConfig(configPath, config.ConfigRequired{IPFSStorageAddr: storePath}); err != nil {
			return response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG, err)
		}
	}

	newAcc := &model.Account{Id: account.Address()}

	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, account.Address()),
		mw.EventSender,
	}

	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create"), comps...); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	coreService := mw.app.MustComponent(core.CName).(core.Service)
	newAcc.Name = req.Name
	bs := mw.app.MustComponent(block.CName).(*block.Service)
	details := []*pb.RpcObjectSetDetailsDetail{{Key: "name", Value: pbtypes.String(req.Name)}}
	if req.GetAvatarLocalPath() != "" {
		hash, err := bs.UploadFile(pb.RpcFileUploadRequest{
			LocalPath: req.GetAvatarLocalPath(),
			Type:      model.BlockContentFile_Image,
		})
		if err != nil {
			log.Warnf("can't add avatar: %v", err)
		} else {
			newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{Image: &model.BlockContentFile{Hash: hash}}}
			details = append(details, &pb.RpcObjectSetDetailsDetail{
				Key:   "iconImage",
				Value: pbtypes.String(hash),
			})
		}
	}
	newAcc.Info = mw.getInfo()

	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   details,
	}); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	mw.foundAccounts = append(mw.foundAccounts, newAcc)
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

	accounts, err := mw.getDerivedAccountsForMnemonic(1)
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_BAD_INPUT, err)
	}
	zeroAccount := accounts[0]
	sendAccountAddEvent(0, &model.Account{Id: zeroAccount.Address(), Name: ""})
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
	if mw.app != nil && req.Id == mw.app.MustComponent(core.CName).(core.Service).Account() {
		mw.app.MustComponent(treegetter.CName).(*block.Service).CloseBlocks()
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

	var repoWasMissing bool
	if _, err := os.Stat(filepath.Join(mw.rootPath, req.Id)); os.IsNotExist(err) {
		if mw.mnemonic == "" {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET, err)
		}
		repoWasMissing = true

		var account wallet.Keypair
		for i := 0; i < 100; i++ {
			account, err = core.WalletAccountAt(mw.mnemonic, i, "")
			if err != nil {
				return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
			}
			if account.Address() == req.Id {
				break
			}
		}

		seedRaw, err := account.Raw()
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
		}

		if err = core.WalletInitRepo(mw.rootPath, seedRaw); err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
	}

	comps := []app.Component{
		anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1"),
		anytype.BootstrapWallet(mw.rootPath, req.Id),
		mw.EventSender,
	}
	var err error

	request := "account_select"
	if repoWasMissing {
		// if we have created the repo, we need to highlight that we are recovering the account
		request = request + "_recover"
	}
	if mw.app, err = anytype.StartNewApp(context.WithValue(context.Background(), metrics.CtxKeyRequest, request), comps...); err != nil {
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
	if fileConf.IPFSStorageAddr != "" {
		srcPath = fileConf.IPFSStorageAddr
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

	err = files.WriteJsonConfig(configPath, config.ConfigRequired{IPFSStorageAddr: destination})
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
	err := mw.doAccountService(func(a account.Service) (err error) {
		resp, err := a.DeleteAccount(context.Background(), req.Revert)
		if resp.GetStatus() != nil {
			st = &model.AccountStatus{
				StatusType:   model.AccountStatusType(resp.Status.Status),
				DeletionDate: resp.Status.DeletionDate,
			}
		}
		return
	})

	mw.refetch()

	if err != nil {
		// TODO: maybe this logic should be in a.DeleteAccount
		code := pb.RpcAccountDeleteResponseError_UNKNOWN_ERROR
		st, ok := status.FromError(err)
		if ok {
			for _, detail := range st.Details() {
				if at, ok := detail.(*cafePb.ErrorAttachment); ok {
					switch at.Code {
					case cafePb.ErrorCodes_AccountIsDeleted:
						code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
					// this code is returned if we call revert but an account is active
					case cafePb.ErrorCodes_AccountIsActive:
						code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ACTIVE
					default:
						break
					}
				}
			}
		}
		return response(nil, code, err)
	}

	return response(st, pb.RpcAccountDeleteResponseError_NULL, nil)
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
	cfg.IPFSStorageAddr = req.IPFSStorageAddr
	err := files.WriteJsonConfig(conf.GetConfigPath(), cfg)
	if err != nil {
		return response(pb.RpcAccountConfigUpdateResponseError_FAILED_TO_WRITE_CONFIG, err)
	}

	return response(pb.RpcAccountConfigUpdateResponseError_NULL, err)
}

func (mw *Middleware) AccountRemoveLocalData() error {
	conf := mw.app.MustComponent(config.CName).(*config.Config)
	address := mw.app.MustComponent(core.CName).(core.Service).Account()

	configPath := conf.GetConfigPath()
	fileConf := config.ConfigRequired{}
	if err := files.GetFileConfig(configPath, &fileConf); err != nil {
		return err
	}

	err := mw.stop()
	if err != nil {
		return err
	}

	if fileConf.IPFSStorageAddr != "" {
		if err2 := os.RemoveAll(fileConf.IPFSStorageAddr); err2 != nil {
			return err2
		}
	}

	err = os.RemoveAll(filepath.Join(mw.rootPath, address))
	if err != nil {
		return err
	}

	return nil
}

func (mw *Middleware) getDerivedAccountsForMnemonic(count int) ([]wallet.Keypair, error) {
	var firstAccounts = make([]wallet.Keypair, count)
	for i := 0; i < count; i++ {
		keypair, err := core.WalletAccountAt(mw.mnemonic, i, "")
		if err != nil {
			return nil, err
		}

		firstAccounts[i] = keypair
	}

	return firstAccounts, nil
}

func (mw *Middleware) isAccountExistsOnDisk(account string) bool {
	if _, err := os.Stat(filepath.Join(mw.rootPath, account)); err == nil {
		return true
	}
	return false
}

func keypairsToAddresses(keypairs []wallet.Keypair) []string {
	var addresses = make([]string, len(keypairs))
	for i, keypair := range keypairs {
		addresses[i] = keypair.Address()
	}
	return addresses
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
