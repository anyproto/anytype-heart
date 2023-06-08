package clientdebugrpc

import (
	"context"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/rpc/debugserver"
	"github.com/anyproto/any-sync/net/secureservice"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/debug/clientdebugrpc/clientdebugrpcproto"
	"github.com/anyproto/anytype-heart/space/storage"
)

const CName = "common.debug.clientdebugrpc"

var log = logger.NewNamed(CName)

func New() ClientDebugRpc {
	return &service{}
}

type configGetter interface {
	GetDebugAPIConfig() config.DebugAPIConfig
}

type ClientDebugRpc interface {
	app.ComponentRunnable
}

type service struct {
	transport    secureservice.SecureService
	cfg          config.DebugAPIConfig
	spaceService space.Service
	blockService *block.Service
	storage      storage.ClientStorage
	file         fileservice.FileService
	server       debugserver.DebugServer
	account      accountservice.Service
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.storage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.cfg = a.MustComponent("config").(configGetter).GetDebugAPIConfig()
	s.transport = a.MustComponent(secureservice.CName).(secureservice.SecureService)
	s.file = a.MustComponent(fileservice.CName).(fileservice.FileService)
	s.blockService = a.MustComponent(block.CName).(*block.Service)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.server = a.MustComponent(debugserver.CName).(debugserver.DebugServer)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	return clientdebugrpcproto.DRPCRegisterClientApi(s.server, &rpcHandler{
		spaceService:   s.spaceService,
		storageService: s.storage,
		blockService:   s.blockService,
		account:        s.account,
		file:           s.file,
	})
}

func (s *service) Close(ctx context.Context) (err error) {
	if !s.cfg.IsEnabled {
		return
	}
	return nil
}
