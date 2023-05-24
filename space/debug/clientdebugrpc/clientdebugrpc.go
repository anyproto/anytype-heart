package clientdebugrpc

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/secureservice"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/debug/clientdebugrpc/clientdebugrpcproto"
	"github.com/anyproto/anytype-heart/space/storage"
)

const CName = "common.debug.clientdebugrpc"

var log = logger.NewNamed(CName)

func New() ClientDebugRpc {
	return &service{BaseDrpcServer: server.NewBaseDrpcServer()}
}

type configGetter interface {
	GetDebugAPIConfig() config.DebugAPIConfig
}

type ClientDebugRpc interface {
	app.ComponentRunnable
	drpc.Mux
}

type service struct {
	transport    secureservice.SecureService
	cfg          config.DebugAPIConfig
	spaceService space.Service
	blockService *block.Service
	storage      storage.ClientStorage
	file         fileservice.FileService
	*server.BaseDrpcServer
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.storage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.cfg = a.MustComponent("config").(configGetter).GetDebugAPIConfig()
	s.transport = a.MustComponent(secureservice.CName).(secureservice.SecureService)
	s.file = a.MustComponent(fileservice.CName).(fileservice.FileService)
	s.blockService = a.MustComponent(block.CName).(*block.Service)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	if !s.cfg.IsEnabled {
		return
	}
	params := server.Params{
		BufferSizeMb:  s.cfg.Stream.MaxMsgSizeMb,
		TimeoutMillis: s.cfg.Stream.TimeoutMilliseconds,
		ListenAddrs:   s.cfg.Server.ListenAddrs,
		Wrapper: func(handler drpc.Handler) drpc.Handler {
			return handler
		},
	}
	err = s.BaseDrpcServer.Run(ctx, params)
	if err != nil {
		return
	}
	return clientdebugrpcproto.DRPCRegisterClientApi(s, &rpcHandler{
		spaceService:   s.spaceService,
		storageService: s.storage,
		file:           s.file,
		blockService:   s.blockService,
	})
}

func (s *service) Close(ctx context.Context) (err error) {
	if !s.cfg.IsEnabled {
		return
	}
	return s.BaseDrpcServer.Close(ctx)
}
