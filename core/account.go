package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/profilefinder"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const defaultCafeUrl = "https://cafe1.anytype.io"
const defaultCafePeerId = "12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"

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

func checkInviteCode(code string, account string) error {
	if code == "" {
		return fmt.Errorf("invite code is empty")
	}

	jsonStr, err := json.Marshal(AlphaInviteRequest{
		Code:    code,
		Account: account,
	})

	// TODO: here we always using the default cafe address, because we want to check invite code only on our server
	// this code should be removed with a public release
	req, err := http.NewRequest("POST", defaultCafeUrl+"/alpha-invite", bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to access cafe server: %s", err.Error())
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode != 200 {
		respJson := AlphaInviteErrorResponse{}
		err = json.Unmarshal(body, &respJson)
		return fmt.Errorf(respJson.Error)
	}

	respJson := AlphaInviteResponse{}
	err = json.Unmarshal(body, &respJson)
	if err != nil {
		return fmt.Errorf("failed to decode response json: %s", err.Error())
	}

	pubk, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeDevice, defaultCafePeerId)
	if err != nil {
		return fmt.Errorf("failed to decode cafe pubkey: %s", err.Error())
	}

	signature, err := base64.RawStdEncoding.DecodeString(respJson.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode cafe signature: %s", err.Error())
	}

	valid, err := pubk.Verify([]byte(code+account), signature)
	if err != nil {
		return fmt.Errorf("failed to verify cafe signature: %s", err.Error())
	}

	if !valid {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (mw *Middleware) getAccountConfig() *pb.RpcAccountConfig {
	fetcher := mw.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cfg := fetcher.GetAccountConfig(ctx)

	// TODO: change proto defs to use same model from "models.proto" and not from "api.proto"
	return &pb.RpcAccountConfig{
		EnableDataview:             cfg.EnableDataview,
		EnableDebug:                cfg.EnableDebug,
		EnableReleaseChannelSwitch: cfg.EnableReleaseChannelSwitch,
		Extra:                      cfg.Extra,
		EnableSpaces:               cfg.EnableSpaces,
	}
}

func (mw *Middleware) AccountCreate(req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	mw.accountSearchCancel()
	mw.m.Lock()

	defer mw.m.Unlock()
	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		var clientConfig *pb.RpcAccountConfig
		if account != nil {
			clientConfig = mw.getAccountConfig()
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

	if err := checkInviteCode(req.AlphaInviteCode, account.Address()); err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_BAD_INVITE_CODE, err)
	}

	seedRaw, err := account.Raw()
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	if err = core.WalletInitRepo(mw.rootPath, seedRaw); err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	newAcc := &model.Account{Id: account.Address()}

	comps, err := anytype.BootstrapConfigAndWallet(true, mw.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	comps = append(comps, mw.EventSender)
	if mw.app, err = anytype.StartNewApp(comps...); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	coreService := mw.app.MustComponent(core.CName).(core.Service)
	newAcc.Name = req.Name
	bs := mw.app.MustComponent(block.CName).(block.Service)
	details := []*pb.RpcBlockSetDetailsDetail{{Key: "name", Value: pbtypes.String(req.Name)}}
	if req.GetAvatarLocalPath() != "" {
		hash, err := bs.UploadFile(pb.RpcUploadFileRequest{
			LocalPath:         req.GetAvatarLocalPath(),
			Type:              model.BlockContentFile_Image,
			DisableEncryption: true,
		})
		if err != nil {
			log.Warnf("can't add avatar: %v", err)
		} else {
			newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{Image: &model.BlockContentFile{Hash: hash}}}
			details = append(details, &pb.RpcBlockSetDetailsDetail{
				Key:   "iconImage",
				Value: pbtypes.String(hash),
			})
		}
	} else if req.GetAvatarColor() != "" {
		details = append(details, &pb.RpcBlockSetDetailsDetail{
			Key:   "iconColor",
			Value: pbtypes.String(req.GetAvatarColor()),
		})
	}

	if err = bs.SetDetails(nil, pb.RpcBlockSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   details,
	}); err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	mw.foundAccounts = append(mw.foundAccounts, newAcc)
	return response(newAcc, pb.RpcAccountCreateResponseError_NULL, nil)
}

