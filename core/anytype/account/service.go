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
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
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

func (s *service) GetInfo(ctx context.Context, spaceID string) (*model.AccountInfo, error) {

	deviceKey := s.wallet.GetDevicePrivkey()
	deviceId := deviceKey.GetPublic().PeerId()

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

	profileSpace, err := s.spaceService.GetPersonalSpace(ctx)
	if err != nil {
		return nil, fmt.Errorf("get personal space: %w", err)
	}
	profileObjectId := profileSpace.DerivedIDs().Profile

	techSpaceId, err := s.spaceCore.DeriveID(ctx, spacecore.TechSpaceType)
	if err != nil {
		return nil, fmt.Errorf("failed to derive tech space id: %w", err)
	}

	ids, err := s.getDerivedIds(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get derived ids: %w", err)
	}

	var spaceViewId string
	// Tech space doesn't have space view
	if spaceID != techSpaceId {
		spaceViewId, err = s.spaceService.SpaceViewId(spaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get spaceViewId: %w", err)
		}
	}

	return &model.AccountInfo{
		HomeObjectId:           ids.Home,
		ArchiveObjectId:        ids.Archive,
		ProfileObjectId:        profileObjectId,
		MarketplaceWorkspaceId: addr.AnytypeMarketplaceWorkspace,
		DeviceId:               deviceId,
		AccountSpaceId:         spaceID,
		WidgetsId:              ids.Widgets,
		SpaceViewId:            spaceViewId,
		GatewayUrl:             gwAddr,
		LocalStoragePath:       cfg.CustomFileStorePath,
		AnalyticsId:            analyticsId,
		NetworkId:              s.getNetworkID(),
		TechSpaceId:            techSpaceId,
	}, nil
}

func (s *service) getDerivedIds(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error) {
	spc, err := s.spaceService.Wait(ctx, spaceID)
	if err != nil {
		return ids, fmt.Errorf("failed to get space: %w", err)
	}
	return spc.DeriveObjectIDs(ctx)
}

func (s *service) getAnalyticsID(ctx context.Context) (string, error) {
	if s.config.AnalyticsId != "" {
		return s.config.AnalyticsId, nil
	}
	ids, err := s.getDerivedIds(ctx, s.personalSpaceId)
	if err != nil {
		return "", fmt.Errorf("failed to get derived ids: %w", err)
	}
	var analyticsID string
	err = cache.Do(s.picker, ids.Workspace, func(sb smartblock.SmartBlock) error {
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
