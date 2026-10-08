package rpcstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	gonet "net"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
	"github.com/anyproto/any-sync/net/transport"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

var closedBlockChan chan blocks.Block

var (
	ErrUnsupported                 = errors.New("unsupported operation")
	ErrNoConnectionToAnyFileClient = errors.New("no connection to any file client")
)

func init() {
	closedBlockChan = make(chan blocks.Block)
	close(closedBlockChan)
}

type ctxKey string

const CtxWaitAvailable = ctxKey("waitAvailable")

func ContextWithWaitAvailable(ctx context.Context) context.Context {
	return context.WithValue(ctx, CtxWaitAvailable, true)
}

func IsWaitWhenAvailable(ctx context.Context) bool {
	wait, ok := ctx.Value(CtxWaitAvailable).(bool)
	return ok && wait
}

type RpcStore interface {
	fileblockstore.BlockStore

	CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) (checkResult []*fileproto.BlockAvailability, err error)
	BindCids(ctx context.Context, spaceID string, fileId domain.FileId, cids []cid.Cid) (err error)

	AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) error
	AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) (err error)
	DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) (err error)
	SpaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error)
	FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error)
	AccountInfo(ctx context.Context) (info *fileproto.AccountInfoResponse, err error)
	IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error
}

const getManyWorkers = 4

// Local peer (p2p) block fetching. See
// docs/superpowers/specs/2026-10-08-local-peer-fetch-progress-design.md.
//
// var (not const) so the real-transport integration test can shorten them.
var (
	// localPeerConnectTimeout bounds one attempt to get a usable sub-conn to
	// one local peer: pool lookup/dial (TCP or QUIC + secure handshake) plus
	// sub-conn open + proto handshake. One budget for both stages.
	localPeerConnectTimeout = 5 * time.Second
	// localPeerStallTimeout: a fetch is cancelled when the sub-conn's
	// BytesRead has not advanced for this long. On yamux the counter moves in
	// 64 KiB drpc frames, so this is also a ~105 kbit/s throughput floor.
	localPeerStallTimeout = 5 * time.Second
	// localPeerStallCheckInterval is the watchdog tick.
	localPeerStallCheckInterval = time.Second
	// localPeerFetchFallbackTimeout bounds a fetch on a conn that does not
	// expose BytesRead (foreign peer implementations): 1 MiB at ~300 kbit/s.
	localPeerFetchFallbackTimeout = 30 * time.Second
	// localPeerBanMin is the first ban; every strike after the previous ban
	// expired doubles it up to localPeerBanMax (failures during an active ban
	// are the same outage and do not escalate). A successful fetch, or
	// localPeerBanMax of quiet after a ban expired, resets the strike count.
	localPeerBanMin = 10 * time.Second
	localPeerBanMax = 5 * time.Minute
)

var (
	errNoLocalPeers = errors.New("no local peers available")
	// errLocalPeerStalled is the cause the watchdog cancels a fetch with.
	errLocalPeerStalled = errors.New("local peer fetch stalled: no bytes received")
)

// localPeerBan is the backoff state of one local peer
type localPeerBan struct {
	until   time.Time
	strikes int
}

// bytesReader is what any-sync's sub-conn (net/peer subConn, which embeds
// connutil.LastUsageConn) exposes: raw bytes read from the sub-stream.
type bytesReader interface {
	BytesRead() int64
}

// reservedCallTimeout bounds a single doNodeReserved RPC. The reserved
// sub-connection is shared and serialized by s.mu, so a stuck call would
// block every other metadata RPC (SpaceInfo, AccountInfo, BlocksCheck,
// FilesInfo). All RPCs routed through doNodeReserved are metadata-only
// and complete in well under a second on healthy infra; 15s is generous
// headroom that still bounds queue growth when the server is degraded.
//
// var (not const) so tests can shorten it.
var reservedCallTimeout = 15 * time.Second

type store struct {
	pool      pool.Pool
	peerStore peerstore.PeerStore

	mu           sync.Mutex
	reservedConn drpc.Conn

	bannedMu       sync.Mutex
	bannedLocalMap map[string]localPeerBan

	// shuffleLocalPeers randomises the candidate order (pool.GetOneOf
	// parity); tests replace it with a no-op
	shuffleLocalPeers func(ids []string)
}

