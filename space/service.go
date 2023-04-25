package space

import (
	"context"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/peermanager"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/anytypeio/any-sync/net/dialer"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/anytypeio/any-sync/net/streampool"
	"github.com/anytypeio/go-anytype-middleware/space/clientspaceproto"
	"github.com/anytypeio/go-anytype-middleware/space/localdiscovery"
	"github.com/anytypeio/go-anytype-middleware/space/peerstore"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
	"go.uber.org/zap"
	"time"
)

const CName = "client.clientspace"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{}
}

type PoolManager interface {
	UnaryPeerPool() pool.Pool
	StreamPeerPool() pool.Pool
}

type Service interface {
	AccountSpace(ctx context.Context) (commonspace.Space, error)
	AccountId() string
	GetSpace(ctx context.Context, id string) (commonspace.Space, error)
	DeriveSpace(ctx context.Context, payload commonspace.SpaceDerivePayload) (commonspace.Space, error)
	StreamPool() streampool.StreamPool
	app.ComponentRunnable
}

type service struct {
	conf                 commonspace.Config
	spaceCache           ocache.OCache
	commonSpace          commonspace.SpaceService
	account              accountservice.Service
	spaceStorageProvider storage.ClientStorage
	streamPool           streampool.StreamPool
	peerStore            peerstore.PeerStore
	dialer               dialer.Dialer
	poolManager          PoolManager
	streamHandler        *streamHandler
	accountId            string
}

func (s *service) Init(a *app.App) (err error) {
	s.conf = a.MustComponent("config").(commonspace.ConfigGetter).GetSpace()
	s.commonSpace = a.MustComponent(commonspace.CName).(commonspace.SpaceService)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.poolManager = a.MustComponent(peermanager.CName).(PoolManager)
	s.spaceStorageProvider = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.dialer = a.MustComponent(dialer.CName).(dialer.Dialer)
	localDiscovery := a.MustComponent(localdiscovery.CName).(localdiscovery.LocalDiscovery)
	localDiscovery.SetNotifier(s)
	s.streamHandler = &streamHandler{s: s}

	s.streamPool = a.MustComponent(streampool.CName).(streampool.Service).NewStreamPool(s.streamHandler, streampool.StreamConfig{
		SendQueueWorkers: 10,
		SendQueueSize:    300,
		DialQueueWorkers: 4,
		DialQueueSize:    100,
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
	s.accountId, err = s.commonSpace.DeriveSpace(context.Background(), commonspace.SpaceDerivePayload{
		SigningKey:    s.account.Account().SignKey,
		EncryptionKey: s.account.Account().EncKey,
	})
	if err != nil {
		return
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

func (s *service) GetSpace(ctx context.Context, id string) (space commonspace.Space, err error) {
	v, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(commonspace.Space), nil
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

func (s *service) loadSpace(ctx context.Context, id string) (value ocache.Object, err error) {
	cc, err := s.commonSpace.NewSpace(ctx, id)
	if err != nil {
		return
	}
	ns, err := newClientSpace(cc)
	if err != nil {
		return
	}
	ns.SyncStatus().(syncstatus.StatusWatcher).SetUpdateReceiver(&statusReceiver{})
	if err = ns.Init(ctx); err != nil {
		return
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

func (s *service) PeerDiscovered(peer localdiscovery.DiscoveredPeer) {
	s.dialer.SetPeerAddrs(peer.PeerId, []string{peer.Addr})
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
	resp, err := clientspaceproto.NewDRPCClientSpaceClient(unaryPeer).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
		SpaceIds: allIds,
	})
	if err != nil {
		return
	}
	log.Debug("got peer ids from peer", zap.String("peer", peer.PeerId), zap.Strings("spaces", resp.SpaceIds))
	s.peerStore.UpdateLocalPeer(peer.PeerId, resp.SpaceIds)
}
