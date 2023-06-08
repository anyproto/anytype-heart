package clientserver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
)

const CName = "client.space.clientserver"

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

type clientServer struct {
	yamux         yamux.Yamux
	provider      datastore.Datastore
	port          int
	storage       *portStorage
	serverStarted bool
}

func (s *clientServer) Init(a *app.App) (err error) {
	s.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	s.yamux = a.MustComponent(yamux.CName).(yamux.Yamux)
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
	db, err := s.provider.SpaceStorage()
	if err != nil {
		return
	}
	s.storage = &portStorage{db}
	oldPort, err := s.storage.getPort()
	if err != nil && err != badger.ErrKeyNotFound {
		return
	}
	list, err := s.prepareListener(oldPort)
	if err != nil {
		return
	}
	s.port, err = s.parsePort(list.Addr().String())
	if err != nil {
		return
	}
	s.yamux.AddListener(list)
	return s.storage.setPort(s.port)
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
	//nolint: gosec
	return net.Listen("tcp", ":")
}

func (s *clientServer) Close(ctx context.Context) (err error) {
	return nil
}
