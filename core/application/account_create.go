package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) AccountCreate(ctx context.Context, req *pb.RpcAccountCreateRequest) (*model.Account, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.stop(); err != nil {
		return nil, errors.Join(ErrFailedToStopApplication, err)
	}

	s.requireClientWithVersion()

	derivationResult, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return nil, err
	}
	accountID := derivationResult.Identity.GetPublic().Account()

	if err = core.WalletInitRepo(s.rootPath, derivationResult.Identity); err != nil {
		return nil, err
	}

	if err = s.handleCustomStorageLocation(req, accountID); err != nil {
		return nil, err
	}

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1")
	if req.DisableLocalNetworkSync {
		cfg.DontStartLocalNetworkSyncAutomatically = true
	}
	if req.PreferYamuxTransport {
		cfg.PeferYamuxTransport = true
	}
	if req.NetworkMode > 0 {
		cfg.NetworkMode = req.NetworkMode
		cfg.NetworkCustomConfigFilePath = req.NetworkCustomConfigFilePath
	}
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, derivationResult),
		s.eventSender,
	}

	newAcc := &model.Account{Id: accountID}

	// in case accountCreate got canceled by other request we loose nothing
	s.appAccountStartInProcessCancelMutex.Lock()
	ctx, s.appAccountStartInProcessCancel = context.WithCancel(ctx)
	s.appAccountStartInProcessCancelMutex.Unlock()
	s.app, err = anytype.StartNewApp(ctx, s.clientWithVersion, comps...)
	s.appAccountStartInProcessCancelMutex.Lock()
	s.appAccountStartInProcessCancel = nil
	s.appAccountStartInProcessCancelMutex.Unlock()
	if errors.Is(ctx.Err(), context.Canceled) {
		// todo: remove local data in case of account create cancelation
	}
	if err != nil {
		return newAcc, errors.Join(ErrFailedToStartApplication, err)
	}

	if err = s.setAccountAndProfileDetails(ctx, req, newAcc); err != nil {
		return newAcc, err
	}

	return newAcc, nil
}

func (s *Service) handleCustomStorageLocation(req *pb.RpcAccountCreateRequest, accountID string) error {
	if req.StorePath != "" && req.StorePath != s.rootPath {
		configPath := filepath.Join(s.rootPath, accountID, config.ConfigFileName)
		storePath := filepath.Join(req.StorePath, accountID)
		err := os.MkdirAll(storePath, 0700)
		if err != nil {
			return errors.Join(ErrFailedToCreateLocalRepo, err)
		}
		// Bootstrap config will later read this config with custom storage location
		if err := config.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: storePath}); err != nil {
			return errors.Join(ErrFailedToWriteConfig, err)
		}
	}
	return nil
}

func (s *Service) setAccountAndProfileDetails(ctx context.Context, req *pb.RpcAccountCreateRequest, newAcc *model.Account) error {
	techSpaceId := app.MustComponent[space.Service](s.app).TechSpaceId()
	personalSpaceId := app.MustComponent[account.Service](s.app).PersonalSpaceID()
	var err error
	newAcc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx, personalSpaceId)
	if err != nil {
		return err
	}

	bs := s.app.MustComponent(block.CName).(*block.Service)
	commonDetails := []*model.Detail{
		{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(req.Name),
		},
		{
			Key:   bundle.RelationKeyIconOption.String(),
			Value: pbtypes.Int64(req.Icon),
		},
	}
	profileDetails := make([]*model.Detail, 0)
	profileDetails = append(profileDetails, commonDetails...)

	if req.GetAvatarLocalPath() != "" {
		hash, _, err := bs.UploadFile(context.Background(), techSpaceId, block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: req.GetAvatarLocalPath(),
				Type:      model.BlockContentFile_Image,
			},
			ObjectOrigin: objectorigin.None(),
		})
		if err != nil {
			log.Warnf("can't add avatar: %v", err)
		} else {
			profileDetails = append(profileDetails, &model.Detail{
				Key:   bundle.RelationKeyIconImage.String(),
				Value: pbtypes.String(hash),
			})
		}
	}
	spaceService := app.MustComponent[space.Service](s.app)
	spc, err := spaceService.Get(ctx, personalSpaceId)
	if err != nil {
		return errors.Join(ErrSetDetails, err)
	}
	accountObjects := spc.DerivedIDs()
	accId, err := spaceService.TechSpace().AccountObjectId()
	if err != nil {
		return errors.Join(ErrSetDetails, err)
	}
	if err := bs.SetDetails(nil,
		accId,
		profileDetails,
	); err != nil {
		return errors.Join(ErrSetDetails, err)
	}

	if err := bs.SetDetails(nil,
		accountObjects.Workspace,
		commonDetails,
	); err != nil {
		return errors.Join(ErrSetDetails, err)
	}
	return nil
}
