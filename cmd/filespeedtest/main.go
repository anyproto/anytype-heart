package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/net/rpc/encoding"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
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
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	"gopkg.in/yaml.v3"
	"storj.io/drpc/drpcconn"

	"github.com/anyproto/any-sync/net/connutil"
)

func main() {
	configPath := flag.String("config", "", "path to network YAML config")
	addr := flag.String("addr", "", "direct file node address (host:port)")
	transport := flag.String("transport", "quic", "transport protocol: quic or yamux")
	spaceId := flag.String("space", "", "space ID (required)")
	fileId := flag.String("file", "", "file ID / root CID (required)")
	workers := flag.Int("workers", 4, "parallel download workers")
	flag.Parse()

	if *spaceId == "" || *fileId == "" {
		fmt.Fprintln(os.Stderr, "usage: filespeedtest -space <spaceId> -file <fileId> [-config <nodeconf.yml> | -addr <address>] [-transport quic|yamux] [-workers N]")
		os.Exit(1)
	}
	if *transport != "quic" && *transport != "yamux" {
		fmt.Fprintln(os.Stderr, "error: -transport must be 'quic' or 'yamux'")
		os.Exit(1)
	}

	if err := run(*configPath, *addr, *transport, *spaceId, *fileId, *workers); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath, addr, transportProto, spaceId, fileId string, workers int) error {
	rootCid, err := cid.Decode(fileId)
	if err != nil {
		return fmt.Errorf("parse file CID: %w", err)
	}

	conf, err := loadNetworkConfig(configPath)
	if err != nil && addr == "" {
		return err
	}

	addresses, err := resolveAddresses(conf, addr, transportProto)
	if err != nil {
		return err
	}

	ctx := context.Background()
	a := new(app.App)
	bootstrap(a, conf)
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("start app: %w", err)
	}
	defer func() { _ = a.Close(ctx) }()

	var mc transport.MultiConn
	var connAddr string

	for _, address := range addresses {
		connAddr = address
		mc, err = dial(ctx, a, address, transportProto)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to connect to %s: %v\n", address, err)
			continue
		}
		break
	}
	if mc == nil {
		return fmt.Errorf("could not connect to any file node")
	}
	defer mc.Close()
	fmt.Printf("Connected to %s\n", connAddr)

	clients := make([]fileproto.DRPCFileClient, 0, workers)
	for i := 0; i < workers; i++ {
		cl, err := openFileClient(ctx, mc)
		if err != nil {
			return fmt.Errorf("open stream %d: %w", i, err)
		}
		clients = append(clients, cl)
	}
	fmt.Printf("Opened %d download streams\n\n", workers)

	return downloadFile(ctx, clients, spaceId, rootCid)
}

func loadNetworkConfig(configPath string) (*nodeconf.Configuration, error) {
	if configPath == "" {
		configPath = os.Getenv("ANY_SYNC_NETWORK")
	}
	if configPath == "" {
		return nil, fmt.Errorf("provide -config <nodeconf.yml> or -addr <address>")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var conf nodeconf.Configuration
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &conf, nil
}

func resolveAddresses(conf *nodeconf.Configuration, addr, transportProto string) ([]string, error) {
	if addr != "" {
		return []string{addr}, nil
	}
	if conf == nil {
		return nil, fmt.Errorf("provide -config <nodeconf.yml> or -addr <address>")
	}

	var addresses []string
	for _, node := range conf.Nodes {
		for _, t := range node.Types {
			if t == "file" {
				for _, a := range node.Addresses {
					isQuic := strings.HasPrefix(a, "quic://")
					if transportProto == "quic" && isQuic {
						addresses = append(addresses, a)
					} else if transportProto == "yamux" && !isQuic {
						addresses = append(addresses, a)
					}
				}
				break
			}
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no %s file node addresses found in config", transportProto)
	}
	return addresses, nil
}

func dial(ctx context.Context, a *app.App, addr, transportProto string) (transport.MultiConn, error) {
	if transportProto == "quic" {
		return dialQuic(ctx, a, strings.TrimPrefix(addr, "quic://"))
	}
	return dialYamux(ctx, a, addr)
}

func dialQuic(ctx context.Context, a *app.App, addr string) (transport.MultiConn, error) {
	qs := a.MustComponent(quic.CName).(quic.Quic)
	fmt.Printf("Dialing %s (QUIC)...\n", addr)
	mc, err := qs.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("quic dial: %w", err)
	}
	return mc, nil
}

func dialYamux(ctx context.Context, a *app.App, addr string) (transport.MultiConn, error) {
	ss := a.MustComponent(secureservice.CName).(secureservice.SecureService)
	fmt.Printf("Dialing %s (yamux/TCP)...\n", addr)

	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}

	cctx, err := ss.SecureOutbound(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("secure handshake: %w", err)
	}

	sess, err := yamux2.Client(conn, yamux2.DefaultConfig())
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("yamux session: %w", err)
	}

	mc := yamux.NewMultiConn(cctx, connutil.NewLastUsageConn(conn), conn.RemoteAddr().String(), sess)
	return mc, nil
}

