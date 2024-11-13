package account

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "account"

var log = logging.Logger(CName)

type Service interface {
	app.Component
	GetInfo(ctx context.Context) (*model.AccountInfo, error)
	GetSpaceInfo(ctx context.Context, spaceId string) (*model.AccountInfo, error)
	Delete(ctx context.Context) (toBeDeleted int64, err error)
	RevertDeletion(ctx context.Context) error
	AccountID() string
	SignData(data []byte) (signature []byte, err error)
	PersonalSpaceID() string
	MyParticipantId(string) string
	// ProfileObjectId returns id of Profile object stored in personal space
	ProfileObjectId() (string, error)
	ProfileInfo() (Profile, error)
	Keys() *accountdata.AccountKeys
}

type service struct {
	spaceCore    spacecore.SpaceCoreService
	spaceService space.Service
	wallet       wallet.Wallet
	gateway      gateway.Gateway
	config       *config.Config
	objectStore  objectstore.ObjectStore
	keyProvider  accountservice.Service
	nodeConf     nodeconf.Service
	coordClient  coordinatorclient.CoordinatorClient

	picker          cache.ObjectGetter
	personalSpaceId string
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.gateway = app.MustComponent[gateway.Gateway](a)
	s.nodeConf = app.MustComponent[nodeconf.Service](a)
	s.coordClient = app.MustComponent[coordinatorclient.CoordinatorClient](a)
	s.keyProvider = app.MustComponent[accountservice.Service](a)
	s.config = app.MustComponent[*config.Config](a)
	s.picker = app.MustComponent[cache.ObjectGetter](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	return
}

func (s *service) Keys() *accountdata.AccountKeys {
	return s.keyProvider.Account()
}

func (s *service) Delete(ctx context.Context) (toBeDeleted int64, err error) {
	confirm, err := coordinatorproto.PrepareAccountDeleteConfirmation(s.wallet.GetAccountPrivkey(), s.wallet.GetDevicePrivkey().GetPublic().PeerId(), s.nodeConf.Configuration().NetworkId)
	if err != nil {
		return
	}
	return s.coordClient.AccountDelete(ctx, confirm)
}

func (s *service) RevertDeletion(ctx context.Context) error {
	return s.coordClient.AccountRevertDeletion(ctx)
}

func (s *service) AccountID() string {
	return s.wallet.Account().SignKey.GetPublic().Account()
}

func (s *service) SignData(data []byte) (signature []byte, err error) {
	return s.wallet.Account().SignKey.Sign(data)
}

func (s *service) PersonalSpaceID() string {
	return s.personalSpaceId
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetInfo(ctx context.Context) (*model.AccountInfo, error) {
	accountId, err := s.spaceService.TechSpace().AccountObjectId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account id: %w", err)
	}
	deviceKey := s.wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().PeerId()

	analyticsId, err := s.getAnalyticsId(ctx, s.spaceService.TechSpace())
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics id: %w", err)
	}

	gwAddr := s.gateway.Addr()
	if gwAddr != "" {
		gwAddr = "http://" + gwAddr
	}

	cfg := config.ConfigRequired{}
	err = config.GetFileConfig(filepath.Join(s.wallet.RepoPath(), config.ConfigFileName), &cfg)
	if err != nil || cfg.CustomFileStorePath == "" {
		cfg.CustomFileStorePath = s.wallet.RepoPath()
	}

	return &model.AccountInfo{
		ProfileObjectId:        accountId,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		DeviceId:               deviceId,
		GatewayUrl:             gwAddr,
		LocalStoragePath:       cfg.CustomFileStorePath,
		AnalyticsId:            analyticsId,
		NetworkId:              s.getNetworkId(),
		TechSpaceId:            s.spaceService.TechSpaceId(),
	}, nil
}

func (s *service) GetSpaceInfo(ctx context.Context, spaceId string) (*model.AccountInfo, error) {
	getInfo, err := s.GetInfo(ctx)
	if err != nil {
		return nil, err
	}
	spc, err := s.spaceService.Wait(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get space: %w", err)
	}
	ids := spc.DerivedIDs()
	spaceViewId, err := s.spaceService.SpaceViewId(spaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get spaceViewId: %w", err)
	}
	getInfo.AccountSpaceId = spaceId
	getInfo.SpaceViewId = spaceViewId
	getInfo.HomeObjectId = ids.Home
	getInfo.WorkspaceObjectId = ids.Workspace
	getInfo.WidgetsId = ids.Widgets
	getInfo.ArchiveObjectId = ids.Archive
	return getInfo, nil
}

func (s *service) getAnalyticsId(ctx context.Context, techSpace techspace.TechSpace) (analyticsId string, err error) {
	if s.config.AnalyticsId != "" {
		return s.config.AnalyticsId, nil
	}
	err = techSpace.DoAccountObject(ctx, func(accountObject techspace.AccountObject) error {
		analyticsId, err = accountObject.GetAnalyticsId()
		if err != nil {
			log.Debug("failed to get analytics id: %s", err)
		}
		return nil
	})
	if analyticsId == "" {
		// TODO Temporarily commented
		// err = s.spaceService.WaitPersonalSpaceMigration(ctx)
		// if err != nil {
		// 	return
		// }
	} else {
		return analyticsId, nil
	}
	err = techSpace.DoAccountObject(ctx, func(accountObject techspace.AccountObject) error {
		analyticsId, err = accountObject.GetAnalyticsId()
		return err
	})
	return
}

func (s *service) getNetworkId() string {
	return s.config.GetNodeConf().NetworkId
}
