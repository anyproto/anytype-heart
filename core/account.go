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
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const cafeUrl = "https://cafe1.anytype.io"
const cafePeerId = "12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"

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

	req, err := http.NewRequest("POST", cafeUrl+"/alpha-invite", bytes.NewBuffer(jsonStr))
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

	pubk, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeDevice, cafePeerId)
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

func (mw *Middleware) AccountCreate(req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		m := &pb.RpcAccountCreateResponse{Account: account, Error: &pb.RpcAccountCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype != nil {
		err := mw.stop()
		if err != nil {
			response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE, err)
		}
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

	err := checkInviteCode(req.AlphaInviteCode, account.Address())
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_BAD_INVITE_CODE, err)
	}

	seedRaw, err := account.Raw()
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	err = core.WalletInitRepo(mw.rootPath, seedRaw)
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	anytypeService, err := core.New(mw.rootPath, account.Address(), mw.reindexDoc, change.NewSnapshotChange)
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytypeService
	newAcc := &model.Account{Id: account.Address()}

	err = mw.start()
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	err = mw.Anytype.InitPredefinedBlocks(false)
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	newAcc.Name = req.Name

	bs := block.NewService(newAcc.Id, anytype.NewService(mw.Anytype), mw.linkPreview, mw.EventSender.Send)
	var details []*pb.RpcBlockSetDetailsDetail
	details = append(details, &pb.RpcBlockSetDetailsDetail{
		Key:   "name",
		Value: pbtypes.String(req.Name),
	})

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

	err = bs.SetDetails(pb.RpcBlockSetDetailsRequest{
		ContextId: mw.Anytype.PredefinedBlocks().Profile,
		Details:   details,
	})
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}
	mw.foundAccounts = append(mw.foundAccounts, newAcc)
	mw.setBlockService(bs)
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

	var sentAccountsMutex = sync.RWMutex{}
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
	for index := 0; index < len(mw.foundAccounts); index++ {
		// in case we returned to the account choose screen we can use cached accounts
		sendAccountAddEvent(index, mw.foundAccounts[index])
	}

	accounts, err := mw.getDerivedAccountsForMnemonic(10)
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_BAD_INPUT, err)
	}

	zeroAccount := accounts[0]
	zeroAccountWasJustCreated := false
	if !mw.isAccountExistsOnDisk(zeroAccount.Address()) {
		seedRaw, err := zeroAccount.Raw()
		if err != nil {
			return response(pb.RpcAccountRecoverResponseError_UNKNOWN_ERROR, err)
		}

		zeroAccountWasJustCreated = true
		err = core.WalletInitRepo(mw.rootPath, seedRaw)
		if err != nil {
			return response(pb.RpcAccountRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
	}

	// do not unlock on defer because client may do AccountSelect before all remote accounts arrives
	// it is ok to unlock just after we've started with the 1st account

	c, err := core.New(mw.rootPath, zeroAccount.Address(), mw.reindexDoc, change.NewSnapshotChange)
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
	}

	mw.Anytype = c

	recoveryFinished := make(chan struct{})
	defer close(recoveryFinished)

	ctx, searchQueryCancel := context.WithTimeout(context.Background(), time.Second*30)
	mw.accountSearchCancelAndWait = func() {
		searchQueryCancel()
		<-recoveryFinished
	}
	defer searchQueryCancel()

	err = mw.start()
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_FAILED_TO_RUN_NODE, err)
	}

	go func() {
		// this is workaround when we are working offline
		for i, acc := range accounts {
			if !mw.isAccountExistsOnDisk(acc.Address()) {
				continue
			}
			// todo: load profile name from the details cache in badger
			sendAccountAddEvent(i, &model.Account{Id: acc.Address(), Name: ""})
		}
	}()

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

				sendAccountAddEvent(index, &model.Account{
					Id:     profile.AccountAddr,
					Name:   profile.Name,
					Avatar: avatar,
				})
			}
		}

	}()

	findProfilesErr := c.FindProfilesByAccountIDs(ctx, keypairsToAddresses(accounts), profilesCh)
	if findProfilesErr != nil {

		log.Errorf("remote profiles request failed: %s", findProfilesErr.Error())
	}

	// wait until we read all profiles from chan and process them
	<-remoteAccountsProceed

	sentAccountsMutex.Lock()
	defer sentAccountsMutex.Unlock()

	if len(sentAccounts) == 0 {
		if zeroAccountWasJustCreated {
			err = mw.stop()
			if err != nil {
				log.Error("failed to stop zero account repo: %s", err.Error())
			}
			err = os.RemoveAll(filepath.Join(mw.rootPath, zeroAccount.Address()))
			if err != nil {
				log.Error("failed to remove zero account repo: %s", err.Error())
			}
		}
		if findProfilesErr != nil {
			return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, fmt.Errorf("failed to fetch remote accounts derived from this mnemonic: %s", findProfilesErr.Error()))
		}

		return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, fmt.Errorf("failed to find any local or remote accounts derived from this mnemonic"))
	}

	return response(pb.RpcAccountRecoverResponseError_NULL, nil)
}

func (mw *Middleware) AccountSelect(req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		m := &pb.RpcAccountSelectResponse{Account: account, Error: &pb.RpcAccountSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// this func will wait until search process will stop in order to be sure node was properly stopped
	mw.accountSearchCancelAndWait()

	mw.m.Lock()
	defer mw.m.Unlock()

	if mw.Anytype == nil || req.Id != mw.Anytype.Account() || !mw.Anytype.IsStarted() {
		// in case user selected account other than the first one(used to perform search)
		// or this is the first time in this session we run the Anytype node
		if mw.Anytype != nil {
			log.Debugf("AccountSelect wrong account %s instead of %s. stop it", mw.Anytype.Account(), req.Id)
			// user chose account other than the first one
			// we need to stop the first node that what used to search other accounts and then start the right one
			err := mw.stop()
			if err != nil {
				return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE, err)
			}
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

			err = core.WalletInitRepo(mw.rootPath, seedRaw)
			if err != nil {
				return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
			}
		}

		anytype, err := core.New(mw.rootPath, req.Id, mw.reindexDoc, change.NewSnapshotChange)
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
		}

		mw.Anytype = anytype

		err = mw.start()

		if err != nil {
			if err == core.ErrRepoCorrupted {
				return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
			}

			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE, err)
		}
	}

	acc := &model.Account{Id: req.Id}
	err := mw.Anytype.InitPredefinedBlocks(true)
	if err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RECOVER_PREDEFINED_BLOCKS, err)
	}

	mw.setBlockService(block.NewService(acc.Id, mw.Anytype, mw.linkPreview, mw.EventSender.Send))
	return response(acc, pb.RpcAccountSelectResponseError_NULL, nil)
}

func (mw *Middleware) AccountStop(req *pb.RpcAccountStopRequest) *pb.RpcAccountStopResponse {
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(code pb.RpcAccountStopResponseErrorCode, err error) *pb.RpcAccountStopResponse {
		m := &pb.RpcAccountStopResponse{Error: &pb.RpcAccountStopResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype == nil {
		return response(pb.RpcAccountStopResponseError_ACCOUNT_IS_NOT_RUNNING, fmt.Errorf("anytype node not set"))
	}

	address := mw.Anytype.Account()
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
