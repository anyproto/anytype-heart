package account

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/any-sync/app"
	"os"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"path/filepath"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/core/block"
	"context"
)

func (s *Service) AccountCreate(ctx context.Context, req *pb.RpcAccountCreateRequest) (*model.Account, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.stop(); err != nil {
		return nil, domain.WrapErrorWithCode(err, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE)
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

	cfg := anytype.BootstrapConfig(true, os.Getenv("ANYTYPE_STAGING") == "1", true)
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, derivationResult),
		s.eventSender,
	}

	newAcc := &model.Account{Id: accountID}

	s.app, err = anytype.StartNewApp(ctx, s.clientWithVersion, comps...)
	if err != nil {
		return newAcc, domain.WrapErrorWithCode(err, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE)
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
			return domain.WrapErrorWithCode(oserror.TransformError(err), pb.RpcAccountCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO)
		}
		// Bootstrap config will later read this config with custom storage location
		if err := config.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: storePath}); err != nil {
			return domain.WrapErrorWithCode(err, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG)
		}
	}
	return nil
}

func (s *Service) setAccountAndProfileDetails(ctx context.Context, req *pb.RpcAccountCreateRequest, newAcc *model.Account) error {
	newAcc.Name = req.Name

	spaceID := app.MustComponent[space.Service](s.app).AccountId()
	var err error
	newAcc.Info, err = app.MustComponent[account.Service](s.app).GetInfo(ctx, spaceID)
	if err != nil {
		return err
	}

	bs := s.app.MustComponent(block.CName).(*block.Service)
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
		hash, err := bs.UploadFile(context.Background(), spaceID, pb.RpcFileUploadRequest{
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

	coreService := s.app.MustComponent(core.CName).(core.Service)
	if err := bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.AccountObjects().Profile,
		Details:   profileDetails,
	}); err != nil {
		return domain.WrapErrorWithCode(err, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME)
	}

	if err := bs.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: coreService.AccountObjects().Account,
		Details:   commonDetails,
	}); err != nil {
		return domain.WrapErrorWithCode(err, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME)
	}
	return nil
}
