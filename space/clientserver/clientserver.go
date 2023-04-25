package clientserver

import (
	"context"
	"errors"
	"fmt"
	gonet "net"
	"strconv"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/net"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/anytypeio/any-sync/net/secureservice"
	"github.com/dgraph-io/badger/v3"
	"github.com/libp2p/go-libp2p/core/sec"
	"storj.io/drpc"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
)

const CName = server.CName

var log = logger.NewNamed(CName)

var ErrNoPortAssigned = errors.New("no port assigned to the server")

func New() DRPCServer {
	return &drpcServer{BaseDrpcServer: server.NewBaseDrpcServer()}
}

type DRPCServer interface {
	app.ComponentRunnable
	drpc.Mux
	Port() int
}

type drpcServer struct {
	config    net.Config
	transport secureservice.SecureService
	provider  datastore.Datastore
	port      int
	storage   *portStorage
	*server.BaseDrpcServer
}

func (s *drpcServer) Init(a *app.App) (err error) {
	s.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	s.config = a.MustComponent("config").(net.ConfigGetter).GetNet()
	s.transport = a.MustComponent(secureservice.CName).(secureservice.SecureService)
	return nil
}

func (s *drpcServer) Name() (name string) {
	return CName
}

func (s *drpcServer) Run(ctx context.Context) (err error) {
	db, err := s.provider.SpaceStorage()
	if err != nil {
		return
	}
	s.storage = &portStorage{db}
	oldPort, err := s.storage.getPort()
	if err != nil && err != badger.ErrKeyNotFound {
		return
	}
	var updatedAddrs []string
	if err == nil {
		for _, addr := range s.config.Server.ListenAddrs {
			split := strings.Split(addr, ":")
			updatedAddrs = append(updatedAddrs, fmt.Sprintf("%s:%d", split[0], oldPort))
		}
	} else {
		updatedAddrs = s.config.Server.ListenAddrs
	}
	params := server.Params{
		BufferSizeMb:  s.config.Stream.MaxMsgSizeMb,
		TimeoutMillis: s.config.Stream.TimeoutMilliseconds,
		ListenAddrs:   updatedAddrs,
		Wrapper: func(handler drpc.Handler) drpc.Handler {
			return handler
		},
		Handshake: func(conn gonet.Conn) (cCtx context.Context, sc sec.SecureConn, err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			return s.transport.SecureInbound(ctx, conn)
		},
	}
	// TODO: the logic must be written so that server wouldn't be mandatory for client to work
	err = s.BaseDrpcServer.Run(ctx, params)
	if err != nil {
		// listening random port
		params.ListenAddrs = []string{":0"}
		err = s.BaseDrpcServer.Run(ctx, params)
		if err != nil {
			return
		}
	}
	s.port, err = s.parsePort()
	if err != nil {
		return
	}
	return s.storage.setPort(s.port)
}

func (s *drpcServer) Port() int {
	return s.port
}

func (s *drpcServer) parsePort() (int, error) {
	addrs := s.BaseDrpcServer.ListenAddrs()
	if len(addrs) == 0 {
		return 0, ErrNoPortAssigned
	}
	split := strings.Split(addrs[0].String(), ":")
	if len(split) <= 1 {
		return 0, ErrNoPortAssigned
	}
	return strconv.Atoi(split[len(split)-1])
}

func (s *drpcServer) Close(ctx context.Context) (err error) {
	return s.BaseDrpcServer.Close(ctx)
}