func newStore(pool pool.Pool, peerStore peerstore.PeerStore) *store {
	return &store{
		pool:           pool,
		peerStore:      peerStore,
		bannedLocalMap: make(map[string]localPeerBan),
		shuffleLocalPeers: func(ids []string) {
			rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		},
	}
}

// strikeLocalPeer records a failure that is evidence against the peer and
// bans it with exponential backoff. Failures while a ban is active neither
// escalate nor extend it: they are the same outage seen by concurrent callers
// (four GetMany workers cut by one pool.Flush, or stalled on one dead link).
func (s *store) strikeLocalPeer(peerId, phase string, cause error) {
	s.bannedMu.Lock()
	defer s.bannedMu.Unlock()
	now := time.Now()
	ban := s.bannedLocalMap[peerId]
	if now.Before(ban.until) {
		log.Debug("local peer failure during an active ban",
			zap.String("peerId", peerId),
			zap.String("phase", phase),
			zap.Int("strikes", ban.strikes),
			zap.Error(cause))
		return
	}
	if ban.strikes > 0 && now.After(ban.until.Add(localPeerBanMax)) {
		// quiet for a whole max period after the last ban expired: start over
		ban.strikes = 0
	}
	ban.strikes++
	dur := localPeerBanMax
	if shift := ban.strikes - 1; shift < 10 && localPeerBanMin<<shift < localPeerBanMax {
		dur = localPeerBanMin << shift
	}
	ban.until = now.Add(dur)
	s.bannedLocalMap[peerId] = ban
	log.Info("local peer struck, banning",
		zap.String("peerId", peerId),
		zap.String("phase", phase),
		zap.Int("strikes", ban.strikes),
		zap.Duration("ban", dur),
		zap.Error(cause))
}

// resetLocalPeer forgets a peer's strikes after a successful fetch
func (s *store) resetLocalPeer(peerId string) {
	s.bannedMu.Lock()
	defer s.bannedMu.Unlock()
	delete(s.bannedLocalMap, peerId)
}

// filterBannedPeers drops the peers whose ban is active. Expired entries are
// kept: the strike count must survive expiry for the backoff to grow.
func (s *store) filterBannedPeers(peerIds []string) []string {
	s.bannedMu.Lock()
	defer s.bannedMu.Unlock()
	now := time.Now()
	result := make([]string, 0, len(peerIds))
	for _, id := range peerIds {
		if ban, ok := s.bannedLocalMap[id]; ok && now.Before(ban.until) {
			continue
		}
		result = append(result, id)
	}
	return result
}

// getNodePeer returns a peer from ResponsibleFilePeers via pool.GetOneOf.
func (s *store) getNodePeer(ctx context.Context) (peer.Peer, error) {
	peerIds := s.peerStore.ResponsibleFilePeers()
	if len(peerIds) == 0 {
		return nil, ErrNoConnectionToAnyFileClient
	}
	return s.pool.GetOneOf(ctx, peerIds)
}

// doNodeDrpc borrows a sub-connection from a node peer, executes the RPC, and returns it.
// Used for big-payload operations (block data transfers).
func (s *store) doNodeDrpc(ctx context.Context, do func(cl fileproto.DRPCFileClient) error) error {
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return fmt.Errorf("get node peer: %w", err)
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		return do(fileproto.NewDRPCFileClient(conn))
	})
}

// doNodeReserved uses a lazily-acquired reserved sub-connection for small-payload operations.
// A mutex serializes access since DRPC connections support only one concurrent stream.
//
// The do callback receives a context bounded by reservedCallTimeout so that a single
// stuck RPC cannot wedge every other caller queued on s.mu. On timeout we reset the
// reserved connection so the next caller starts with a fresh sub-conn.
func (s *store) doNodeReserved(ctx context.Context, do func(ctx context.Context, cl fileproto.DRPCFileClient) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureReservedConn(ctx); err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, reservedCallTimeout)
	defer cancel()
	err := do(callCtx, fileproto.NewDRPCFileClient(s.reservedConn))
	if err != nil {
		// Connection may be broken (or stuck past the deadline); reset for reconnect on next call.
		// Distinguish a deadline-induced reset from a real RPC error so we can spot it in logs:
		// the deadline only fires when callCtx expired but the parent ctx did not.
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			log.Warn("doNodeReserved: reserved call exceeded deadline, resetting conn",
				zap.Duration("timeout", reservedCallTimeout))
		}
		s.resetReservedConnLocked()
		return err
	}
	return nil
}

