package spacecore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	// nolint: misspell
	commonconfig "github.com/anyproto/any-sync/commonspace/config"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/object/treesyncer"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/space/spacecore/clientspaceproto"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

const (
	CName         = "client.space.spacecore"
	SpaceType     = "anytype.space"
	TechSpaceType = "anytype.techspace"
	ChangeType    = "anytype.object"
)

var log = logger.NewNamed(CName)

func New() SpaceCoreService {
	return &service{}
}

type PoolManager interface {
	UnaryPeerPool() pool.Pool
	StreamPeerPool() pool.Pool
}

//go:generate mockgen -package mock_space -destination ./mock_space/service_mock.go github.com/anyproto/anytype-heart/space/spacecore SpaceCoreService
//go:generate mockgen -package mock_space -destination ./mock_space/commonspace_space_mock.go github.com/anyproto/any-sync/commonspace Space
type SpaceCoreService interface {
	Create(ctx context.Context, replicationKey uint64) (*AnySpace, error)
	Derive(ctx context.Context, spaceType string) (space *AnySpace, err error)
	DeriveID(ctx context.Context, spaceType string) (id string, err error)
	Delete(ctx context.Context, spaceID string) (payload NetworkStatus, err error)
	RevertDeletion(ctx context.Context, spaceID string) (err error)
	Get(ctx context.Context, id string) (*AnySpace, error)

	StreamPool() streampool.StreamPool
	app.ComponentRunnable
}

type service struct {
	conf                 commonconfig.Config
	spaceCache           ocache.OCache
	accountKeys          *accountdata.AccountKeys
	nodeConf             nodeconf.Service
	commonSpace          commonspace.SpaceService
	coordinator          coordinatorclient.CoordinatorClient
	wallet               wallet.Wallet
	spaceStorageProvider storage.ClientStorage
	streamPool           streampool.StreamPool
	peerStore            peerstore.PeerStore
	peerService          peerservice.PeerService
	poolManager          PoolManager
	streamHandler        *streamHandler
	syncStatusService    syncStatusService
}

type syncStatusService interface {
	RegisterSpace(space commonspace.Space)
	UnregisterSpace(space commonspace.Space)
}

func (s *service) Init(a *app.App) (err error) {
	conf := a.MustComponent(config.CName).(*config.Config)
	s.conf = conf.GetSpace()
	s.accountKeys = a.MustComponent(accountservice.CName).(accountservice.Service).Account()
	s.nodeConf = a.MustComponent(nodeconf.CName).(nodeconf.Service)
	s.commonSpace = a.MustComponent(commonspace.CName).(commonspace.SpaceService)
	s.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)
	s.coordinator = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	s.poolManager = a.MustComponent(peermanager.CName).(PoolManager)
	s.spaceStorageProvider = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.peerService = a.MustComponent(peerservice.CName).(peerservice.PeerService)
	s.syncStatusService = app.MustComponent[syncStatusService](a)
	localDiscovery := a.MustComponent(localdiscovery.CName).(localdiscovery.LocalDiscovery)
	localDiscovery.SetNotifier(s)
	s.streamHandler = &streamHandler{spaceCore: s}

	s.streamPool = a.MustComponent(streampool.CName).(streampool.Service).NewStreamPool(s.streamHandler, streampool.StreamConfig{
		SendQueueSize:    300,
		DialQueueWorkers: 4,
		DialQueueSize:    300,
	})
	s.spaceCache = ocache.New(
		s.loadSpace,
		ocache.WithLogger(log.Sugar()),
		ocache.WithGCPeriod(time.Minute),
		ocache.WithTTL(time.Duration(s.conf.GCTTL)*time.Second),
	)

	err = spacesyncproto.DRPCRegisterSpaceSync(a.MustComponent(server.CName).(server.DRPCServer), &rpcHandler{s})
	if err != nil {
		return
	}
	return clientspaceproto.DRPCRegisterClientSpace(a.MustComponent(server.CName).(server.DRPCServer), &rpcHandler{s})
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	return
}

func parseReplicationKey(spaceID string) (uint64, error) {
	parts := strings.Split(spaceID, ".")
	raw := parts[len(parts)-1]
	return strconv.ParseUint(raw, 36, 64)
}

