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
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
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

	for attempt := 1; attempt < 3; attempt++ {
		if mw.localAccountCachedAt != nil && time.Now().Sub(*mw.localAccountCachedAt).Hours() < 1 {
			break
		}

		// wait until recover ends
		time.Sleep(time.Second * 5)
		continue
	}

	if mw.Anytype != nil {
		err := mw.stop()
		if err != nil {
			response(nil, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE, err)
		}
	}

	index := len(mw.localAccounts)
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

		log.Warnf("Account already exists locally, but doesn't exist in the localAccounts list")
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

	anytype, err := core.New(mw.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytype
	newAcc := &model.Account{Id: account.Address()}

	err = mw.start()
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	err = mw.Anytype.InitPredefinedBlocks(false)
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	//err = mw.AccountSetNameAndAvatar(req.Name, req.GetAvatarLocalPath(), req.GetAvatarColor())
	//if err != nil {
	//	return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	//}
	newAcc.Name = req.Name
	/*if req.GetAvatarLocalPath() != "" {
		_, err := mw.AccountSetAvatar(req.GetAvatarLocalPath())
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}

		hash, err := mw.Textile.Avatar()
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}
		newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{Image: &model.BlockContentFile{Hash: hash}}}
	} else if req.GetAvatarColor() != "" {
		err := mw.AccountSetAvatarColor(req.GetAvatarColor())
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}
	}*/

	mw.localAccounts = append(mw.localAccounts, newAcc)
	mw.setBlockService(block.NewService(newAcc.Id, mw.Anytype, mw.linkPreview, mw.SendEvent))
	return response(newAcc, pb.RpcAccountCreateResponseError_NULL, nil)
}

func (mw *Middleware) AccountRecover(_ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	response := func(code pb.RpcAccountRecoverResponseErrorCode, err error) *pb.RpcAccountRecoverResponse {
		m := &pb.RpcAccountRecoverResponse{Error: &pb.RpcAccountRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var sentAccounts = make(map[string]struct{})
	sendAccountAddEvent := func(index int, account *model.Account) {
		sentAccounts[account.Id] = struct{}{}
		m := &pb.Event{Messages: []*pb.EventMessage{{&pb.EventMessageValueOfAccountShow{AccountShow: &pb.EventAccountShow{Index: int32(index), Account: account}}}}}
		if mw.SendEvent != nil {
			mw.SendEvent(m)
		}
	}

	if mw.mnemonic == "" {
		return response(pb.RpcAccountRecoverResponseError_NEED_TO_RECOVER_WALLET_FIRST, nil)
	}

	shouldCancel := false
	var accountSearchFinished = make(chan struct{}, 1)
	var searchQueryCancel context.CancelFunc

	mw.accountSearchCancel = func() {
		shouldCancel = true
		if searchQueryCancel != nil {
			searchQueryCancel()
		}

		<-accountSearchFinished
	}

	var hasAccountLoaded = map[string]struct{}{}
	for index := 0; index < len(mw.localAccounts); index++ {
		// in case we returned to the account choose screen we can use cached accounts
		sendAccountAddEvent(index, mw.localAccounts[index])
		hasAccountLoaded[mw.localAccounts[index].Id] = struct{}{}
		if shouldCancel {
			return response(pb.RpcAccountRecoverResponseError_NULL, nil)
		}
	}

	type nonameAccountWithIndex struct {
		id    string
		index int
	}

	var accountsOnDisk []nonameAccountWithIndex
	for i := 0; i <= 10; i++ {
		account, err := core.WalletAccountAt(mw.mnemonic, i, "")
		if err != nil {
			break
		}

		if _, alreadyLoaded := hasAccountLoaded[account.Address()]; alreadyLoaded {
			continue
		}

		if _, err := os.Stat(filepath.Join(mw.rootPath, account.Address())); err == nil {
			accountsOnDisk = append(accountsOnDisk, nonameAccountWithIndex{
				id:    account.Address(),
				index: i,
			})
		}
	}

	accountSearchFinished <- struct{}{}
	n := time.Now()
	mw.localAccountCachedAt = &n

	// this is workaround when we working offline
	for _, accountOnDisk := range accountsOnDisk {
		// todo: load profile name from the details cache in badger
		if _, exists := sentAccounts[accountOnDisk.id]; !exists {
			sendAccountAddEvent(accountOnDisk.index, &model.Account{Id: accountOnDisk.id, Name: accountOnDisk.id})
		}
	}
	// todo: reimplement after cafe2.0 will be ready

	if len(accountsOnDisk) == 0 && len(mw.localAccounts) == 0 {
		return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, fmt.Errorf("remote account recovery not implemeted yet"))
	}

	return response(pb.RpcAccountRecoverResponseError_NULL, nil)
}

func (mw *Middleware) AccountSelect(req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	mw.m.Lock()
	defer mw.m.Unlock()

	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		m := &pb.RpcAccountSelectResponse{Account: account, Error: &pb.RpcAccountSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.accountSearchCancel != nil {
		// this func will wait until search process will stop in order to be sure node was properly stopped
		mw.accountSearchCancel()
	}

	if mw.Anytype == nil || req.Id != mw.Anytype.Account() {
		// in case user selected account other than the first one(used to perform search)
		// or this is the first time in this session we run the Anytype node
		if mw.Anytype != nil {
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

			account, err := core.WalletAccountAt(mw.mnemonic, len(mw.localAccounts), "")
			if err != nil {
				return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
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

		anytype, err := core.New(mw.rootPath, req.Id)
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

	/*acc.Name, err = mw.Anytype.Textile.Name()
	if err != nil {
		return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	avatarHashOrColor, err := mw.Anytype.Textile.Avatar()
	if err != nil {
		return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	if acc.Name == "" && avatarHashOrColor == "" {
		for {
			// wait for cafe registration
			// in order to use cafeAPI instead of pubsub
			if cs := mw.Anytype.Textile.Node().CafeSessions(); cs != nil && len(cs.Items) > 0 {
				break
			}

			time.Sleep(time.Second)
		}

		contact, err := mw.Anytype.AccountRequestStoredContact(context.Background(), req.Id)
		if err != nil {
			return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}
		acc.Name = contact.Name
		avatarHashOrColor = contact.Avatar
	}

	if avatarHashOrColor != "" {
		acc.Avatar = getAvatarFromString(avatarHashOrColor)
	}
	*/
	mw.setBlockService(block.NewService(acc.Id, mw.Anytype, mw.linkPreview, mw.SendEvent))
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

func getAvatarFromString(avatarHashOrColor string) *model.AccountAvatar {
	if strings.HasPrefix(avatarHashOrColor, "#") {
		return &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfColor{avatarHashOrColor}}
	} else {
		return &model.AccountAvatar{
			&model.AccountAvatarAvatarOfImage{&model.BlockContentFile{Hash: avatarHashOrColor}},
		}
	}
}
