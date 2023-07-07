package space

import (
	"context"
	"errors"
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
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/space/clientspaceproto"
	"github.com/anyproto/anytype-heart/space/localdiscovery"
	"github.com/anyproto/anytype-heart/space/peerstore"
	"github.com/anyproto/anytype-heart/space/storage"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

const (
	CName      = "client.clientspace"
	SpaceType  = "anytype.space"
	ChangeType = "anytype.object"
)

var ErrUsingOldStorage = errors.New("using old storage")

var log = logger.NewNamed(CName)

func New() Service {
	return &service{}
}

type PoolManager interface {
	UnaryPeerPool() pool.Pool
	StreamPeerPool() pool.Pool
}

//go:generate mockgen -package mock_space -destination ./mock_space/service_mock.go github.com/anyproto/anytype-heart/space Service
//go:generate mockgen -package mock_space -destination ./mock_space/commonspace_space_mock.go github.com/anyproto/any-sync/commonspace Space
type Service interface {
	AccountSpace(ctx context.Context) (commonspace.Space, error)
	AccountId() string
	CreateSpace(ctx context.Context) (container commonspace.Space, err error)
	GetSpace(ctx context.Context, id string) (Space, error)
	DeriveSpace(ctx context.Context, payload commonspace.SpaceDerivePayload) (commonspace.Space, error)
	DeleteSpace(ctx context.Context, spaceID string, revert bool) (payload StatusPayload, err error)
	DeleteAccount(ctx context.Context, revert bool) (payload StatusPayload, err error)
	StreamPool() streampool.StreamPool
	CloseSessionsInAllObjects()
	app.ComponentRunnable
}

type ObjectFactory interface {
	InitObject(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error)
}

type service struct {
	conf                 commonconfig.Config
	spaceCache           ocache.OCache
	commonSpace          commonspace.SpaceService
	client               coordinatorclient.CoordinatorClient
	wallet               wallet.Wallet
	spaceStorageProvider storage.ClientStorage
	streamPool           streampool.StreamPool
	peerStore            peerstore.PeerStore
	peerService          peerservice.PeerService

	objectFactory ObjectFactory
	sbtProvider   typeprovider.SmartBlockTypeProvider
	core          core.Service
	commonAccount accountservice.Service

	poolManager   PoolManager
	streamHandler *streamHandler
	accountId     string
	newAccount    bool
}

func (s *service) Init(a *app.App) (err error) {
	conf := a.MustComponent(config.CName).(*config.Config)
	s.conf = conf.GetSpace()
	s.newAccount = conf.NewAccount
	s.commonSpace = a.MustComponent(commonspace.CName).(commonspace.SpaceService)
	s.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)
	s.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	s.poolManager = a.MustComponent(peermanager.CName).(PoolManager)
	s.spaceStorageProvider = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.peerService = a.MustComponent(peerservice.CName).(peerservice.PeerService)

	s.objectFactory = app.MustComponent[ObjectFactory](a)
	s.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.core = app.MustComponent[core.Service](a)
	s.commonAccount = app.MustComponent[accountservice.Service](a)

	localDiscovery := a.MustComponent(localdiscovery.CName).(localdiscovery.LocalDiscovery)
	localDiscovery.SetNotifier(s)
	s.streamHandler = &streamHandler{s: s}

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
	err = s.checkOldSpace()
	if err != nil {
		return
	}
	payload := commonspace.SpaceDerivePayload{
		SigningKey: s.wallet.GetAccountPrivkey(),
		MasterKey:  s.wallet.GetMasterKey(),
		SpaceType:  SpaceType,
	}
	if s.newAccount {
		// creating storage
		s.accountId, err = s.commonSpace.DeriveSpace(ctx, payload)
		if err != nil {
			return
		}
	} else {
		s.accountId, err = s.commonSpace.DeriveId(ctx, payload)
		if err != nil {
			return
		}
		// pulling space from remote
		_, err = s.GetSpace(ctx, s.accountId)
		if err != nil {
			return
		}
	}
	return
}

func (s *service) DeriveSpace(ctx context.Context, payload commonspace.SpaceDerivePayload) (container commonspace.Space, err error) {
	id, err := s.commonSpace.DeriveSpace(ctx, payload)
	if err != nil {
		return
	}

	obj, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return obj.(commonspace.Space), nil
}

func (s *service) AccountSpace(ctx context.Context) (container commonspace.Space, err error) {
	return s.GetSpace(ctx, s.accountId)
}

func (s *service) AccountId() string {
	return s.accountId
}

func parseReplicationKey(spaceID string) (uint64, error) {
	parts := strings.Split(spaceID, ".")
	raw := parts[len(parts)-1]
	return strconv.ParseUint(raw, 36, 64)
}

