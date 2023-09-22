package account

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "account"

var log = logging.Logger(CName)

type Service interface {
	app.Component
	GetInfo(ctx context.Context, spaceID string) (*model.AccountInfo, error)
	Delete(ctx context.Context) error
	RevertDeletion(ctx context.Context) error
	AccountId() string
	PersonalSpaceId() string
}

type service struct {
	spaceService spacecore.Service
	wallet       wallet.Wallet
	gateway      gateway.Gateway
	config       *config.Config
	blockService *block.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[spacecore.Service](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.gateway = app.MustComponent[gateway.Gateway](a)
	s.config = app.MustComponent[*config.Config](a)
	s.blockService = app.MustComponent[*block.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetInfo(ctx context.Context, spaceID string) (*model.AccountInfo, error) {
	deviceKey := s.wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().Account()

	analyticsId, err := s.getAnalyticsID(ctx)
	if err != nil {
		log.Errorf("failed to get analytics id: %s", err)
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

	ids, err := s.spaceService.TechSpace().SpaceDerivedIDs(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get derived ids: %w", err)
	}
	return &model.AccountInfo{
		HomeObjectId:           ids.Home,
		ArchiveObjectId:        ids.Archive,
		ProfileObjectId:        ids.Profile,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		AccountSpaceId:         spaceID,
		WorkspaceObjectId:      ids.Workspace,
		WidgetsId:              ids.Widgets,
		GatewayUrl:             gwAddr,
		DeviceId:               deviceId,
		LocalStoragePath:       cfg.CustomFileStorePath,
		TimeZone:               cfg.TimeZone,
		AnalyticsId:            analyticsId,
		NetworkId:              s.getNetworkID(),
	}, nil
}

func (s *service) getAnalyticsID(ctx context.Context) (string, error) {
	if s.config.AnalyticsId != "" {
		return s.config.AnalyticsId, nil
	}
	ids, err := s.spaceService.TechSpace().SpaceDerivedIDs(ctx, s.spaceService.AccountId())
	if err != nil {
		return "", fmt.Errorf("failed to get derived ids: %w", err)
	}
	sb, err := s.blockService.PickBlock(context.Background(), ids.Workspace)
	if err != nil {
		return "", err
	}

	var analyticsID string
	st := sb.NewState().GetSetting(state.SettingsAnalyticsId)
	if st == nil {
		log.Errorf("analytics id not found")
	} else {
		analyticsID = st.GetStringValue()
	}

	return analyticsID, err
}

func (s *service) getNetworkID() string {
	return s.config.GetNodeConf().NetworkId
}
