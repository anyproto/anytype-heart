package account

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "account"

var log = logging.Logger(CName)

type Service interface {
	app.Component
	GetInfo(ctx context.Context, spaceID string) (*model.AccountInfo, error)
	Delete(ctx context.Context) (spacecore.NetworkStatus, error)
	RevertDeletion(ctx context.Context) error
	AccountID() string
	PersonalSpaceID() string
}

type service struct {
	spaceCore    spacecore.SpaceCoreService
	spaceService space.SpaceService
	wallet       wallet.Wallet
	gateway      gateway.Gateway
	config       *config.Config

	picker       getblock.ObjectGetter
	once            sync.Once
	personalSpaceID string
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.SpaceService](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.gateway = app.MustComponent[gateway.Gateway](a)
	s.config = app.MustComponent[*config.Config](a)
	s.picker = app.MustComponent[getblock.ObjectGetter](a)
	s.personalSpaceID, err = s.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	return
}

func (s *service) Delete(ctx context.Context) (spacecore.NetworkStatus, error) {
	return spacecore.NetworkStatus{}, fmt.Errorf("not implemented")
}

func (s *service) RevertDeletion(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (s *service) AccountID() string {
	return s.wallet.Account().SignKey.GetPublic().Account()
}

func (s *service) PersonalSpaceID() string {
	return s.personalSpaceID
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

	// TODO Temporary
	personalIds, err := s.getIds(ctx, s.PersonalSpaceID())
	if err != nil {
		return nil, fmt.Errorf("failed to get derived ids: %w", err)
	}

	ids, err := s.getIds(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get derived ids: %w", err)
	}

	spaceViewId, err := s.spaceService.SpaceViewId(spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spaceViewId: %w", err)
	}

	return &model.AccountInfo{
		HomeObjectId:           ids.Home,
		ArchiveObjectId:        ids.Archive,
		ProfileObjectId:        personalIds.Profile,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		DeviceId:               deviceId,
		AccountSpaceId:         spaceID,
		WidgetsId:              ids.Widgets,
		SpaceViewId:            spaceViewId,
		GatewayUrl:             gwAddr,
		LocalStoragePath:       cfg.CustomFileStorePath,
		TimeZone:               cfg.TimeZone,
		AnalyticsId:            analyticsId,
		NetworkId:              s.getNetworkID(),
	}, nil
}

func (s *service) getIds(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error) {
	spc, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return ids, fmt.Errorf("failed to get space: %w", err)
	}
	return spc.DeriveObjectIDs(ctx)
}

func (s *service) getAnalyticsID(ctx context.Context) (string, error) {
	if s.config.AnalyticsId != "" {
		return s.config.AnalyticsId, nil
	}
	ids, err := s.getIds(ctx, s.personalSpaceID)
	if err != nil {
		return "", fmt.Errorf("failed to get derived ids: %w", err)
	}
	var analyticsID string
	err = getblock.Do(s.picker, ids.Workspace, func(sb smartblock.SmartBlock) error {
		st := sb.NewState().GetSetting(state.SettingsAnalyticsId)
		if st == nil {
			log.Errorf("analytics id not found")
		} else {
			analyticsID = st.GetStringValue()
		}
		return nil
	})
	return analyticsID, err
}

func (s *service) getNetworkID() string {
	return s.config.GetNodeConf().NetworkId
}