func (s *store) ensureReservedConn(ctx context.Context) error {
	if s.reservedConn != nil {
		select {
		case <-s.reservedConn.Closed():
			s.reservedConn = nil

		default:
			return nil
		}
	}
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return fmt.Errorf("get node peer for reserved conn: %w", err)
	}
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return fmt.Errorf("acquire reserved conn: %w", err)
	}
	s.reservedConn = conn
	return nil
}

func (s *store) resetReservedConnLocked() {
	s.reservedConn = nil
}

// Get retrieves a block, trying local peers first then falling back to the node peer.
func (s *store) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	spaceId := fileblockstore.CtxGetSpaceId(ctx)
	data, err := s.getLocalThenNode(ctx, spaceId, k)
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(data, k)
}

func (s *store) getLocalThenNode(ctx context.Context, spaceId string, k cid.Cid) ([]byte, error) {
	data, err := s.getFromLocalPeers(ctx, spaceId, k)
	if err == nil {
		return data, nil
	}
	// no fallback log when every local peer is absent or banned: that is the
	// normal state without LAN peers and would log once per block
	if !errors.Is(err, errNoLocalPeers) {
		log.Debug("local peer block get failed, trying node", zap.String("cid", k.String()), zap.Error(err))
	}
	return s.getFromNodePeer(ctx, spaceId, k)
}

// getFromLocalPeers fetches a block from a local (LAN) peer. Only connecting
// is bounded by a timeout; the fetch itself runs under the caller's ctx and
// is cancelled by a watchdog when no bytes arrive for localPeerStallTimeout.
// Failures that say something about the peer strike it (see
// connectLocalPeer and isLocalPeerFetchFailure); a caller that ends the
// call never does.
func (s *store) getFromLocalPeers(ctx context.Context, spaceId string, k cid.Cid) ([]byte, error) {
	localPeerIds := s.filterBannedPeers(s.peerStore.LocalPeerIds(spaceId))
	if len(localPeerIds) == 0 {
		return nil, fmt.Errorf("get from local peers: %w", errNoLocalPeers)
	}
	p, conn, err := s.connectLocalPeer(ctx, localPeerIds)
	if err != nil {
		return nil, err
	}
	data, err := s.fetchBlockFromLocalPeer(ctx, p, conn, spaceId, k)
	if err != nil {
		if ctx.Err() == nil && isLocalPeerFetchFailure(err) {
			s.strikeLocalPeer(p.Id(), "fetch", err)
		}
		return nil, err
	}
	s.resetLocalPeer(p.Id())
	return data, nil
}

