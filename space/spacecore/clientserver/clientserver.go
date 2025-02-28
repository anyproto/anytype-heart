package clientserver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/transport"
	"github.com/anyproto/any-sync/net/transport/quic"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const (
	CName           = "client.space.clientserver"
	PreferredSchema = transport.Quic
)

var log = logger.NewNamed(CName)

var ErrNoPortAssigned = errors.New("no port assigned to the server")

func New() ClientServer {
	return &clientServer{}
}

type ClientServer interface {
	app.ComponentRunnable
	Port() int
	ServerStarted() bool
}

type DbProvider interface {
	GetCommonDb() anystore.DB
}

type clientServer struct {
	quic          quic.Quic
	provider      anystoreprovider.Provider
	port          int
	storage       keyvaluestore.Store[int]
	serverStarted bool
}

func (s *clientServer) Init(a *app.App) (err error) {
	s.provider = app.MustComponent[anystoreprovider.Provider](a)
	s.quic = a.MustComponent(quic.CName).(quic.Quic)
	return nil
}

func (s *clientServer) Name() (name string) {
	return CName
}

func (s *clientServer) Run(ctx context.Context) error {
	if err := s.startServer(ctx); err != nil {
		log.InfoCtx(ctx, "failed to start drpc server", zap.Error(err))
	} else {
		s.serverStarted = true
	}
	return nil
}

func (s *clientServer) Port() int {
	return s.port
}

func (s *clientServer) startServer(ctx context.Context) (err error) {
	s.storage = keyvaluestore.NewJsonFromCollection[int](s.provider.GetSystemCollection())

	oldPort, err := s.storage.Get(ctx, anystoreprovider.SystemKeys.PortKey())
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return
	}
	s.port, err = s.listenQuic(ctx, oldPort)
	if err != nil {
		return
	}
	return s.storage.Set(ctx, anystoreprovider.SystemKeys.PortKey(), s.port)
}

func (s *clientServer) parsePort(addr string) (int, error) {
	split := strings.Split(addr, ":")
	if len(split) <= 1 {
		return 0, ErrNoPortAssigned
	}
	return strconv.Atoi(split[len(split)-1])
}

func (s *clientServer) ServerStarted() bool {
	return s.serverStarted
}

func (s *clientServer) prepareListener(port int) (net.Listener, error) {
	if port != 0 {
		list, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			return list, nil
		}
	}
	// otherwise listening to new port
	// nolint: gosec
	return net.Listen("tcp", ":")
}

func (s *clientServer) listenQuic(ctx context.Context, savedPort int) (port int, err error) {
	// trying to listen to old port or get new one
	list, err := s.prepareListener(savedPort)
	if err != nil {
		return
	}
	port, err = s.parsePort(list.Addr().String())
	if err != nil {
		return
	}
	_ = list.Close()
	addrs, err := s.quic.ListenAddrs(ctx, "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		return
	}
	return s.parsePort(addrs[0].String())
}

func (s *clientServer) Close(_ context.Context) (err error) {
	return nil
}
