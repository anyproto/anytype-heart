package network

import (
	"fmt"
	"net"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net/connutil"
	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/anyproto/any-sync/net/secureservice/handshake/handshakeproto"
	"github.com/anyproto/any-sync/net/transport"
	"github.com/anyproto/any-sync/net/transport/quic"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/testutil/accounttest"
	"github.com/anyproto/go-chash"
	yamux2 "github.com/hashicorp/yamux"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"storj.io/drpc/drpcconn"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-debug-network")

func probeYamux(ctx context.Context, a *app.App, requestIterations int, addr string) error {
	ss := a.MustComponent(secureservice.CName).(secureservice.SecureService)
	l := log.With(zap.String("addr", addr))

	l.Debug("open TCP conn")
	st := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Second*60)
	if err != nil {
		l.Warn("open TCP conn error", zap.Error(err), zap.Duration("dur", time.Since(st)))
		return fmt.Errorf("open TCP conn error: %w", err)
	} else {
		l.Debug("TCP conn established", zap.Duration("dur", time.Since(st)))
		l = l.With(zap.String("ip", conn.RemoteAddr().String()))
	}
	defer conn.Close()

	l.Debug("start handshake")
	hst := time.Now()
	cctx, err := ss.SecureOutbound(ctx, conn)
	if err != nil {
		l.Warn("handshake error", zap.Error(err), zap.Duration("dur", time.Since(hst)))
		return fmt.Errorf("handshake error: %w", err)
	} else {
		l.Debug("handshake success", zap.Duration("dur", time.Since(hst)), zap.Duration("total", time.Since(st)))
	}

	yst := time.Now()
	l.Debug("open yamux session")
	sess, err := yamux2.Client(conn, yamux2.DefaultConfig())
	if err != nil {
		l.Warn("yamux session error", zap.Error(err), zap.Duration("dur", time.Since(yst)))
		return fmt.Errorf("yamux session error: %w", err)
	} else {
		l.Debug("yamux session success", zap.Duration("dur", time.Since(yst)), zap.Duration("total", time.Since(st)))
	}

	mc := yamux.NewMultiConn(cctx, connutil.NewLastUsageConn(conn), conn.RemoteAddr().String(), sess)
	l.Debug("open sub connection")
	scst := time.Now()
	sc, err := mc.Open(ctx)
	if err != nil {
		l.Warn("open sub connection error", zap.Error(err), zap.Duration("dur", time.Since(scst)))
		return fmt.Errorf("open sub connection error: %w", err)
	} else {
		l.Debug("open sub conn success", zap.Duration("dur", time.Since(scst)), zap.Duration("total", time.Since(st)))
		defer sc.Close()
	}

	l.Debug("start proto handshake")
	phst := time.Now()
	if err = handshake.OutgoingProtoHandshake(ctx, sc, handshakeproto.ProtoType_DRPC); err != nil {
		l.Warn("proto handshake error", zap.Duration("dur", time.Since(phst)), zap.Error(err))
		return fmt.Errorf("proto handshake error: %w", err)
	} else {
		l.Debug("proto handshake success", zap.Duration("dur", time.Since(phst)), zap.Duration("total", time.Since(st)))
	}

	for i := 0; i < requestIterations; i++ {
		l.Debugf("start configuration request %d", i)
		rst := time.Now()
		resp, err := coordinatorproto.NewDRPCCoordinatorClient(drpcconn.New(sc)).NetworkConfiguration(ctx, &coordinatorproto.NetworkConfigurationRequest{})
		if err != nil {
			l.Warn("configuration request error", zap.Int("iter", i), zap.Error(err), zap.Duration("dur", time.Since(rst)))
			return fmt.Errorf("configuration request %d error: %w", i, err)
		} else {
			l.Debug("configuration request success", zap.Int("iter", i), zap.Duration("dur", time.Since(rst)), zap.Duration("total", time.Since(st)), zap.String("nid", resp.GetNetworkId()))
		}
	}
	l.Info("success", zap.Duration("dur", time.Since(st)))
	return nil
}

