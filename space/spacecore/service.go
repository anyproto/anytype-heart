package spacecore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/spacepayloads"
	"go.uber.org/zap"

	// nolint: misspell
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
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

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/object/treesyncer"
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/space/spacecore/keyvalueobserver"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

const (
	CName = "client.space.spacecore"
)

var log = logger.NewNamed(CName)

type ctxKey int

const OptsKey ctxKey = iota

type Opts struct {
	SignKey crypto.PrivKey
}

func New() SpaceCoreService {
	return &service{}
}

type PoolManager interface {
	UnaryPeerPool() pool.Pool
	StreamPeerPool() pool.Pool
}

type SpaceCoreService interface {
	Create(ctx context.Context, spaceType spacedomain.SpaceType, replicationKey uint64, metadataPayload []byte) (*AnySpace, error)
	Derive(ctx context.Context, spaceType spacedomain.SpaceType) (space *AnySpace, err error)
	DeriveID(ctx context.Context, spaceType spacedomain.SpaceType) (id string, err error)
	CreateOneToOneSpace(ctx context.Context, bPk crypto.PubKey) (space *AnySpace, err error)
	Delete(ctx context.Context, spaceId string) (err error)
	Get(ctx context.Context, id string) (*AnySpace, error)
	Pick(ctx context.Context, id string) (*AnySpace, error)
	CloseSpace(ctx context.Context, id string) error
	StorageExistsLocally(ctx context.Context, spaceId string) (exists bool, err error)
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

	dbsAreFlushing     atomic.Bool
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func (s *service) Init(a *app.App) (err error) {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

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
	localDiscovery := a.MustComponent(localdiscovery.CName).(localdiscovery.LocalDiscovery)
	localDiscovery.SetNotifier(s)
	s.spaceCache = ocache.New(
		s.loadSpace,
		ocache.WithLogger(log.Sugar()),
		ocache.WithGCPeriod(time.Minute),
		ocache.WithTTL(time.Duration(s.conf.GCTTL)*time.Second),
	)
	s.streamPool = a.MustComponent(streampool.CName).(streampool.StreamPool)
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

func (s *service) Derive(ctx context.Context, spaceType spacedomain.SpaceType) (space *AnySpace, err error) {
	payload := spacepayloads.SpaceDerivePayload{
		SigningKey: s.wallet.GetAccountPrivkey(),
		MasterKey:  s.wallet.GetMasterKey(),
		SpaceType:  string(spaceType),
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

func (s *service) CreateOneToOneSpace(ctx context.Context, bPk crypto.PubKey) (space *AnySpace, err error) {
	id, err := s.commonSpace.DeriveOneToOneSpace(ctx, s.wallet.GetAccountPrivkey(), bPk)
	if err != nil {
		return
	}

	obj, err := s.spaceCache.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return obj.(*AnySpace), nil
}

func (s *service) CloseSpace(ctx context.Context, id string) error {
	_, err := s.spaceCache.Remove(ctx, id)
	return err
}

func (s *service) DeriveID(ctx context.Context, spaceType spacedomain.SpaceType) (id string, err error) {
	payload := spacepayloads.SpaceDerivePayload{
		SigningKey: s.wallet.GetAccountPrivkey(),
		MasterKey:  s.wallet.GetMasterKey(),
		SpaceType:  string(spaceType),
	}
	return s.commonSpace.DeriveId(ctx, payload)
}

func (s *service) Create(ctx context.Context, spaceType spacedomain.SpaceType, replicationKey uint64, metadataPayload []byte) (container *AnySpace, err error) {
	metadataPrivKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate metadata key: %w", err)
	}
	payload := spacepayloads.SpaceCreatePayload{
		SigningKey:     s.wallet.GetAccountPrivkey(),
		MasterKey:      s.wallet.GetMasterKey(),
		ReadKey:        crypto.NewAES(),
		MetadataKey:    metadataPrivKey,
		SpaceType:      string(spaceType),
		ReplicationKey: replicationKey,
		Metadata:       metadataPayload,
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

func (s *service) Pick(ctx context.Context, id string) (space *AnySpace, err error) {
	v, err := s.spaceCache.Pick(ctx, id)
	if err != nil {
		return
	}
	return v.(*AnySpace), nil
}

func (s *service) StorageExistsLocally(ctx context.Context, spaceId string) (exists bool, err error) {
	st, err := s.spaceStorageProvider.WaitSpaceStorage(ctx, spaceId)
	if err != nil && !errors.Is(err, spacestorage.ErrSpaceStorageMissing) {
		return false, err
	}
	if errors.Is(err, spacestorage.ErrSpaceStorageMissing) {
		return false, nil
	}
	err = st.Close(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *service) Delete(ctx context.Context, spaceId string) (err error) {
	networkID := s.nodeConf.Configuration().NetworkId
	delConf, err := coordinatorproto.PrepareDeleteConfirmation(s.accountKeys.SignKey, spaceId, s.accountKeys.PeerId, networkID)
	if err != nil {
		return
	}
	err = s.coordinator.SpaceDelete(ctx, spaceId, delConf)
	if err != nil {
		err = convertCoordError(err)
		return
	}
	return
}

func (s *service) loadSpace(ctx context.Context, id string) (value ocache.Object, err error) {
	kvObserver := keyvalueobserver.New()
	statusService := objectsyncstatus.NewSyncStatusService()
	deps := commonspace.Deps{
		TreeSyncer: treesyncer.NewTreeSyncer(id),
		SyncStatus: statusService,
		Indexer:    kvObserver,
	}
	if res, ok := ctx.Value(OptsKey).(Opts); ok && res.SignKey != nil {
		// TODO: [stream] replace with real peer id
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		if err != nil {
			return nil, err
		}
		acc := &accountdata.AccountKeys{
			PeerKey: pk,
			SignKey: res.SignKey,
			PeerId:  pk.GetPublic().PeerId(),
		}
		deps.AccountService = &customAccountService{acc}
	}
	cc, err := s.commonSpace.NewSpace(ctx, id, deps)

	if err != nil {
		return
	}
	ns, err := newAnySpace(cc, kvObserver)
	if err != nil {
		return
	}
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
	s.componentCtxCancel()
	return s.spaceCache.Close()
}

func (s *service) Flush(timeout time.Duration, waitPending bool) {
	if !s.dbsAreFlushing.CompareAndSwap(false, true) {
		return
	}
	defer s.dbsAreFlushing.Store(false)

	var dbs []anystore.DB
	s.spaceCache.ForEach(func(v ocache.Object) (isContinue bool) {
		if space, ok := v.(commonspace.Space); ok {
			dbs = append(dbs, space.Storage().AnyStore())
		}
		return true
	})
	var idleDuration time.Duration
	if waitPending {
		idleDuration = time.Millisecond * 50
	}

	wg := sync.WaitGroup{}
	for _, db := range dbs {
		wg.Add(1)
		go func(db anystore.DB) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(s.componentCtx, timeout)
			defer cancel()
			err := db.Flush(ctx, idleDuration, anystore.FlushModeCheckpointPassive)
			if err != nil {
				log.With(zap.Error(err)).Error("failed to flush db")
			}
		}(db)
	}
	wg.Wait()
}
