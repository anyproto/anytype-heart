package core

import (
	"archive/zip"
	"context"
	"github.com/anytypeio/any-sync/app"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	importer "github.com/anytypeio/go-anytype-middleware/core/block/import"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-anytype-middleware/util/files"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (mw *Middleware) UserDataImport(cctx context.Context,
	req *pb.RpcUserDataImportRequest) *pb.RpcUserDataImportResponse {
	ctx := mw.newContext(cctx)

	response := func(code pb.RpcUserDataImportResponseErrorCode, err error) *pb.RpcUserDataImportResponse {
		m := &pb.RpcUserDataImportResponse{Error: &pb.RpcUserDataImportResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	profile, err := importer.ImportUserProfile(ctx, req)
	if err != nil {
		return response(pb.RpcUserDataImportResponseError_UNKNOWN_ERROR, err)
	}
	address, err := mw.createAccount(profile, req)
	if err != nil {
		return response(pb.RpcUserDataImportResponseError_UNKNOWN_ERROR, err)
	}

	importer := mw.app.MustComponent(importer.CName).(importer.Importer)
	err = importer.ImportUserData(ctx, req, address)

	if err != nil {
		return response(pb.RpcUserDataImportResponseError_UNKNOWN_ERROR, err)
	}

	mw.mnemonic = ""

	return response(pb.RpcUserDataImportResponseError_NULL, nil)
}

func (mw *Middleware) createAccount(profile *pb.Profile, req *pb.RpcUserDataImportRequest) (string, error) {
	mw.m.Lock()

	defer mw.m.Unlock()
	if err := mw.stop(); err != nil {
		return "", err
	}

	mw.rootPath = req.RootPath
	mw.foundAccounts = nil

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return "", err
	}
	err = mw.setMnemonic(profile.Mnemonic)
	if err != nil {
		return "", err
	}
	mw.accountSearchCancel()

	err = mw.extractAccountDirectory(profile, req)
	if err != nil {
		return "", err
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", false)
	index := len(mw.foundAccounts)
	var account wallet.Keypair
	account, err = core.WalletAccountAt(mw.mnemonic, index, "")
	if err != nil {
		return "", err
	}

	if req.StorePath != "" && req.StorePath != mw.rootPath {
		configPath := filepath.Join(mw.rootPath, account.Address(), config.ConfigFileName)

		storePath := filepath.Join(req.StorePath, account.Address())

		err = os.MkdirAll(storePath, 0700)
		if err != nil {
			return "", err
		}

		if err = files.WriteJsonConfig(configPath, config.ConfigRequired{IPFSStorageAddr: storePath}); err != nil {
			return "", err
		}
	}

	newAcc := &model.Account{Id: account.Address()}

	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(mw.rootPath, account.Address()),
		mw.EventSender,
	}

	ctxWithValue := context.WithValue(context.Background(), metrics.CtxKeyRequest, "account_create")
	if mw.app, err = anytype.StartNewApp(ctxWithValue, comps...); err != nil {
		return "", err
	}

	newAcc.Name = profile.Name
	details := []*pb.RpcObjectSetDetailsDetail{{Key: "name", Value: pbtypes.String(profile.Name)}}
	newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{
		Image: &model.BlockContentFile{Hash: profile.Avatar},
	}}
	details = append(details, &pb.RpcObjectSetDetailsDetail{
		Key:   "iconImage",
		Value: pbtypes.String(profile.Avatar),
	})

	newAcc.Info = mw.getInfo()
	bs := mw.app.MustComponent(block.CName).(*block.Service)
	coreService := mw.app.MustComponent(core.CName).(core.Service)
	if err = bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.PredefinedBlocks().Profile,
		Details:   details,
	}); err != nil {
		return "", err
	}

	mw.foundAccounts = append(mw.foundAccounts, newAcc)
	return account.Address(), nil
}

func (mw *Middleware) extractAccountDirectory(profile *pb.Profile, req *pb.RpcUserDataImportRequest) error {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return err
	}
	for _, file := range archive.File {
		path := filepath.Join(mw.rootPath, file.Name)
		if file.FileInfo().IsDir() && strings.EqualFold(file.FileInfo().Name(), profile.Address) {
			os.MkdirAll(path, file.Mode())
			break
		}
	}
	for _, file := range archive.File {
		if strings.EqualFold(file.FileInfo().Name(), profile.Address) && file.FileInfo().IsDir() {
			continue
		}
		fName := file.FileHeader.Name
		if strings.Contains(fName, profile.Address) {
			if err = mw.createAccountFile(fName, file); err != nil {
				return err
			}

		}
	}
	return nil
}

func (mw *Middleware) createAccountFile(fName string, file *zip.File) error {
	path := filepath.Join(mw.rootPath, fName)
	if file.FileInfo().IsDir() {
		os.MkdirAll(path, file.Mode())
		return nil
	}
	fileReader, err := file.Open()
	if err != nil {
		return err
	}

	defer fileReader.Close()
	targetFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}

	defer targetFile.Close()
	if _, err = io.Copy(targetFile, fileReader); err != nil {
		return err
	}
	return nil
}
