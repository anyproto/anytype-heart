package space

import (
	"context"
	"errors"
	"github.com/gogo/protobuf/proto"
	"time"

	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	//nolint: misspell
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/any-sync/commonspace/peermanager"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/anytypeio/any-sync/coordinator/coordinatorclient"
	"github.com/anytypeio/any-sync/coordinator/coordinatorproto"
	"github.com/anytypeio/any-sync/net/dialer"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/anytypeio/any-sync/net/streampool"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/space/clientspaceproto"
	"github.com/anytypeio/go-anytype-middleware/space/localdiscovery"
	"github.com/anytypeio/go-anytype-middleware/space/peerstore"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
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

type Service interface {
	AccountSpace(ctx context.Context) (commonspace.Space, error)
	AccountId() string
	GetSpace(ctx context.Context, id string) (commonspace.Space, error)
	DeriveSpace(ctx context.Context, payload commonspace.SpaceDerivePayload) (commonspace.Space, error)
	DeleteSpace(ctx context.Context, spaceID string, revert bool) (payload StatusPayload, err error)
	DeleteAccount(ctx context.Context, revert bool) (payload StatusPayload, err error)
	StreamPool() streampool.StreamPool
	app.ComponentRunnable
}

type service struct {
	conf                 commonspace.Config
	spaceCache           ocache.OCache
	commonSpace          commonspace.SpaceService
	client               coordinatorclient.CoordinatorClient
	account              accountservice.Service
	spaceStorageProvider storage.ClientStorage
	streamPool           streampool.StreamPool
	peerStore            peerstore.PeerStore
	dialer               dialer.Dialer
	poolManager          PoolManager
	streamHandler        *streamHandler
	accountId            string
	newAccount           bool
}

func (s *service) Init(a *app.App) (err error) {
	conf := a.MustComponent(config.CName).(*config.Config)
	s.conf = conf.GetSpace()
	s.newAccount = conf.NewAccount
	s.commonSpace = a.MustComponent(commonspace.CName).(commonspace.SpaceService)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
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
	err = s.checkOldSpace()
	if err != nil {
		return
	}
	payload := commonspace.SpaceDerivePayload{
		SigningKey:    s.account.Account().SignKey,
		EncryptionKey: s.account.Account().EncKey,
		SpaceType:     SpaceType,
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
	s.dialer.SetPeerAddrs(peer.PeerId, peer.Addrs)
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
		err = st.Close()
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
