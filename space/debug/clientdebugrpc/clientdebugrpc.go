package clientdebugrpc

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/anytypeio/any-sync/net/secureservice"
	"storj.io/drpc"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/debug/clientdebugrpc/clientdebugrpcproto"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
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
		Converter: s.transport.BasicListener,
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
