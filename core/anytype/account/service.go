package account

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "account"

var log = logging.Logger(CName)

type Service interface {
	app.Component
	GetInfo(spaceID string) (*model.AccountInfo, error)
}

type service struct {
	spaceService space.Service
	coreService  core.Service
	wallet       wallet.Wallet
	gateway      gateway.Gateway
	config       *config.Config
	blockService *block.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	s.coreService = app.MustComponent[core.Service](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.gateway = app.MustComponent[gateway.Gateway](a)
	s.config = app.MustComponent[*config.Config](a)
	s.blockService = app.MustComponent[*block.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetInfo(spaceID string) (*model.AccountInfo, error) {
	deviceKey := s.wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().Account()

	analyticsId, err := s.getAnalyticsID()
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

	ctx := session.NewContext(context.Background(), spaceID)
	ids, err := s.coreService.DerivePredefinedObjects(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("derive predefined objects: %w", err)
	}
	return &model.AccountInfo{
		HomeObjectId:           ids.Home,
		ArchiveObjectId:        ids.Archive,
		ProfileObjectId:        ids.Profile,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		// AccountSpaceId:         ids.Account,
		AccountSpaceId:   spaceID,
		WidgetsId:        ids.Widgets,
		GatewayUrl:       gwAddr,
		DeviceId:         deviceId,
		LocalStoragePath: cfg.CustomFileStorePath,
		TimeZone:         cfg.TimeZone,
		AnalyticsId:      analyticsId,
	}, nil
}

func (s *service) getAnalyticsID() (string, error) {
	if s.config.AnalyticsId != "" {
		return s.config.AnalyticsId, nil
	}
	accountCtx := session.NewContext(context.Background(), s.spaceService.AccountId())
	accountObjectID := s.coreService.PredefinedBlocks().Account
	sb, err := s.blockService.PickBlock(accountCtx, accountObjectID)
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