// connectLocalPeer returns a peer and an acquired sub-conn for one of ids.
// Already-connected peers are preferred through a non-blocking probe; then
// every candidate gets one localPeerConnectTimeout budget for pool lookup/dial
// plus sub-conn acquisition. Candidates whose failure is evidence against them
// are struck; the caller's own ctx ending, the pool closing, another caller's
// aborted shared dial and a peer closed locally are not.
func (s *store) connectLocalPeer(ctx context.Context, ids []string) (peer.Peer, drpc.Conn, error) {
	candidates := make([]string, len(ids))
	copy(candidates, ids)
	s.shuffleLocalPeers(candidates)

	// Non-blocking scan: pool.Pick serves a live peer from its fast path
	// without looking at ctx, and for an entry still being dialed by someone
	// else ocache's waitLoad returns at once under a done ctx instead of
	// waiting out that dial. A found peer still goes through the bounded
	// acquire below.
	probeCtx, cancelProbe := context.WithCancel(ctx)
	cancelProbe()
	for i, id := range candidates {
		if p, err := s.pool.Pick(probeCtx, id); err == nil && p != nil && !p.IsClosed() {
			candidates[0], candidates[i] = candidates[i], candidates[0]
			break
		}
	}

	var lastErr error
	for _, id := range candidates {
		p, conn, err := s.connectOneLocalPeer(ctx, id)
		if err == nil {
			return p, conn, nil
		}
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("connect local peer: %w", ctx.Err())
		}
		if errors.Is(err, ocache.ErrClosed) {
			return nil, nil, fmt.Errorf("connect local peer: %w", err)
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf("connect local peer: %w", errors.Join(net.ErrUnableToConnect, lastErr))
}

// connectOneLocalPeer gets a peer and a sub-conn under one connect budget and
// strikes the peer when the failure is evidence against it.
func (s *store) connectOneLocalPeer(ctx context.Context, id string) (peer.Peer, drpc.Conn, error) {
	connectCtx, cancel := context.WithTimeout(ctx, localPeerConnectTimeout)
	defer cancel()
	p, err := s.pool.Get(connectCtx, id)
	if err != nil {
		if s.isLocalPeerConnectFailure(ctx, connectCtx, err, false) {
			s.strikeLocalPeer(id, "dial", err)
		}
		return nil, nil, fmt.Errorf("dial local peer: %w", err)
	}
	conn, err := p.AcquireDrpcConn(connectCtx)
	if err != nil {
		if s.isLocalPeerConnectFailure(ctx, connectCtx, err, true) {
			s.strikeLocalPeer(id, "acquire", err)
		}
		return nil, nil, fmt.Errorf("acquire local peer conn: %w", err)
	}
	return p, conn, nil
}

// isLocalPeerConnectFailure says whether a pool.Get (acquire false) or
// AcquireDrpcConn (acquire true) error is evidence against the peer.
//
// Not evidence: the caller's own ctx ending; the pool closing
// (ocache.ErrClosed); a ctx error while our connect ctx is still alive, which
// is another caller's shared dial being aborted (ocache.Get retries those
// only maxLoadRetries times and then returns the owner's error, Canceled or
// DeadlineExceeded); and, from Acquire only, a peer object closed locally
// (transport.ErrConnClosed: pool Flush or gc; the next Get redials). From
// pool.Get, ErrConnClosed is a dial whose conn died during setup: evidence.
//
// Evidence: our connect budget expiring (slow dial, limiter wait, slow open)
// and every real dial/handshake/open error.
//
// Trade-off: an any-sync internal timeout shorter than localPeerConnectTimeout
// that surfaces as DeadlineExceeded with our ctx alive no longer strikes. The
// heart's DialTimeoutSec is 10 s, above the 5 s budget, so a real slow dial
// outlasts connectCtx and is struck by the branch above; if an internal
// timeout were shorter, every call would still be bounded by it.
func (s *store) isLocalPeerConnectFailure(ctx, connectCtx context.Context, err error, acquire bool) bool {
	if ctx.Err() != nil || errors.Is(err, ocache.ErrClosed) {
		return false
	}
	if connectCtx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if acquire && errors.Is(err, transport.ErrConnClosed) {
		return false
	}
	return true
}

// isLocalPeerFetchFailure says whether a BlockGet error, with the caller's
// ctx still alive, is evidence against the peer: the watchdog cancelled it,
// the sub-conn ended under the RPC (net.ErrClosed, which transport's
// ErrConnClosed unwraps to, or drpc's context stand-ins for a transport EOF),
// or the peer closed the stream without a response (io.EOF). Application
// errors mean the peer answered.
func isLocalPeerFetchFailure(err error) bool {
	return errors.Is(err, errLocalPeerStalled) ||
		errors.Is(err, gonet.ErrClosed) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF)
}

// fetchBlockFromLocalPeer runs BlockGet on an acquired sub-conn under the
// caller's ctx, with a watchdog that cancels the RPC when the conn stops
// receiving bytes, and releases the conn. The ctx handed to ReleaseDrpcConn
// is the one the RPC ran under: when it is done the sub-conn's transport has
// been terminated by drpc and the peer must not re-pool it.
func (s *store) fetchBlockFromLocalPeer(ctx context.Context, p peer.Peer, conn drpc.Conn, spaceId string, k cid.Cid) ([]byte, error) {
	fetchCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	defer p.ReleaseDrpcConn(fetchCtx, conn)
	stop := watchFetchProgress(fetchCtx, conn, cancel)
	defer stop()
	resp, err := fileproto.NewDRPCFileClient(conn).BlockGet(fetchCtx, &fileproto.BlockGetRequest{
		SpaceId: spaceId,
		Cid:     k.Bytes(),
	})
	// join the watchdog before reading the cause and before the release
	stop()
	if err != nil {
		if cause := context.Cause(fetchCtx); errors.Is(cause, errLocalPeerStalled) {
			return nil, fmt.Errorf("local peer block get: %w", errLocalPeerStalled)
		}
		err = rpcerr.Unwrap(err)
		if errors.Is(err, fileprotoerr.ErrCIDNotFound) {
			return nil, fmt.Errorf("local peer block get: %w", format.ErrNotFound{Cid: k})
		}
		return nil, fmt.Errorf("local peer block get: %w", err)
	}
	return resp.Data, nil
}

