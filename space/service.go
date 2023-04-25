package space

import (
	"context"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/anytypeio/any-sync/net/streampool"
	"time"
)

const CName = "client.clientspace"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{}
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
	spaceStorageProvider spacestorage.SpaceStorageProvider
	streamPool           streampool.StreamPool
	accountId            string
}

func (s *service) Init(a *app.App) (err error) {
	s.conf = a.MustComponent("config").(commonspace.ConfigGetter).GetSpace()
	s.commonSpace = a.MustComponent(commonspace.CName).(commonspace.SpaceService)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.spaceStorageProvider = a.MustComponent(spacestorage.CName).(spacestorage.SpaceStorageProvider)
	s.streamPool = a.MustComponent(streampool.CName).(streampool.Service).NewStreamPool(&streamHandler{s: s}, streampool.StreamConfig{
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
	return spacesyncproto.DRPCRegisterSpaceSync(a.MustComponent(server.CName).(server.DRPCServer), &rpcHandler{s})
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