func openFileClient(ctx context.Context, mc transport.MultiConn) (fileproto.DRPCFileClient, error) {
	sc, err := mc.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("open sub-conn: %w", err)
	}
	remoteProto, err := handshake.OutgoingProtoHandshake(ctx, sc, &handshakeproto.Proto{
		Proto:     handshakeproto.ProtoType_DRPC,
		Encodings: []handshakeproto.Encoding{handshakeproto.Encoding_Snappy, handshakeproto.Encoding_None},
	})
	if err != nil {
		sc.Close()
		return nil, fmt.Errorf("proto handshake: %w", err)
	}
	useSnappy := slices.Contains(remoteProto.Encodings, handshakeproto.Encoding_Snappy)
	conn := encoding.WrapConnEncoding(drpcconn.New(sc), useSnappy)
	return fileproto.NewDRPCFileClient(conn), nil
}

// downloadFile walks the CID DAG from rootCid, downloads all blocks, and prints speed stats.
func downloadFile(ctx context.Context, clients []fileproto.DRPCFileClient, spaceId string, rootCid cid.Cid) error {
	type blockResult struct {
		k     cid.Cid
		data  []byte
		links []cid.Cid
		err   error
	}

	jobs := make(chan cid.Cid, 256)
	results := make(chan blockResult, 256)

	// Start workers
	var wg sync.WaitGroup
	for _, cl := range clients {
		wg.Add(1)
		go func(cl fileproto.DRPCFileClient) {
			defer wg.Done()
			for k := range jobs {
				data, err := getBlock(ctx, cl, spaceId, k)
				var links []cid.Cid
				if err == nil {
					links = extractLinks(k, data)
				}
				results <- blockResult{k: k, data: data, links: links, err: err}
			}
		}(cl)
	}

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// BFS coordinator
	visited := map[cid.Cid]struct{}{rootCid: {}}
	queue := []cid.Cid{rootCid}
	inflight := 0
	var totalBytes int64
	var totalBlocks int64
	var errors int64

	start := time.Now()
	done := make(chan struct{})

	// Progress printer
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(start)
				bytes := atomic.LoadInt64(&totalBytes)
				blocks := atomic.LoadInt64(&totalBlocks)
				speed := float64(bytes) / elapsed.Seconds()
				fmt.Printf("\r  %d blocks | %s | %s/s | %s",
					blocks, formatBytes(bytes), formatBytes(int64(speed)), elapsed.Truncate(time.Millisecond))
			case <-done:
				return
			}
		}
	}()

	for len(queue) > 0 || inflight > 0 {
		// Try to send work to workers or receive results
		var sendCh chan cid.Cid
		var sendCid cid.Cid
		if len(queue) > 0 {
			sendCh = jobs
			sendCid = queue[0]
		}

		select {
		case sendCh <- sendCid:
			queue = queue[1:]
			inflight++
		case res, ok := <-results:
			if !ok {
				// workers finished (shouldn't happen while inflight > 0)
				goto finish
			}
			inflight--
			if res.err != nil {
				errors++
				fmt.Printf("\n  block %s: %v\n", res.k, res.err)
				continue
			}
			atomic.AddInt64(&totalBytes, int64(len(res.data)))
			atomic.AddInt64(&totalBlocks, 1)

			for _, link := range res.links {
				if _, ok := visited[link]; !ok {
					visited[link] = struct{}{}
					queue = append(queue, link)
				}
			}
		case <-ctx.Done():
			close(jobs)
			close(done)
			return ctx.Err()
		}
	}