func (s *service) Derive(ctx context.Context, spaceType string) (space *AnySpace, err error) {
	payload := commonspace.SpaceDerivePayload{
		SigningKey: s.wallet.GetAccountPrivkey(),
		MasterKey:  s.wallet.GetMasterKey(),
		SpaceType:  spaceType,
	}
	id, err := s.commonSpace.DeriveSpace(ctx, payload)
	if err != nil {
		return
	}
	obj, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return obj.(*AnySpace), nil
}

func (s *service) DeriveID(ctx context.Context, spaceType string) (id string, err error) {
	payload := commonspace.SpaceDerivePayload{
		SigningKey: s.wallet.GetAccountPrivkey(),
		MasterKey:  s.wallet.GetMasterKey(),
		SpaceType:  spaceType,
	}
	return s.commonSpace.DeriveId(ctx, payload)
}

func (s *service) Create(ctx context.Context, replicationKey uint64) (container *AnySpace, err error) {
	metadataPrivKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate metadata key: %w", err)
	}
	payload := commonspace.SpaceCreatePayload{
		SigningKey:     s.wallet.GetAccountPrivkey(),
		MasterKey:      s.wallet.GetMasterKey(),
		ReadKey:        crypto.NewAES(),
		MetadataKey:    metadataPrivKey,
		SpaceType:      SpaceType,
		ReplicationKey: replicationKey,
	}
	id, err := s.commonSpace.CreateSpace(ctx, payload)
	if err != nil {
		return
	}
	obj, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return obj.(*AnySpace), nil
}

func (s *service) Get(ctx context.Context, id string) (space *AnySpace, err error) {
	v, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(*AnySpace), nil
}

func (s *service) HandleMessage(ctx context.Context, senderId string, req *spacesyncproto.ObjectSyncMessage) (err error) {
	var msg = &spacesyncproto.SpaceSubscription{}
	if err = msg.Unmarshal(req.Payload); err != nil {
		return
	}
	log.InfoCtx(ctx, "got subscription message", zap.Strings("spaceIds", msg.SpaceIds))
	if msg.Action == spacesyncproto.SpaceSubscriptionAction_Subscribe {
		return s.streamPool.AddTagsCtx(ctx, msg.SpaceIds...)
	} else {
		return s.streamPool.RemoveTagsCtx(ctx, msg.SpaceIds...)
	}
}

func (s *service) StreamPool() streampool.StreamPool {
	return s.streamPool
}

func (s *service) Delete(ctx context.Context, spaceID string) (payload NetworkStatus, err error) {
	networkID := s.nodeConf.Configuration().NetworkId
	delConf, err := coordinatorproto.PrepareDeleteConfirmation(s.accountKeys.SignKey, spaceID, s.accountKeys.PeerId, networkID)
	if err != nil {
		return
	}
	status, err := s.coordinator.ChangeStatus(ctx, spaceID, delConf)
	if err != nil {
		err = convertCoordError(err)
		return
	}
	payload = NewSpaceStatus(status)
	return
}

func (s *service) RevertDeletion(ctx context.Context, spaceID string) (err error) {
	_, err = s.coordinator.ChangeStatus(ctx, spaceID, nil)
	if err != nil {
		err = convertCoordError(err)
		return
	}
	return
}

func (s *service) loadSpace(ctx context.Context, id string) (value ocache.Object, err error) {
	cc, err := s.commonSpace.NewSpace(ctx, id, commonspace.Deps{TreeSyncer: treesyncer.NewTreeSyncer(id)})
	if err != nil {
		return
	}
	ns, err := newAnySpace(cc, s.syncStatusService)
	if err != nil {
		return
	}
	if err = ns.Init(ctx); err != nil {
		return
	}
	if err != nil {
		return nil, fmt.Errorf("store mapping for space: %w", err)
	}
	return ns, nil
}

func (s *service) getOpenedSpaceIds() (ids []string) {
	s.spaceCache.ForEach(func(v ocache.Object) (isContinue bool) {
		ids = append(ids, v.(commonspace.Space).Id())
		return true
	})
	return
}

func (s *service) Close(ctx context.Context) (err error) {
	return s.spaceCache.Close()
}