func probeQuic(ctx context.Context, a *app.App, requestIterations int, addr string) error {
	qs := a.MustComponent(quic.CName).(quic.Quic)
	l := log.With(zap.String("addr", addr))

	l.Debug("open QUIC conn")
	st := time.Now()
	mc, err := qs.Dial(ctx, addr)
	if err != nil {
		l.Warn("open QUIC conn error", zap.Error(err), zap.Duration("dur", time.Since(st)))
		return fmt.Errorf("open QUIC conn error: %w", err)
	} else {
		l.Debug("QUIC conn established", zap.Duration("dur", time.Since(st)))
		l = l.With(zap.String("ip", mc.Addr()))
	}
	defer mc.Close()

	l.Debug("open sub connection")
	scst := time.Now()
	sc, err := mc.Open(ctx)
	if err != nil {
		l.Warn("open sub connection error", zap.Error(err), zap.Duration("dur", time.Since(scst)))
		return fmt.Errorf("open sub connection error: %w", err)
	} else {
		l.Debug("open sub conn success", zap.Duration("dur", time.Since(scst)), zap.Duration("total", time.Since(st)))
		defer sc.Close()
	}

	l.Debug("start proto handshake")
	phst := time.Now()
	if err = handshake.OutgoingProtoHandshake(ctx, sc, handshakeproto.ProtoType_DRPC); err != nil {
		l.Warn("proto handshake error", zap.Duration("dur", time.Since(phst)), zap.Error(err))
		return fmt.Errorf("proto handshake error: %w", err)
	} else {
		l.Debug("proto handshake success", zap.Duration("dur", time.Since(phst)), zap.Duration("total", time.Since(st)))
	}

	for i := 0; i < requestIterations; i++ {
		l.Debugf("start configuration request %d", i)
		rst := time.Now()
		resp, err := coordinatorproto.NewDRPCCoordinatorClient(drpcconn.New(sc)).NetworkConfiguration(ctx, &coordinatorproto.NetworkConfigurationRequest{})
		if err != nil {
			l.Warn("configuration request error", zap.Int("iter", i), zap.Error(err), zap.Duration("dur", time.Since(rst)))
			return fmt.Errorf("configuration request %d error: %w", i, err)
		} else {
			l.Debug("configuration request success", zap.Int("iter", i), zap.Duration("dur", time.Since(rst)), zap.Duration("total", time.Since(st)), zap.String("nid", resp.GetNetworkId()))
		}
	}
	l.Info("success", zap.Duration("dur", time.Since(st)))
	return nil
}

func bootstrap(cfg2 *config.Config, a *app.App) {
	q := quic.New()
	a.Register(cfg{}).
		Register(q).
		Register(&nodeConf{conf: cfg2.GetNodeConf()}).
		Register(&accounttest.AccountTestService{}).
		Register(secureservice.New())
	q.SetAccepter(new(accepter))
}

type accepter struct {
}

func (a accepter) Accept(mc transport.MultiConn) (err error) {
	return nil
}

type nodeConf struct {
	conf nodeconf.Configuration
}

func (n nodeConf) Id() string {
	return "test"
}

func (n nodeConf) Configuration() nodeconf.Configuration {
	return n.conf
}

func (n nodeConf) NodeIds(spaceId string) []string {
	return nil
}

func (n nodeConf) IsResponsible(spaceId string) bool {
	return false
}

func (n nodeConf) FilePeers() []string {
	return nil
}

func (n nodeConf) ConsensusPeers() []string {
	return nil
}

func (n nodeConf) CoordinatorPeers() []string {
	return nil
}

func (n nodeConf) PeerAddresses(peerId string) (addrs []string, ok bool) {
	return nil, false
}

func (n nodeConf) CHash() chash.CHash {
	return nil
}

func (n nodeConf) Partition(spaceId string) (part int) {
	return 0
}

func (n nodeConf) NodeTypes(nodeId string) []nodeconf.NodeType {
	return []nodeconf.NodeType{nodeconf.NodeTypeCoordinator}
}

func (n nodeConf) NetworkCompatibilityStatus() nodeconf.NetworkCompatibilityStatus {
	return 0
}

func (n nodeConf) Init(a *app.App) (err error) {
	return nil
}

func (n nodeConf) Name() (name string) {
	return nodeconf.CName
}

func (n nodeConf) Run(ctx context.Context) (err error) {
	return nil
}

func (n nodeConf) Close(ctx context.Context) (err error) {
	return nil
}

func (c nodeConf) NamingNodePeers() []string {
	return []string{}
}

func (c nodeConf) PaymentProcessingNodePeers() []string {
	return []string{}
}

type cfg struct {
}

func (c cfg) Name() string          { return "config" }
func (c cfg) Init(a *app.App) error { return nil }

func (c cfg) GetYamux() yamux.Config {
	return yamux.Config{
		WriteTimeoutSec:    60,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}

func (c cfg) GetQuic() quic.Config {
	return quic.Config{
		WriteTimeoutSec:    60,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}