finish:
	close(jobs)
	close(done)
	wg.Wait()

	elapsed := time.Since(start)
	bytes := atomic.LoadInt64(&totalBytes)
	blocks := atomic.LoadInt64(&totalBlocks)
	speed := float64(bytes) / elapsed.Seconds()

	fmt.Printf("\r\033[K") // clear progress line
	fmt.Println("---")
	fmt.Printf("Blocks:     %d\n", blocks)
	fmt.Printf("Downloaded: %s\n", formatBytes(bytes))
	fmt.Printf("Errors:     %d\n", errors)
	fmt.Printf("Duration:   %s\n", elapsed.Truncate(time.Millisecond))
	fmt.Printf("Speed:      %s/s\n", formatBytes(int64(speed)))
	return nil
}

func getBlock(ctx context.Context, cl fileproto.DRPCFileClient, spaceId string, k cid.Cid) ([]byte, error) {
	resp, err := cl.BlockGet(ctx, &fileproto.BlockGetRequest{
		SpaceId: spaceId,
		Cid:     k.Bytes(),
	})
	if err != nil {
		return nil, rpcerr.Unwrap(err)
	}
	return resp.Data, nil
}

func extractLinks(k cid.Cid, data []byte) []cid.Cid {
	if k.Prefix().Codec != cid.DagProtobuf {
		return nil
	}
	node, err := merkledag.DecodeProtobuf(data)
	if err != nil {
		return nil
	}
	links := node.Links()
	result := make([]cid.Cid, 0, len(links))
	for _, l := range links {
		result = append(result, l.Cid)
	}
	return result
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// --- Minimal app bootstrap (following netcheck.go pattern) ---

func bootstrap(a *app.App, conf *nodeconf.Configuration) {
	q := quic.New()
	nc := &toolNodeConf{}
	if conf != nil {
		nc.conf = *conf
	}
	a.Register(&toolConfig{}).
		Register(nc).
		Register(q).
		Register(&accounttest.AccountTestService{}).
		Register(secureservice.New())
	q.SetAccepter(new(nullAccepter))
}

type toolConfig struct{}

func (c toolConfig) Name() string          { return "config" }
func (c toolConfig) Init(_ *app.App) error { return nil }

func (c toolConfig) GetYamux() yamux.Config {
	return yamux.Config{
		WriteTimeoutSec:    60,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}

func (c toolConfig) GetQuic() quic.Config {
	return quic.Config{
		WriteTimeoutSec:    1200,
		DialTimeoutSec:     60,
		KeepAlivePeriodSec: 120,
	}
}

type toolNodeConf struct {
	conf nodeconf.Configuration
}

func (n *toolNodeConf) Id() string                            { return n.conf.Id }
func (n *toolNodeConf) Configuration() nodeconf.Configuration { return n.conf }
func (n *toolNodeConf) NodeIds(string) []string               { return nil }
func (n *toolNodeConf) IsResponsible(string) bool             { return false }
func (n *toolNodeConf) FilePeers() []string                   { return nil }
func (n *toolNodeConf) ConsensusPeers() []string              { return nil }
func (n *toolNodeConf) CoordinatorPeers() []string            { return nil }
func (n *toolNodeConf) NamingNodePeers() []string             { return nil }
func (n *toolNodeConf) PaymentProcessingNodePeers() []string  { return nil }
func (n *toolNodeConf) CHash() chash.CHash                   { return nil }
func (n *toolNodeConf) Partition(string) int                  { return 0 }
func (n *toolNodeConf) NetworkCompatibilityStatus() nodeconf.NetworkCompatibilityStatus { return 0 }
func (n *toolNodeConf) Init(_ *app.App) error                 { return nil }
func (n *toolNodeConf) Name() string                          { return nodeconf.CName }
func (n *toolNodeConf) Run(_ context.Context) error           { return nil }
func (n *toolNodeConf) Close(_ context.Context) error         { return nil }

func (n *toolNodeConf) PeerAddresses(peerId string) ([]string, bool) {
	for _, node := range n.conf.Nodes {
		if node.PeerId == peerId {
			return node.Addresses, true
		}
	}
	return nil, false
}

func (n *toolNodeConf) NodeTypes(nodeId string) []nodeconf.NodeType {
	for _, node := range n.conf.Nodes {
		if node.PeerId == nodeId {
			return node.Types
		}
	}
	return nil
}

type nullAccepter struct{}

func (nullAccepter) Accept(transport.MultiConn) error { return nil }