func (mw *Middleware) AccountRecover(_ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(code pb.RpcAccountRecoverResponseErrorCode, err error) *pb.RpcAccountRecoverResponse {
		m := &pb.RpcAccountRecoverResponse{Error: &pb.RpcAccountRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var sentAccountsMutex sync.RWMutex
	var sentAccounts = make(map[string]struct{})
	sendAccountAddEvent := func(index int, account *model.Account) {
		sentAccountsMutex.Lock()
		defer sentAccountsMutex.Unlock()
		if _, exists := sentAccounts[account.Id]; exists {
			return
		}

		sentAccounts[account.Id] = struct{}{}
		m := &pb.Event{Messages: []*pb.EventMessage{{&pb.EventMessageValueOfAccountShow{AccountShow: &pb.EventAccountShow{Index: int32(index), Account: account}}}}}
		mw.EventSender.Send(m)
	}

	if mw.mnemonic == "" {
		return response(pb.RpcAccountRecoverResponseError_NEED_TO_RECOVER_WALLET_FIRST, nil)
	}

	var remoteAccountsProceed = make(chan struct{})
	// todo: this case temporarily commented-out because we only have 1 acc per mnemonic
	/*for index := 0; index < len(mw.foundAccounts); index++ {
		// in case we returned to the account choose screen we can use cached accounts
		sendAccountAddEvent(index, mw.foundAccounts[index])
	}*/

	accounts, err := mw.getDerivedAccountsForMnemonic(10)
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_BAD_INPUT, err)
	}

	// todo: this case temporarily prioritized because we only have 1 acc per mnemonic
	for i, acc := range accounts {
		if !mw.isAccountExistsOnDisk(acc.Address()) {
			continue
		}
		// todo: load profile name from the details cache in badger
		sendAccountAddEvent(i, &model.Account{Id: acc.Address(), Name: ""})
		return response(pb.RpcAccountRecoverResponseError_NULL, nil)
	}

	zeroAccount := accounts[0]

	// stop current account
	if err := mw.stop(); err != nil {
		return response(pb.RpcAccountRecoverResponseError_FAILED_TO_STOP_RUNNING_NODE, err)
	}

	if mw.app, err = anytype.StartAccountRecoverApp(mw.EventSender, zeroAccount); err != nil {
		return response(pb.RpcAccountRecoverResponseError_FAILED_TO_RUN_NODE, err)
	}

	profileFinder := mw.app.MustComponent(profilefinder.CName).(profilefinder.Service)
	recoveryFinished := make(chan struct{})
	defer close(recoveryFinished)

	ctx, searchQueryCancel := context.WithTimeout(context.Background(), time.Second*30)
	mw.accountSearchCancel = func() { searchQueryCancel() }
	defer searchQueryCancel()

	profilesCh := make(chan core.Profile)
	go func() {
		defer func() {
			close(remoteAccountsProceed)
		}()
		for {
			select {
			case profile, ok := <-profilesCh:
				if !ok {
					return
				}

				log.Infof("remote recovery got %+v", profile)
				var avatar *model.AccountAvatar
				if profile.IconImage != "" {
					avatar = &model.AccountAvatar{
						Avatar: &model.AccountAvatarAvatarOfImage{
							Image: &model.BlockContentFile{Hash: profile.IconImage},
						},
					}
				} else if profile.IconColor != "" {
					avatar = &model.AccountAvatar{
						Avatar: &model.AccountAvatarAvatarOfColor{
							Color: profile.IconColor,
						},
					}
				}

				var index int
				for i, account := range accounts {
					if account.Address() == profile.AccountAddr {
						index = i
						break
					}
				}

				account := &model.Account{
					Id:     profile.AccountAddr,
					Name:   profile.Name,
					Avatar: avatar,
				}

				var alreadyExists bool
				for _, foundAccount := range mw.foundAccounts {
					if foundAccount.Id == account.Id {
						alreadyExists = true
					}
				}
				if !alreadyExists {
					mw.foundAccounts = append(mw.foundAccounts, account)
				}

				sendAccountAddEvent(index, account)
			}
		}
	}()

	findProfilesErr := profileFinder.FindProfilesByAccountIDs(ctx, keypairsToAddresses(accounts), profilesCh)
	if findProfilesErr != nil {
		log.Errorf("remote profiles request failed: %s", findProfilesErr.Error())
	}

	// wait until we finish to read profiles from chan and process them in case request was successful
	<-remoteAccountsProceed

	sentAccountsMutex.Lock()
	defer sentAccountsMutex.Unlock()

	err = mw.stop()
	if err != nil {
		log.Error("failed to stop zero account repo: %s", err.Error())
	}

	if len(sentAccounts) == 0 {
		if findProfilesErr != nil {
			return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, fmt.Errorf("failed to fetch remote accounts derived from this mnemonic: %s", findProfilesErr.Error()))
		}
		return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, fmt.Errorf("failed to find any local or remote accounts derived from this mnemonic"))
	}

	return response(pb.RpcAccountRecoverResponseError_NULL, nil)
}