func (s *service) CreateSpace(ctx context.Context) (container commonspace.Space, err error) {
	replicationKey, err := parseReplicationKey(s.accountId)
	if err != nil {
		return nil, fmt.Errorf("parse account's replication key: %w", err)
	}
	payload := commonspace.SpaceCreatePayload{
		SigningKey:     s.wallet.GetAccountPrivkey(),
		MasterKey:      s.wallet.GetMasterKey(),
		ReadKey:        crypto.NewAES().Bytes(),
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
	return obj.(commonspace.Space), nil
}

func (s *service) GetSpace(ctx context.Context, id string) (space Space, err error) {
	v, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(Space), nil
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

func (s *service) DeleteAccount(ctx context.Context, revert bool) (payload StatusPayload, err error) {
	return s.DeleteSpace(ctx, s.accountId, revert)
}

func (s *service) DeleteSpace(ctx context.Context, spaceID string, revert bool) (payload StatusPayload, err error) {
	space, err := s.GetSpace(ctx, spaceID)
	if err != nil {
		return
	}
	var (
		raw    *treechangeproto.RawTreeChangeWithId
		status *coordinatorproto.SpaceStatusPayload
	)
	if !revert {
		raw, err = space.SpaceDeleteRawChange(ctx)
		if err != nil {
			return
		}
	}
	status, err = s.client.ChangeStatus(ctx, spaceID, raw)
	if err != nil {
		err = coordError(err)
		return
	}
	payload = newSpaceStatus(status)
	return
}

func (s *service) loadSpace(ctx context.Context, id string) (value ocache.Object, err error) {
	cc, err := s.commonSpace.NewSpace(ctx, id)
	if err != nil {
		return
	}
	ns, err := newClientSpace(cc, s.objectFactory, s.sbtProvider, s.core, s.commonAccount)
	if err != nil {
		return
	}
	if err = ns.Init(ctx); err != nil {
		return
	}
	ns.SyncStatus().(syncstatus.StatusWatcher).SetUpdateReceiver(&statusReceiver{})
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

func (s *service) PeerDiscovered(peer localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	s.peerService.SetPeerAddrs(peer.PeerId, peer.Addrs)
	ctx := context.Background()
	unaryPeer, err := s.poolManager.UnaryPeerPool().Get(ctx, peer.PeerId)
	if err != nil {
		return
	}
	allIds, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
	log.Debug("sending info about spaces to peer", zap.String("peer", peer.PeerId), zap.Strings("spaces", allIds))
	var resp *clientspaceproto.SpaceExchangeResponse
	err = unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
			SpaceIds: allIds,
			LocalServer: &clientspaceproto.LocalServer{
				Ips:  own.Addrs,
				Port: int32(own.Port),
			},
		})
		return err
	})
	if err != nil {
		return
	}
	log.Debug("got peer ids from peer", zap.String("peer", peer.PeerId), zap.Strings("spaces", resp.SpaceIds))
	s.peerStore.UpdateLocalPeer(peer.PeerId, resp.SpaceIds)
}

func (s *service) checkOldSpace() (err error) {
	old, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
	for _, id := range old {
		st, err := s.spaceStorageProvider.WaitSpaceStorage(context.Background(), id)
		if err != nil {
			return err
		}
		header, err := st.SpaceHeader()
		if err != nil {
			return err
		}
		tp, err := s.getSpaceType(header)
		if err != nil {
			return err
		}
		if tp == "derived.space" {
			return ErrUsingOldStorage
		}
		err = st.Close(context.Background())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *service) getSpaceType(header *spacesyncproto.RawSpaceHeaderWithId) (tp string, err error) {
	raw := &spacesyncproto.RawSpaceHeader{}
	err = proto.Unmarshal(header.RawHeader, raw)
	if err != nil {
		return
	}
	payload := &spacesyncproto.SpaceHeader{}
	err = proto.Unmarshal(raw.SpaceHeader, payload)
	if err != nil {
		return
	}
	tp = payload.SpaceType
	return
}

func (s *service) GetLogFields() []zap.Field {
	return []zap.Field{
		zap.Bool("newAccount", s.newAccount),
	}
}

func (s *service) CloseSessionsInAllObjects() {
	s.spaceCache.ForEach(func(v ocache.Object) (isContinue bool) {
		space := v.(*clientSpace)
		space.cache.ForEach(func(v ocache.Object) (isContinue bool) {
			ob := v.(smartblock.SmartBlock)
			ob.Lock()
			ob.ObjectCloseAllSessions()
			ob.Unlock()
			return true
		})
		return true
	})
}