// watchFetchProgress cancels ctx with errLocalPeerStalled when conn stops
// receiving bytes for localPeerStallTimeout (checked every
// localPeerStallCheckInterval), or after localPeerFetchFallbackTimeout when
// conn has no byte counter. Once stop has begun no stall cancel can happen: a
// decision taken concurrently is dropped. stop joins the goroutine and is safe
// to call more than once.
func watchFetchProgress(ctx context.Context, conn drpc.Conn, cancel context.CancelCauseFunc) (stop func()) {
	done := make(chan struct{})
	exited := make(chan struct{})
	var (
		mu      sync.Mutex
		stopped bool
	)
	stall := func() {
		mu.Lock()
		defer mu.Unlock()
		if !stopped {
			cancel(errLocalPeerStalled)
		}
	}
	counter, hasCounter := conn.(bytesReader)
	go func() {
		defer close(exited)
		if !hasCounter {
			timer := time.NewTimer(localPeerFetchFallbackTimeout)
			defer timer.Stop()
			select {
			case <-done:
			case <-ctx.Done():
			case <-timer.C:
				stall()
			}
			return
		}
		ticker := time.NewTicker(localPeerStallCheckInterval)
		defer ticker.Stop()
		last := counter.BytesRead()
		lastProgress := time.Now()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				// not the tick's value: a ticker delivers the time the tick was
				// scheduled, which after a suspension or starvation is in the
				// past and would backdate the progress stamp
				cur := counter.BytesRead()
				now := time.Now()
				if cur != last {
					last, lastProgress = cur, now
					continue
				}
				if now.Sub(lastProgress) >= localPeerStallTimeout {
					stall()
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		mu.Lock()
		stopped = true
		mu.Unlock()
		once.Do(func() { close(done) })
		<-exited
	}
}

func (s *store) getFromNodePeer(ctx context.Context, spaceId string, k cid.Cid) ([]byte, error) {
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("get node peer: %w", err)
	}
	return s.getBlock(ctx, p, spaceId, k, IsWaitWhenAvailable(ctx))
}

func (s *store) getBlock(ctx context.Context, p peer.Peer, spaceId string, k cid.Cid, wait bool) ([]byte, error) {
	var data []byte
	err := p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err := fileproto.NewDRPCFileClient(conn).BlockGet(ctx, &fileproto.BlockGetRequest{
			SpaceId: spaceId,
			Cid:     k.Bytes(),
			Wait:    wait,
		})
		if err != nil {
			err = rpcerr.Unwrap(err)
			if errors.Is(err, fileprotoerr.ErrCIDNotFound) {
				return format.ErrNotFound{Cid: k}
			}
			return err
		}
		data = resp.Data
		return nil
	})
	return data, err
}

// GetMany retrieves multiple blocks concurrently with a bounded worker pool,
// trying local peers first then falling back to node.
func (s *store) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	resultCh := make(chan blocks.Block, len(ks))
	go func() {
		spaceId := fileblockstore.CtxGetSpaceId(ctx)
		var wg sync.WaitGroup
		// close only once every launched worker has returned: a worker that
		// finishes after a cancel must not find the channel closed
		defer func() {
			wg.Wait()
			close(resultCh)
		}()
		sem := make(chan struct{}, getManyWorkers)
		for _, k := range ks {
			wg.Add(1)
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Done()
				return
			}
			go func(k cid.Cid) {
				defer func() {
					<-sem
					wg.Done()
				}()
				data, err := s.getLocalThenNode(ctx, spaceId, k)
				if err != nil {
					log.Info("get many: block error", zap.String("cid", k.String()), zap.Error(err))
					return
				}
				b, err := blocks.NewBlockWithCid(data, k)
				if err != nil {
					log.Info("get many: new block error", zap.String("cid", k.String()), zap.Error(err))
					return
				}
				select {
				case <-ctx.Done():
				case resultCh <- b:
				}
			}(k)
		}
	}()
	return resultCh
}

func (s *store) Add(ctx context.Context, bs []blocks.Block) error {
	return ErrUnsupported
}

