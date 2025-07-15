package debug

import (
	"context"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net/connutil"
	"github.com/anyproto/any-sync/net/rpc/encoding"
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
	"github.com/matishsiao/goInfo"
	"gopkg.in/yaml.v3"
	"storj.io/drpc/drpcconn"
)

func (d *debug) NetCheck(ctx context.Context, clientYml string) (string, error) {
	var sb strings.Builder
	var checkAddrs []string
	if clientYml != "" {
		file, err := os.ReadFile(clientYml)
		if err != nil {
			return "", err
		}

		var configFile nodeconf.Configuration
		if err := yaml.Unmarshal(file, &configFile); err != nil {
			return "", err
		}

		for _, node := range configFile.Nodes {
			for _, t := range node.Types {
				if t == "coordinator" {
					for _, address := range node.Addresses {
						if !strings.HasPrefix(address, "quic://") {
							address = "yamux://" + address
						}
						checkAddrs = append(checkAddrs, address)
					}
				}
			}
		}
	} else {
		checkAddrs = strings.Split(defaultAddrs, ",")
	}
	info, err := goInfo.GetInfo()
	if err != nil {
		sb.WriteString(fmt.Sprintf("error getting system info: %s\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("system info: %s\n", info.String()))
	}

	a := new(app.App)
	bootstrap(a)
	if err := a.Start(ctx); err != nil {
		panic(err)
	}

	for _, addr := range checkAddrs {
		addr = strings.TrimSpace(addr)
		switch {
		case strings.HasPrefix(addr, "yamux://"):
			res, err := probeYamux(ctx, a, addr[8:])
			if err != nil {
				sb.WriteString(fmt.Sprintf("error probing yamux %s: %s\n", addr, err))
			} else {
				sb.WriteString(res)
			}
		case strings.HasPrefix(addr, "quic://"):
			res, err := probeQuic(ctx, a, addr[7:])
			if err != nil {
				sb.WriteString(fmt.Sprintf("error probing yamux %s: %s\n", addr, err))
			} else {
				sb.WriteString(res)
			}
		default:
			return "", fmt.Errorf("unexpected address scheme: %s", addr)
		}
	}
	return sb.String(), err
}

func probeYamux(ctx context.Context, a *app.App, addr string) (string, error) {
	var sb strings.Builder
	ss := a.MustComponent(secureservice.CName).(secureservice.SecureService)
	sb.WriteString(fmt.Sprintf("open TCP conn, addr: %s\n", addr))
	st := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Second*60)
	if err != nil {
		return "", fmt.Errorf("open TCP conn error: %w, dur: %s", err, time.Since(st))
	} else {
		sb.WriteString(fmt.Sprintf("TCP conn established, ip:%s, dur: %s\n", conn.RemoteAddr().String(), time.Since(st)))
	}
	defer conn.Close()

	sb.WriteString("start handshake\n")
	hst := time.Now()
	cctx, err := ss.SecureOutbound(ctx, conn)
	if err != nil {
		return "", fmt.Errorf("handshake error: %w, dur: %s", err, time.Since(hst))
	} else {
		sb.WriteString(fmt.Sprintf("handshake success, dur: %s, total: %s\n", time.Since(hst), time.Since(st)))
	}

	yst := time.Now()
	sb.WriteString("open yamux session\n")
	sess, err := yamux2.Client(conn, yamux2.DefaultConfig())
	if err != nil {
		return "", fmt.Errorf("yamux session error: %w, dur: %s", err, time.Since(yst))
	} else {
		sb.WriteString(fmt.Sprintf("yamux session success, dur: %s, total: %s\n", time.Since(yst), time.Since(st)))
	}

	mc := yamux.NewMultiConn(cctx, connutil.NewLastUsageConn(conn), conn.RemoteAddr().String(), sess)
	sb.WriteString("open sub connection\n")
	scst := time.Now()
	sc, err := mc.Open(ctx)
	if err != nil {
		return "", fmt.Errorf("open sub connection error: %w, dur: %s", err, time.Since(scst))
	} else {
		sb.WriteString(fmt.Sprintf("open sub conn success, dur: %s, total: %s\n", time.Since(scst), time.Since(st)))
		defer sc.Close()
	}

	sb.WriteString("start proto handshake\n")
	phst := time.Now()
	var remoteProto *handshakeproto.Proto
	if remoteProto, err = handshake.OutgoingProtoHandshake(ctx, sc, &handshakeproto.Proto{
		Proto:     handshakeproto.ProtoType_DRPC,
		Encodings: []handshakeproto.Encoding{handshakeproto.Encoding_Snappy, handshakeproto.Encoding_None},
	}); err != nil {
		return "", fmt.Errorf("proto handshake error: %w, dur: %s", err, time.Since(phst))
	} else {
		sb.WriteString(fmt.Sprintf("proto handshake success, dur: %s, total: %s\n", time.Since(phst), time.Since(st)))
	}

	sb.WriteString("start configuration request\n")
	rst := time.Now()
	useSnappy := slices.Contains(remoteProto.Encodings, handshakeproto.Encoding_Snappy)
	resp, err := coordinatorproto.NewDRPCCoordinatorClient(encoding.WrapConnEncoding(drpcconn.New(sc), useSnappy)).NetworkConfiguration(ctx, &coordinatorproto.NetworkConfigurationRequest{})
	if err != nil {
		return "", fmt.Errorf("configuration request error: %w, dur: %s", err, time.Since(rst))
	} else {
		sb.WriteString(fmt.Sprintf("configuration request success, dur: %s, total: %s, nid: %s\n", time.Since(rst), time.Since(st), resp.GetNetworkId()))
	}
	sb.WriteString(fmt.Sprintf("success, dur: %s\n", time.Since(st)))
	return sb.String(), nil
}

func probeQuic(ctx context.Context, a *app.App, addr string) (string, error) {
	var sb strings.Builder
	qs := a.MustComponent(quic.CName).(quic.Quic)
	sb.WriteString(fmt.Sprintf("open QUIC conn, addr: %s\n", addr))
	st := time.Now()
	mc, err := qs.Dial(ctx, addr)
	if err != nil {
		return "", fmt.Errorf("open QUIC conn error: %w", err)
	} else {
		sb.WriteString(fmt.Sprintf("QUIC conn established, ip:%s, dur: %s\n", mc.Addr(), time.Since(st)))
	}
	defer mc.Close()

	sb.WriteString("open sub connection\n")
	scst := time.Now()
	sc, err := mc.Open(ctx)
	if err != nil {
		return "", fmt.Errorf("open sub connection error: %w", err)
	} else {
		sb.WriteString(fmt.Sprintf("open sub conn success, dur: %s, total: %s\n", time.Since(scst), time.Since(st)))
		defer sc.Close()
	}

	sb.WriteString("start proto handshake\n")
	phst := time.Now()
	var remoteProto *handshakeproto.Proto
	if remoteProto, err = handshake.OutgoingProtoHandshake(ctx, sc, &handshakeproto.Proto{
		Proto:     handshakeproto.ProtoType_DRPC,
		Encodings: []handshakeproto.Encoding{handshakeproto.Encoding_Snappy, handshakeproto.Encoding_None},
	}); err != nil {
		return "", fmt.Errorf("proto handshake error: %w", err)
	} else {
		sb.WriteString(fmt.Sprintf("proto handshake success, dur: %s, total: %s\n", time.Since(phst), time.Since(st)))
	}

	sb.WriteString("start configuration request\n")
	rst := time.Now()
	useSnappy := slices.Contains(remoteProto.Encodings, handshakeproto.Encoding_Snappy)
	resp, err := coordinatorproto.NewDRPCCoordinatorClient(encoding.WrapConnEncoding(drpcconn.New(sc), useSnappy)).NetworkConfiguration(ctx, &coordinatorproto.NetworkConfigurationRequest{})
	if err != nil {
		return "", fmt.Errorf("configuration request error: %w", err)
	} else {
		sb.WriteString(fmt.Sprintf("configuration request success, dur: %s, total: %s, nid: %s\n", time.Since(rst), time.Since(st), resp.GetNetworkId()))
	}
	sb.WriteString(fmt.Sprintf("success, dur: %s\n", time.Since(st)))
	return sb.String(), nil
}

func bootstrap(a *app.App) {
	q := quic.New()
	a.Register(&config{}).
		Register(&nodeConf{}).
		Register(q).
		Register(&accounttest.AccountTestService{}).
		Register(secureservice.New())
	q.SetAccepter(new(accepter))
}

type config struct {
}

func (c config) Name() string          { return "config" }
func (c config) Init(a *app.App) error { return nil }

func (c config) GetYamux() yamux.Config {
	return yamux.Config{
		WriteTimeoutSec:    60,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}

func (c config) GetQuic() quic.Config {
	return quic.Config{
		WriteTimeoutSec:    1200,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}

type nodeConf struct {
}

func (n nodeConf) Id() string {
	return "test"
}

func (n nodeConf) Configuration() nodeconf.Configuration {
	return nodeconf.Configuration{
		Id:           "test",
		NetworkId:    "",
		Nodes:        nil,
		CreationTime: time.Time{},
	}
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

func (n nodeConf) NamingNodePeers() []string {
	return nil
}

func (n nodeConf) PaymentProcessingNodePeers() []string {
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

type accepter struct {
}

func (a accepter) Accept(mc transport.MultiConn) (err error) {
	return nil
}
