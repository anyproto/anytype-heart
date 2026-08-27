package clientserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/transport/quic"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const CName = "client.space.clientserver"

var log = logger.NewNamed(CName)

var ErrNoPortAssigned = errors.New("no port assigned to the server")

// listenAttempts bounds retries when the UDP side of a freshly reserved port
// turns out to be occupied: first try the saved port, then ephemeral ports.
const listenAttempts = 4

func New() ClientServer {
	return &clientServer{}
}

type ClientServer interface {
	app.ComponentRunnable
	Port() int
	ServerStarted() bool
}

// udpListener is the part of quic.Quic used here; narrowed for tests.
type udpListener interface {
	ListenAddrs(ctx context.Context, addrs ...string) ([]net.Addr, error)
}

// tcpListenerRegistry is the part of yamux.Yamux used here; narrowed for tests.
type tcpListenerRegistry interface {
	AddListener(lis net.Listener)
}

// keep the narrowed interfaces in sync with any-sync at compile time, so an
// upstream API change fails the build instead of panicking in Init
var (
	_ udpListener         = (quic.Quic)(nil)
	_ tcpListenerRegistry = (yamux.Yamux)(nil)
)

type clientServer struct {
	quic          udpListener
	yamux         tcpListenerRegistry
	provider      anystoreprovider.Provider
	port          int
	storage       keyvaluestore.Store[int]
	serverStarted bool
}

func (s *clientServer) Init(a *app.App) (err error) {
	s.provider = app.MustComponent[anystoreprovider.Provider](a)
	s.quic = a.MustComponent(quic.CName).(udpListener)
	s.yamux = a.MustComponent(yamux.CName).(tcpListenerRegistry)
	return nil
}

func (s *clientServer) Name() (name string) {
	return CName
}

func (s *clientServer) Run(ctx context.Context) error {
	if err := s.startServer(ctx); err != nil {
		// the app stays up without local P2P, but both local listeners are
		// missing — warn, don't bury it at info
		log.WarnCtx(ctx, "failed to start local p2p server", zap.Error(err))
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
		return fmt.Errorf("get saved port: %w", err)
	}
	s.port, err = s.listen(ctx, oldPort)
	if err != nil {
		return fmt.Errorf("bind local listeners: %w", err)
	}
	if oldPort == s.port {
		return nil
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

// listen binds both local transports on one shared port: TCP for yamux and
// UDP for QUIC. The TCP listener doubles as the port reservation. A free TCP
// port does not guarantee a free UDP port, so on a UDP bind failure the
// reservation is dropped and another port is tried.
func (s *clientServer) listen(ctx context.Context, savedPort int) (port int, err error) {
	tryPort := savedPort
	for i := 0; i < listenAttempts; i++ {
		port, err = s.listenPort(ctx, tryPort)
		if err == nil {
			return port, nil
		}
		tryPort = 0
	}
	return 0, err
}

func (s *clientServer) listenPort(ctx context.Context, tryPort int) (port int, err error) {
	list, err := s.prepareListener(tryPort)
	if err != nil {
		return 0, fmt.Errorf("listen tcp: %w", err)
	}
	port, err = s.parsePort(list.Addr().String())
	if err != nil {
		_ = list.Close()
		return 0, fmt.Errorf("parse tcp listener port: %w", err)
	}
	addrs, err := s.quic.ListenAddrs(ctx, "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		_ = list.Close()
		return 0, fmt.Errorf("listen udp on port %d: %w", port, err)
	}
	port, err = s.parsePort(addrs[0].String())
	if err != nil {
		_ = list.Close()
		return 0, fmt.Errorf("parse udp listener port: %w", err)
	}
	// Hand the TCP listener to the yamux transport so local peers can dial us
	// over TCP as well. AddListener starts the accept loop itself, and
	// yamux.Run starts one per registered listener — so this must run after
	// yamux.Run. Component registration order guarantees it: yamux is
	// registered before clientserver, and this is only called from Run. It is
	// also the last step of an attempt, so a failed attempt never registers.
	s.yamux.AddListener(list)
	return port, nil
}

func (s *clientServer) Close(_ context.Context) (err error) {
	return nil
}