func (mw *Middleware) AccountSelect(req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		var clientConfig *pb.RpcAccountConfig
		if account != nil {
			clientConfig = mw.getAccountConfig()
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
		mw.app.MustComponent("blockService").(block.Service).CloseBlocks()
		return response(&model.Account{Id: req.Id}, pb.RpcAccountSelectResponseError_NULL, nil)
	}

	// in case user selected account other than the first one(used to perform search)
	// or this is the first time in this session we run the Anytype node
	if err := mw.stop(); err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE, err)
	}

	if req.RootPath != "" {
		mw.rootPath = req.RootPath
	}

	if _, err := os.Stat(filepath.Join(mw.rootPath, req.Id)); os.IsNotExist(err) {
		if mw.mnemonic == "" {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET, err)
		}

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

		var accountPreviouslyWasFoundRemotely bool
		for _, foundAccount := range mw.foundAccounts {
			if foundAccount.Id == account.Address() {
				accountPreviouslyWasFoundRemotely = true
			}
		}

		// do not allow to create repo if it wasn't previously(in the same session) found on the cafe
		if !accountPreviouslyWasFoundRemotely {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, fmt.Errorf("first you need to recover your account from remote cafe or create the new one with invite code"))
		}

		seedRaw, err := account.Raw()
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
		}

		if err = core.WalletInitRepo(mw.rootPath, seedRaw); err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
	}

	comps, err := anytype.BootstrapConfigAndWallet(false, mw.rootPath, req.Id)
	if err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
	}

	comps = append(comps, mw.EventSender)
	if mw.app, err = anytype.StartNewApp(comps...); err != nil {
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		if strings.Contains(err.Error(), errSubstringMultipleAnytypeInstance) {
			return response(nil, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING, err)
		}

		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE, err)
	}

	return response(&model.Account{Id: req.Id}, pb.RpcAccountSelectResponseError_NULL, nil)
}

func (mw *Middleware) AccountStop(req *pb.RpcAccountStopRequest) *pb.RpcAccountStopResponse {
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

	address := mw.app.MustComponent(core.CName).(core.Service).Account()
	err := mw.stop()
	if err != nil {
		return response(pb.RpcAccountStopResponseError_FAILED_TO_STOP_NODE, err)
	}

	if req.RemoveData {
		err := os.RemoveAll(filepath.Join(mw.rootPath, address))
		if err != nil {
			return response(pb.RpcAccountStopResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA, err)
		}
	}

	return response(pb.RpcAccountStopResponseError_NULL, nil)
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