func (s *store) Delete(ctx context.Context, c cid.Cid) error {
	return ErrUnsupported
}

// AddToFile pushes blocks to the file node one at a time.
func (s *store) AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) error {
	if len(bs) == 0 {
		return nil
	}
	for _, b := range bs {
		err := s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
			_, err := cl.BlockPush(ctx, &fileproto.BlockPushRequest{
				SpaceId: spaceId,
				FileId:  fileId.String(),
				Cid:     b.Cid().Bytes(),
				Data:    b.RawData(),
			})
			if err != nil {
				return rpcerr.Unwrap(err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// AddToFileMany pushes multiple file blocks in a single RPC call.
func (s *store) AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) error {
	if len(req.FileBlocks) == 0 {
		return nil
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.BlockPushMany(ctx, req)
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// CheckAvailability checks which CIDs exist on the file node.
func (s *store) CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
	cidsB := make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	var result []*fileproto.BlockAvailability
	err := s.doNodeReserved(ctx, func(ctx context.Context, cl fileproto.DRPCFileClient) error {
		resp, err := cl.BlocksCheck(ctx, &fileproto.BlocksCheckRequest{
			SpaceId: spaceID,
			Cids:    cidsB,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp.BlocksAvailability
		return nil
	})
	return result, err
}

// BindCids binds CIDs to a file on the file node.
func (s *store) BindCids(ctx context.Context, spaceID string, fileId domain.FileId, cids []cid.Cid) error {
	cidsB := make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.BlocksBind(ctx, &fileproto.BlocksBindRequest{
			SpaceId: spaceID,
			FileId:  fileId.String(),
			Cids:    cidsB,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// DeleteFiles deletes files from the file node.
func (s *store) DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) error {
	rawFileIds := make([]string, 0, len(fileIds))
	for _, id := range fileIds {
		rawFileIds = append(rawFileIds, id.String())
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.FilesDelete(ctx, &fileproto.FilesDeleteRequest{
			SpaceId: spaceId,
			FileIds: rawFileIds,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// AccountInfo retrieves account-level file storage info.
func (s *store) AccountInfo(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
	var result *fileproto.AccountInfoResponse
	err := s.doNodeReserved(ctx, func(ctx context.Context, cl fileproto.DRPCFileClient) error {
		resp, err := cl.AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp
		return nil
	})
	return result, err
}

// SpaceInfo retrieves space-level file storage info.
func (s *store) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	var result *fileproto.SpaceInfoResponse
	err := s.doNodeReserved(ctx, func(ctx context.Context, cl fileproto.DRPCFileClient) error {
		resp, err := cl.SpaceInfo(ctx, &fileproto.SpaceInfoRequest{
			SpaceId: spaceId,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp
		return nil
	})
	return result, err
}

// FilesInfo retrieves info about specific files.
func (s *store) FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error) {
	rawFileIds := make([]string, 0, len(fileIds))
	for _, id := range fileIds {
		rawFileIds = append(rawFileIds, id.String())
	}
	var result []*fileproto.FileInfo
	err := s.doNodeReserved(ctx, func(ctx context.Context, cl fileproto.DRPCFileClient) error {
		resp, err := cl.FilesInfo(ctx, &fileproto.FilesInfoRequest{
			SpaceId: spaceId,
			FileIds: rawFileIds,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp.FilesInfo
		return nil
	})
	return result, err
}

// IterateFiles iterates over all files across all spaces via streaming RPC.
func (s *store) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		for _, space := range resp.Spaces {
			if err := iterateSpaceFiles(ctx, cl, space.SpaceId, iterFunc); err != nil {
				return fmt.Errorf("iterate space files: %w", err)
			}
		}
		return nil
	})
}

func iterateSpaceFiles(ctx context.Context, client fileproto.DRPCFileClient, spaceId string, iterFunc func(fileId domain.FullFileId)) error {
	filesStream, err := client.FilesGet(ctx, &fileproto.FilesGetRequest{SpaceId: spaceId})
	if err != nil {
		return rpcerr.Unwrap(err)
	}
	defer filesStream.Close()
	for {
		resp, err := filesStream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		iterFunc(domain.FullFileId{
			SpaceId: spaceId,
			FileId:  domain.FileId(resp.FileId),
		})
	}
}

// Close is a no-op; the reserved connection is closed with the pool component.
func (s *store) Close() error {
	return nil
}
