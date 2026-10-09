package rpcstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
	"github.com/anyproto/any-sync/net/transport"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

// Unit tests for the local peer path run under testing/synctest with the
// production constants: every fake blocks only on channels, timers and ctx, so
// fake time advances deterministically and the assertions on when a fetch was
// cancelled are exact.

const (
	testSpaceId = "space1"
	testLocalA  = "localA"
	testLocalB  = "localB"
	testNodeId  = "node1"
)

// ---- fakes ----

type fakePeerStore struct {
	local []string
	nodes []string
}

func (f *fakePeerStore) Init(*app.App) error                { return nil }
func (f *fakePeerStore) Name() string                       { return peerstore.CName }
func (f *fakePeerStore) ResponsibleNodeIds(string) []string { return nil }
func (f *fakePeerStore) ResponsibleFilePeers() []string     { return slices.Clone(f.nodes) }
func (f *fakePeerStore) LocalPeerIds(string) []string       { return slices.Clone(f.local) }
func (f *fakePeerStore) AllLocalPeers() []string            { return slices.Clone(f.local) }
func (f *fakePeerStore) UpdateLocalPeer(string, []string)   {}
func (f *fakePeerStore) RemoveLocalPeer(string)             {}
func (f *fakePeerStore) AddObserver(peerstore.Observer)     {}

type getCall struct {
	id       string
	deadline time.Time
	bounded  bool
}

type pickCall struct {
	id      string
	ctxDone bool
}

type fakePool struct {
	mu    sync.Mutex
	picks map[string]func(ctx context.Context) (peer.Peer, error)
	gets  map[string]func(ctx context.Context) (peer.Peer, error)
	node  peer.Peer

	getCalls      []getCall
	pickCalls     []pickCall
	getOneOfCalls int
	seen          []*fakePeer
}

func newFakePool(node peer.Peer) *fakePool {
	return &fakePool{
		picks: map[string]func(ctx context.Context) (peer.Peer, error){},
		gets:  map[string]func(ctx context.Context) (peer.Peer, error){},
		node:  node,
	}
}

func (p *fakePool) Get(ctx context.Context, id string) (peer.Peer, error) {
	p.mu.Lock()
	d, ok := ctx.Deadline()
	p.getCalls = append(p.getCalls, getCall{id: id, deadline: d, bounded: ok})
	fn := p.gets[id]
	p.mu.Unlock()
	if fn == nil {
		return nil, net.ErrUnableToConnect
	}
	pr, err := fn(ctx)
	p.track(pr)
	return pr, err
}

// track remembers every fake peer handed out, so the fixture can check their
// lease invariants at the end of the test
func (p *fakePool) track(pr peer.Peer) {
	fp, ok := pr.(*fakePeer)
	if !ok || fp == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !slices.Contains(p.seen, fp) {
		p.seen = append(p.seen, fp)
	}
}

func (p *fakePool) seenPeers() []*fakePeer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.seen)
}

func (p *fakePool) Pick(ctx context.Context, id string) (peer.Peer, error) {
	p.mu.Lock()
	p.pickCalls = append(p.pickCalls, pickCall{id: id, ctxDone: ctx.Err() != nil})
	fn := p.picks[id]
	p.mu.Unlock()
	if fn == nil {
		return nil, ocache.ErrNotExists
	}
	pr, err := fn(ctx)
	p.track(pr)
	return pr, err
}

func (p *fakePool) GetOneOf(ctx context.Context, peerIds []string) (peer.Peer, error) {
	p.mu.Lock()
	p.getOneOfCalls++
	node := p.node
	p.mu.Unlock()
	if node == nil {
		return nil, net.ErrUnableToConnect
	}
	p.track(node)
	return node, nil
}

func (p *fakePool) AddPeer(context.Context, peer.Peer) error { return nil }
func (p *fakePool) Flush(context.Context) error              { return nil }

func (p *fakePool) getCallsFor(id string) []getCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var res []getCall
	for _, c := range p.getCalls {
		if c.id == id {
			res = append(res, c)
		}
	}
	return res
}

func (p *fakePool) pickCallsFor(id string) []pickCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var res []pickCall
	for _, c := range p.pickCalls {
		if c.id == id {
			res = append(res, c)
		}
	}
	return res
}

func (p *fakePool) nodeCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getOneOfCalls
}

type releaseRec struct {
	conn   drpc.Conn
	err    error
	cause  error
	pooled bool
}

// fakePeer models any-sync peer's sub-conn leasing (net/peer/peer.go):
// AcquireDrpcConn hands out a pooled conn before asking acquire for a new
// one, discards closed idle conns instead of handing them out, and never
// leases one conn twice at a time; ReleaseDrpcConn re-pools a conn only when
// the release ctx is live, the conn is not closed and it has no unfinished
// call (checkReleased's Unblocked wait); otherwise it closes it (closeAsync)
// and drops it. Close, like a real peer's, closes every sub-conn (in-flight
// calls on them fail) and refuses later leases; the store never closes a
// peer, so the fixture fails any test in which Close was called. Breaches of
// these invariants by the code under test are recorded as violations and fail
// the test in the fixture's cleanup.
type fakePeer struct {
	id      string
	acquire func(ctx context.Context) (drpc.Conn, error)
	closed  atomic.Bool

	mu         sync.Mutex
	acquires   int
	closeCalls int
	closeCh    chan struct{}
	releases   []releaseRec
	idle       []drpc.Conn
	leased     map[drpc.Conn]bool
	violations []string
}

func newFakePeer(id string, acquire func(ctx context.Context) (drpc.Conn, error)) *fakePeer {
	return &fakePeer{id: id, acquire: acquire, leased: map[drpc.Conn]bool{}, closeCh: make(chan struct{})}
}

// peerWithConn is a peer whose only conn is conn: a second lease while it is
// out, or a lease after it was closed, fails like a real peer whose conn
// could not be opened
func peerWithConn(id string, conn drpc.Conn) *fakePeer {
	return newFakePeer(id, func(context.Context) (drpc.Conn, error) { return conn, nil })
}

func isConnClosed(conn drpc.Conn) bool {
	select {
	case <-conn.Closed():
		return true
	default:
		return false
	}
}

func (p *fakePeer) Id() string               { return p.id }
func (p *fakePeer) Context() context.Context { return context.Background() }
func (p *fakePeer) IsClosed() bool           { return p.closed.Load() }
func (p *fakePeer) CloseChan() <-chan struct{} {
	return p.closeCh
}
func (p *fakePeer) SetTTL(time.Duration)                 {}
func (p *fakePeer) TryClose(time.Duration) (bool, error) { return false, nil }

// Close closes the peer like any-sync's: every leased and idle sub-conn is
// closed, which fails the calls running on them
func (p *fakePeer) Close() error {
	p.mu.Lock()
	p.closeCalls++
	conns := slices.Clone(p.idle)
	for conn := range p.leased {
		conns = append(conns, conn)
	}
	p.idle = nil
	first := !p.closed.Swap(true)
	p.mu.Unlock()
	if first {
		close(p.closeCh)
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	return nil
}

func (p *fakePeer) closeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeCalls
}

func (p *fakePeer) AcquireDrpcConn(ctx context.Context) (drpc.Conn, error) {
	p.mu.Lock()
	p.acquires++
	if p.closeCalls > 0 {
		p.mu.Unlock()
		return nil, transport.ErrConnClosed
	}
	for len(p.idle) > 0 {
		n := len(p.idle)
		conn := p.idle[n-1]
		p.idle = p.idle[:n-1]
		if isConnClosed(conn) {
			continue // peer.acquireDrpcConn drops a closed inactive conn
		}
		p.leased[conn] = true
		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()
	conn, err := p.acquire(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if isConnClosed(conn) {
		return nil, errors.New("fake peer: open sub-conn: closed")
	}
	if p.leased[conn] {
		return nil, errors.New("fake peer: open sub-conn: conn already leased")
	}
	p.leased[conn] = true
	return conn, nil
}

func (p *fakePeer) ReleaseDrpcConn(ctx context.Context, conn drpc.Conn) {
	closed := isConnClosed(conn)
	busy := false
	if ic, ok := conn.(interface{ inflightCalls() int32 }); ok {
		busy = ic.inflightCalls() > 0
	}
	pooled := ctx.Err() == nil && !closed && !busy
	if !pooled && !closed {
		_ = conn.Close()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.leased[conn] {
		p.violations = append(p.violations, "release of a conn that is not leased")
	}
	delete(p.leased, conn)
	p.releases = append(p.releases, releaseRec{conn: conn, err: ctx.Err(), cause: context.Cause(ctx), pooled: pooled})
	if pooled {
		p.idle = append(p.idle, conn)
	}
}

func (p *fakePeer) DoDrpc(ctx context.Context, do func(conn drpc.Conn) error) error {
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return err
	}
	defer p.ReleaseDrpcConn(ctx, conn)
	return do(conn)
}

func (p *fakePeer) violationsList() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.violations)
}

func (p *fakePeer) stats() (acquires int, releases []releaseRec) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acquires, slices.Clone(p.releases)
}

// blockScript decides what an Invoke of BlockGet does. It must block only on
// ctx, channels or timers.
type blockScript func(ctx context.Context) ([]byte, error)

// plainConn is a drpc.Conn without BytesRead. Invoke checks the request
// like the serving side would (rpchandler.BlockGet casts the CID) and fails on
// a closed conn like drpc does, also when the conn is closed while the call
// runs.
type plainConn struct {
	script blockScript
	// allowWait accepts Wait: true (the node path); a local fetch must not
	// ask the peer to wait for a block it does not have
	allowWait bool

	invokes  atomic.Int32
	inflight atomic.Int32
	closes   atomic.Int32

	closeOnce sync.Once
	closed    chan struct{}

	mu          sync.Mutex
	cancelledAt []time.Time // fake-time stamps of invokes that ended with a done ctx
	requests    []*fileproto.BlockGetRequest
}

func newPlainConn(script blockScript) *plainConn {
	return &plainConn{script: script, closed: make(chan struct{})}
}

func (c *plainConn) inflightCalls() int32 { return c.inflight.Load() }

func (c *plainConn) checkRequest(req *fileproto.BlockGetRequest) error {
	if req.SpaceId != testSpaceId {
		return fmt.Errorf("fake conn: unexpected space id %q", req.SpaceId)
	}
	if _, err := cid.Cast(req.Cid); err != nil {
		return fmt.Errorf("fake conn: cast cid: %w", err)
	}
	if req.Wait && !c.allowWait {
		return errors.New("fake conn: local fetch must not wait")
	}
	return nil
}

func (c *plainConn) Invoke(ctx context.Context, _ string, _ drpc.Encoding, in, out drpc.Message) error {
	if isConnClosed(c) {
		return drpc.ClosedError.New("connection closed")
	}
	c.invokes.Add(1)
	c.inflight.Add(1)
	defer c.inflight.Add(-1)
	req := in.(*fileproto.BlockGetRequest)
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.mu.Unlock()
	if err := c.checkRequest(req); err != nil {
		return err
	}
	// the script sees the conn closing as its ctx ending, like a drpc call
	// whose transport is torn down
	scriptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.closed:
			cancel()
		case <-scriptCtx.Done():
		}
	}()
	data, err := c.script(scriptCtx)
	if isConnClosed(c) && ctx.Err() == nil {
		return drpc.ClosedError.New("connection closed")
	}
	if ctx.Err() != nil {
		c.mu.Lock()
		c.cancelledAt = append(c.cancelledAt, time.Now())
		c.mu.Unlock()
	}
	if err != nil {
		return err
	}
	*out.(*fileproto.BlockGetResponse) = fileproto.BlockGetResponse{Cid: req.Cid, Data: data}
	return nil
}

func (c *plainConn) requestsSeen() []*fileproto.BlockGetRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.requests)
}

func (c *plainConn) NewStream(context.Context, string, drpc.Encoding) (drpc.Stream, error) {
	return nil, errors.New("unexpected NewStream")
}
func (c *plainConn) Close() error {
	c.closes.Add(1)
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}
func (c *plainConn) Closed() <-chan struct{} { return c.closed }
func (c *plainConn) cancellations() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.cancelledAt)
}

// progressConn is a drpc.Conn with a BytesRead counter the script can advance
type progressConn struct {
	*plainConn
	bytes atomic.Int64
	polls atomic.Int32
}

func newProgressConn(baseline int64) *progressConn {
	c := &progressConn{plainConn: newPlainConn(nil)}
	c.bytes.Store(baseline)
	return c
}

func (c *progressConn) BytesRead() int64 {
	c.polls.Add(1)
	return c.bytes.Load()
}

func answer(data []byte) blockScript {
	return func(context.Context) ([]byte, error) { return data, nil }
}

func fail(err error) blockScript {
	return func(context.Context) ([]byte, error) { return nil, err }
}

func silent() blockScript {
	return func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

// progressing advances c.bytes by step every interval for total, then answers.
// A zero total means "advance forever until ctx ends".
func progressing(c *progressConn, total, interval time.Duration, step int64, data []byte) blockScript {
	return func(ctx context.Context) ([]byte, error) {
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
				c.bytes.Add(step)
			}
			if total > 0 && time.Since(start) >= total {
				return data, nil
			}
		}
	}
}

// progressThenSilent advances for active, then blocks until ctx ends
func progressThenSilent(c *progressConn, active, interval time.Duration, step int64) blockScript {
	return func(ctx context.Context) ([]byte, error) {
		start := time.Now()
		for time.Since(start) < active {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
				c.bytes.Add(step)
			}
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

// ---- fixture ----

type localFixture struct {
	*store
	pool      *fakePool
	peerStore *fakePeerStore
	node      *fakePeer
	nodeData  []byte
	// baseCtx is cancelled in t.Cleanup so an operation a failed test left
	// behind unwinds instead of tripping the bubble's deadlock detector
	baseCtx context.Context

	nodeMu     sync.Mutex
	nodeScript blockScript
	nodeConns  []*plainConn
}

func newLocalFixture(t *testing.T, localIds ...string) *localFixture {
	t.Helper()
	nodeData := []byte("from-node")
	fx := &localFixture{nodeData: nodeData, nodeScript: answer(nodeData)}
	// one conn per lease, like a real node peer under concurrent GetMany
	// workers
	fx.node = newFakePeer(testNodeId, func(context.Context) (drpc.Conn, error) {
		fx.nodeMu.Lock()
		defer fx.nodeMu.Unlock()
		c := newPlainConn(fx.nodeScript)
		c.allowWait = true
		fx.nodeConns = append(fx.nodeConns, c)
		return c, nil
	})
	fx.pool = newFakePool(fx.node)
	fx.peerStore = &fakePeerStore{local: localIds, nodes: []string{testNodeId}}
	fx.store = newStore(fx.pool, fx.peerStore)
	fx.shuffleLocalPeers = func([]string) {} // deterministic candidate order
	baseCtx, cancel := context.WithCancel(context.Background())
	fx.baseCtx = baseCtx
	t.Cleanup(cancel)
	t.Cleanup(func() {
		for _, p := range fx.pool.seenPeers() {
			assert.Empty(t, p.violationsList(), "lease violations on peer %s", p.id)
			assert.Zero(t, p.closeCount(), "the store must never close peer %s", p.id)
		}
	})
	return fx
}

// setNodeScript changes what the node's conns opened from now on do
func (fx *localFixture) setNodeScript(script blockScript) {
	fx.nodeMu.Lock()
	defer fx.nodeMu.Unlock()
	fx.nodeScript = script
}

func (fx *localFixture) ctx() context.Context {
	return fileblockstore.CtxWithSpaceId(fx.baseCtx, testSpaceId)
}

func (fx *localFixture) banOf(id string) (localPeerBan, bool) {
	fx.bannedMu.Lock()
	defer fx.bannedMu.Unlock()
	b, ok := fx.bannedLocalMap[id]
	return b, ok
}

func (fx *localFixture) requireStruck(t *testing.T, id string, strikes int) {
	t.Helper()
	b, ok := fx.banOf(id)
	require.True(t, ok, "peer %s must have a ban entry", id)
	assert.Equal(t, strikes, b.strikes, "strikes of %s", id)
	assert.True(t, b.until.After(time.Now()), "ban of %s must be active", id)
}

func (fx *localFixture) requireNotStruck(t *testing.T, id string) {
	t.Helper()
	b, ok := fx.banOf(id)
	assert.False(t, ok, "peer %s must not be struck, got %+v", id, b)
}

type fetchResult struct {
	data []byte
	err  error
}

// runBounded runs fn in a goroutine and waits for it under an independent
// fake-time limit, so a hang fails with a message instead of relying on the
// synctest deadlock detector.
func runBounded(t *testing.T, limit time.Duration, fn func() ([]byte, error)) fetchResult {
	t.Helper()
	res := make(chan fetchResult, 1)
	go func() {
		data, err := fn()
		res <- fetchResult{data: data, err: err}
	}()
	select {
	case r := <-res:
		return r
	case <-time.After(limit):
		t.Fatalf("operation did not finish within %v of fake time", limit)
		return fetchResult{}
	}
}

func testCid(s string) cid.Cid {
	return blocks.NewBlock([]byte(s)).Cid()
}

// ---- fetch-phase tests ----

func TestLocalPeer_SlowButProgressingFetchCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		conn := newProgressConn(1234)
		want := []byte("from-local")
		// 40 s is beyond the 30 s fallback cap: an implementation that ignores
		// the counter and applies the cap would cancel this fetch
		conn.script = progressing(conn, 40*time.Second, 500*time.Millisecond, 64<<10, want)
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }

		got := runBounded(t, 45*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		assert.Empty(t, conn.cancellations(), "fetch must not be cancelled")
		fx.requireNotStruck(t, testLocalA)
		assert.Equal(t, 0, fx.pool.nodeCalls())
		assert.Greater(t, conn.polls.Load(), int32(1), "watchdog must poll the counter")
		reqs := conn.requestsSeen()
		require.Len(t, reqs, 1)
		assert.Equal(t, &fileproto.BlockGetRequest{SpaceId: testSpaceId, Cid: testCid("a").Bytes()}, reqs[0])
		_, releases := local.stats()
		require.Len(t, releases, 1)
		assert.NoError(t, releases[0].err, "Release must see a live ctx so the conn is re-pooled")
		assert.Same(t, conn, releases[0].conn)
	})
}

func TestLocalPeer_ProgressThenSilenceIsCancelledAfterStallWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		conn := newProgressConn(0)
		conn.script = progressThenSilent(conn, 3*time.Second, 500*time.Millisecond, 4096)
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }
		start := time.Now()

		got := runBounded(t, 15*time.Second, func() ([]byte, error) {
			return fx.getLocalThenNode(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err, "node must serve the block")
		assert.Equal(t, fx.nodeData, got.data)
		cancels := conn.cancellations()
		require.Len(t, cancels, 1)
		since := cancels[0].Sub(start)
		// last increment at 3 s; stall window 5 s; tick 1 s
		assert.GreaterOrEqual(t, since, 8*time.Second, "cancelled too early: %v", since)
		assert.LessOrEqual(t, since, 9*time.Second, "cancelled too late: %v", since)
		fx.requireStruck(t, testLocalA, 1)
		_, releases := local.stats()
		require.Len(t, releases, 1)
		assert.ErrorIs(t, releases[0].cause, errLocalPeerStalled)
	})
}

func TestLocalPeer_SilentFetchIsCancelledBannedAndFallsBackToNode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		conn := newProgressConn(99)
		conn.script = silent()
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }
		start := time.Now()

		got := runBounded(t, 7*time.Second, func() ([]byte, error) {
			return fx.getLocalThenNode(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, fx.nodeData, got.data)
		cancels := conn.cancellations()
		require.Len(t, cancels, 1)
		since := cancels[0].Sub(start)
		assert.GreaterOrEqual(t, since, localPeerStallTimeout, "cancelled before the stall window: %v", since)
		assert.LessOrEqual(t, since, localPeerStallTimeout+localPeerStallCheckInterval, "cancelled after the stall window: %v", since)
		fx.requireStruck(t, testLocalA, 1)
		assert.Equal(t, []string{}, fx.filterBannedPeers([]string{testLocalA}))
		_, releases := local.stats()
		require.Len(t, releases, 1)
		assert.ErrorIs(t, releases[0].err, context.Canceled)
		assert.ErrorIs(t, releases[0].cause, errLocalPeerStalled)
		assert.Equal(t, 1, fx.pool.nodeCalls())

		// banned: the next Get goes straight to the node without touching the local pool
		before := len(fx.pool.getCallsFor(testLocalA))
		got = runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getLocalThenNode(fx.ctx(), testSpaceId, testCid("b"))
		})
		require.NoError(t, got.err)
		assert.Equal(t, before, len(fx.pool.getCallsFor(testLocalA)))
		assert.Equal(t, 2, fx.pool.nodeCalls())
	})
}

func TestLocalPeer_ConnWithoutBytesReadUsesFallbackCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		conn := newPlainConn(silent())
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }
		start := time.Now()

		got := runBounded(t, 32*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, errLocalPeerStalled)
		cancels := conn.cancellations()
		require.Len(t, cancels, 1)
		since := cancels[0].Sub(start)
		assert.GreaterOrEqual(t, since, localPeerFetchFallbackTimeout, "cancelled before the fallback cap: %v", since)
		assert.LessOrEqual(t, since, localPeerFetchFallbackTimeout+localPeerStallCheckInterval, "cancelled after the fallback cap: %v", since)
		fx.requireStruck(t, testLocalA, 1)
	})
}

func TestLocalPeer_CallerEndingTheFetchDoesNotBan(t *testing.T) {
	cases := []struct {
		name    string
		makeCtx func(ctx context.Context) (context.Context, context.CancelFunc)
		want    error
	}{
		{
			name: "caller cancel",
			makeCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				time.AfterFunc(time.Second, cancel)
				return ctx, cancel
			},
			want: context.Canceled,
		},
		{
			name: "caller deadline",
			makeCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithTimeout(ctx, time.Second)
			},
			want: context.DeadlineExceeded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fx := newLocalFixture(t, testLocalA)
				conn := newProgressConn(0)
				conn.script = silent()
				local := peerWithConn(testLocalA, conn)
				fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }
				ctx, cancel := tc.makeCtx(fx.ctx())
				defer cancel()

				got := runBounded(t, 3*time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(ctx, testSpaceId, testCid("a"))
				})

				require.ErrorIs(t, got.err, tc.want)
				assert.NotErrorIs(t, got.err, errLocalPeerStalled)
				fx.requireNotStruck(t, testLocalA)
				_, releases := local.stats()
				require.Len(t, releases, 1)
				assert.ErrorIs(t, releases[0].err, tc.want)
			})
		})
	}
}

func TestLocalPeer_FetchErrorClassification(t *testing.T) {
	codedCIDNotFound := drpcerr.WithCode(errors.New("remote: CID not found"), rpcerr.Code(fileprotoerr.ErrCIDNotFound))
	codedForbidden := drpcerr.WithCode(errors.New("remote: forbidden"), rpcerr.Code(fileprotoerr.ErrForbidden))
	cases := []struct {
		name   string
		err    error
		want   error
		strike bool
	}{
		{name: "io.EOF strikes", err: io.EOF, want: io.EOF, strike: true},
		{name: "conn closed strikes", err: transport.ErrConnClosed, want: transport.ErrConnClosed, strike: true},
		// drpc's stand-ins for the sub-conn ending under the RPC while the
		// caller's ctx is alive
		{name: "Canceled with live caller ctx strikes", err: context.Canceled, want: context.Canceled, strike: true},
		{name: "DeadlineExceeded with live caller ctx strikes", err: context.DeadlineExceeded, want: context.DeadlineExceeded, strike: true},
		{name: "coded CID not found maps to ErrNotFound, no strike", err: codedCIDNotFound, want: format.ErrNotFound{}, strike: false},
		{name: "coded application error, no strike", err: codedForbidden, want: fileprotoerr.ErrForbidden, strike: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fx := newLocalFixture(t, testLocalA)
				conn := newProgressConn(0)
				conn.script = fail(tc.err)
				local := peerWithConn(testLocalA, conn)
				fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }

				got := runBounded(t, time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
				})

				require.Error(t, got.err)
				assert.ErrorIs(t, got.err, tc.want)
				if tc.strike {
					fx.requireStruck(t, testLocalA, 1)
				} else {
					fx.requireNotStruck(t, testLocalA)
				}
			})
		})
	}
}

func TestLocalPeer_SuccessResetsStrikes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		fx.strikeLocalPeer(testLocalA, "test", errors.New("x"))
		b, _ := fx.banOf(testLocalA)
		time.Sleep(b.until.Sub(time.Now()) + time.Millisecond) // let the ban expire
		conn := newProgressConn(0)
		conn.script = answer([]byte("ok"))
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return local, nil }

		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		fx.requireNotStruck(t, testLocalA)
	})
}

// ---- connect-phase tests ----

func blockingGet(p peer.Peer) func(ctx context.Context) (peer.Peer, error) {
	return func(ctx context.Context) (peer.Peer, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func immediateGet(p peer.Peer) func(ctx context.Context) (peer.Peer, error) {
	return func(context.Context) (peer.Peer, error) { return p, nil }
}

func TestLocalPeer_ConnectDialTimeoutStrikesOnlyTheSlowPeer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA, testLocalB)
		want := []byte("from-B")
		connB := newProgressConn(0)
		connB.script = answer(want)
		peerB := peerWithConn(testLocalB, connB)
		fx.pool.gets[testLocalA] = blockingGet(nil)
		fx.pool.gets[testLocalB] = immediateGet(peerB)
		start := time.Now()

		got := runBounded(t, 7*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		callsA := fx.pool.getCallsFor(testLocalA)
		require.Len(t, callsA, 1)
		require.True(t, callsA[0].bounded, "dial of A must be bounded")
		assert.Equal(t, start.Add(localPeerConnectTimeout), callsA[0].deadline)
		assert.Equal(t, localPeerConnectTimeout, time.Since(start), "B must be dialed right after A's budget")
		fx.requireStruck(t, testLocalA, 1)
		fx.requireNotStruck(t, testLocalB)
		assert.Equal(t, []string{testLocalB}, fx.filterBannedPeers([]string{testLocalA, testLocalB}))
		banA, _ := fx.banOf(testLocalA)
		assert.Equal(t, start.Add(localPeerConnectTimeout).Add(localPeerBanMin), banA.until, "first ban is localPeerBanMin")
		assert.Equal(t, 0, fx.pool.nodeCalls())
	})
}

func TestLocalPeer_ConnectDialTimeoutLeavesTheNextCandidateUnstruck(t *testing.T) {
	// B answers with an application error, so neither a strike nor a reset
	// touches B's ban state after the connect phase: an implementation that
	// strikes the whole list on A's timeout is visible here (in the test
	// above B's success would reset it).
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA, testLocalB)
		connB := newProgressConn(0)
		connB.script = fail(drpcerr.WithCode(errors.New("remote: CID not found"), rpcerr.Code(fileprotoerr.ErrCIDNotFound)))
		fx.pool.gets[testLocalA] = blockingGet(nil)
		fx.pool.gets[testLocalB] = immediateGet(peerWithConn(testLocalB, connB))

		got := runBounded(t, 7*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, format.ErrNotFound{})
		fx.requireStruck(t, testLocalA, 1)
		fx.requireNotStruck(t, testLocalB)
	})
}

func TestLocalPeer_ConnectScanPrefersConnectedPeerWithoutWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA, testLocalB)
		want := []byte("from-B")
		connB := newProgressConn(0)
		connB.script = answer(want)
		peerB := peerWithConn(testLocalB, connB)
		// A is "loading" in the pool: a Pick with a live ctx would wait for it
		fx.pool.picks[testLocalA] = func(ctx context.Context) (peer.Peer, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		fx.pool.picks[testLocalB] = immediateGet(peerB)
		fx.pool.gets[testLocalA] = blockingGet(nil)
		fx.pool.gets[testLocalB] = immediateGet(peerB)
		start := time.Now()

		got := runBounded(t, 7*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		assert.Equal(t, time.Duration(0), time.Since(start), "a connected peer must be used without waiting")
		picksA := fx.pool.pickCallsFor(testLocalA)
		require.Len(t, picksA, 1)
		assert.True(t, picksA[0].ctxDone, "the scan must probe with a done ctx so it cannot block")
		assert.Empty(t, fx.pool.getCallsFor(testLocalA), "A must not be dialed while B is connected")
		fx.requireNotStruck(t, testLocalA)
		fx.requireNotStruck(t, testLocalB)
	})
}

func TestLocalPeer_ConnectFailureClassification(t *testing.T) {
	type tc struct {
		name       string
		getA       func(ctx context.Context) (peer.Peer, error)
		acquireA   func(ctx context.Context) (drpc.Conn, error)
		wantStrike bool
		wantErr    error // nil: the block must come from B
		wantB      bool
	}
	cases := []tc{
		{
			name:  "aborted shared load (Canceled with our ctx alive) does not strike",
			getA:  func(context.Context) (peer.Peer, error) { return nil, context.Canceled },
			wantB: true,
		},
		{
			name:  "foreign shared-load DeadlineExceeded with our ctx alive does not strike",
			getA:  func(context.Context) (peer.Peer, error) { return nil, context.DeadlineExceeded },
			wantB: true,
		},
		{
			name:       "dial ErrConnClosed strikes (only Acquire is exempt)",
			getA:       func(context.Context) (peer.Peer, error) { return nil, transport.ErrConnClosed },
			wantStrike: true,
			wantB:      true,
		},
		{
			name:    "pool closing returns at once without striking",
			getA:    func(context.Context) (peer.Peer, error) { return nil, ocache.ErrClosed },
			wantErr: ocache.ErrClosed,
		},
		{
			name:       "immediate dial error strikes",
			getA:       func(context.Context) (peer.Peer, error) { return nil, errors.New("dial tcp: connection refused") },
			wantStrike: true,
			wantB:      true,
		},
		{
			name: "acquire Canceled with our ctx alive does not strike",
			acquireA: func(context.Context) (drpc.Conn, error) {
				return nil, context.Canceled
			},
			wantB: true,
		},
		{
			name: "acquire DeadlineExceeded with our ctx alive does not strike",
			acquireA: func(context.Context) (drpc.Conn, error) {
				return nil, context.DeadlineExceeded
			},
			wantB: true,
		},
		{
			name: "acquire ErrConnClosed does not strike",
			acquireA: func(context.Context) (drpc.Conn, error) {
				return nil, transport.ErrConnClosed
			},
			wantB: true,
		},
		{
			name: "acquire handshake error strikes",
			acquireA: func(context.Context) (drpc.Conn, error) {
				return nil, errors.New("handshake: remote incompatible proto")
			},
			wantStrike: true,
			wantB:      true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fx := newLocalFixture(t, testLocalA, testLocalB)
				want := []byte("from-B")
				connB := newProgressConn(0)
				connB.script = answer(want)
				peerB := peerWithConn(testLocalB, connB)
				fx.pool.gets[testLocalB] = immediateGet(peerB)
				if c.getA != nil {
					fx.pool.gets[testLocalA] = c.getA
				} else {
					fx.pool.gets[testLocalA] = immediateGet(newFakePeer(testLocalA, c.acquireA))
				}

				got := runBounded(t, 7*time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
				})

				if c.wantErr != nil {
					require.ErrorIs(t, got.err, c.wantErr)
					assert.Empty(t, fx.pool.getCallsFor(testLocalB), "must return at once, not try B")
				} else {
					require.NoError(t, got.err)
					assert.Equal(t, want, got.data)
				}
				if c.wantStrike {
					fx.requireStruck(t, testLocalA, 1)
				} else {
					fx.requireNotStruck(t, testLocalA)
				}
				fx.requireNotStruck(t, testLocalB)
			})
		})
	}
}

func TestLocalPeer_ConnectDialAndAcquireShareOneBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA, testLocalB)
		want := []byte("from-B")
		connB := newProgressConn(0)
		connB.script = answer(want)
		peerB := peerWithConn(testLocalB, connB)
		var acquireDeadline time.Time
		peerA := newFakePeer(testLocalA, func(ctx context.Context) (drpc.Conn, error) {
			acquireDeadline, _ = ctx.Deadline()
			<-ctx.Done()
			return nil, ctx.Err()
		})
		// the dial of A succeeds after 60% of the budget; the acquire must
		// then get only the remaining 40%, not a fresh budget
		dialTook := localPeerConnectTimeout * 3 / 5
		fx.pool.gets[testLocalA] = func(ctx context.Context) (peer.Peer, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(dialTook):
				return peerA, nil
			}
		}
		fx.pool.gets[testLocalB] = immediateGet(peerB)
		start := time.Now()

		got := runBounded(t, 10*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		assert.Equal(t, start.Add(localPeerConnectTimeout), acquireDeadline, "acquire must share the dial's budget")
		assert.Equal(t, localPeerConnectTimeout, time.Since(start))
		fx.requireStruck(t, testLocalA, 1)
		acquires, releases := peerA.stats()
		assert.Equal(t, 1, acquires)
		assert.Empty(t, releases, "nothing to release after a failed acquire")
	})
}

func TestLocalPeer_CallerDeadlineDuringDialDoesNotStrike(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		fx.pool.gets[testLocalA] = blockingGet(nil)
		ctx, cancel := context.WithTimeout(fx.ctx(), time.Second)
		defer cancel()

		got := runBounded(t, 3*time.Second, func() ([]byte, error) {
			_, _, err := fx.connectLocalPeer(ctx, []string{testLocalA})
			return nil, err
		})

		require.ErrorIs(t, got.err, context.DeadlineExceeded)
		assert.NotErrorIs(t, got.err, net.ErrUnableToConnect)
		fx.requireNotStruck(t, testLocalA)
	})
}

func TestLocalPeer_CallerCancelDuringAcquireDoesNotStrike(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		peerA := newFakePeer(testLocalA, func(ctx context.Context) (drpc.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		fx.pool.gets[testLocalA] = immediateGet(peerA)
		ctx, cancel := context.WithCancel(fx.ctx())
		defer cancel()
		time.AfterFunc(time.Second, cancel)

		got := runBounded(t, 3*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(ctx, testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, context.Canceled)
		assert.NotErrorIs(t, got.err, net.ErrUnableToConnect)
		fx.requireNotStruck(t, testLocalA)
		acquires, _ := peerA.stats()
		assert.Equal(t, 1, acquires)
	})
}

func TestLocalPeer_EachSlowCandidateGetsAFreshConnectBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const testLocalC = "localC"
		fx := newLocalFixture(t, testLocalA, testLocalB, testLocalC)
		want := []byte("from-C")
		connC := newProgressConn(0)
		connC.script = answer(want)
		fx.pool.gets[testLocalA] = blockingGet(nil)
		fx.pool.gets[testLocalB] = blockingGet(nil)
		fx.pool.gets[testLocalC] = immediateGet(peerWithConn(testLocalC, connC))
		start := time.Now()

		got := runBounded(t, 12*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		callsA, callsB := fx.pool.getCallsFor(testLocalA), fx.pool.getCallsFor(testLocalB)
		require.Len(t, callsA, 1)
		require.Len(t, callsB, 1)
		assert.Equal(t, start.Add(localPeerConnectTimeout), callsA[0].deadline)
		assert.Equal(t, start.Add(2*localPeerConnectTimeout), callsB[0].deadline, "B gets its own budget after A's")
		assert.Equal(t, 2*localPeerConnectTimeout, time.Since(start))
		fx.requireStruck(t, testLocalA, 1)
		fx.requireStruck(t, testLocalB, 1)
		fx.requireNotStruck(t, testLocalC)
	})
}

func TestLocalPeer_ConnectAllFailedReportsUnableToConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA, testLocalB)
		errA := errors.New("dial A: connection refused")
		errB := errors.New("dial B: no route to host")
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) { return nil, errA }
		fx.pool.gets[testLocalB] = func(context.Context) (peer.Peer, error) { return nil, errB }

		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, net.ErrUnableToConnect)
		assert.ErrorIs(t, got.err, errB, "the last candidate's error must survive")
		fx.requireStruck(t, testLocalA, 1)
		fx.requireStruck(t, testLocalB, 1)
	})
}

// ---- ban backoff ----

func TestLocalPeer_BanBackoff(t *testing.T) {
	errX := errors.New("x")
	t.Run("concurrent failures during one ban do not escalate", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			start := time.Now()

			// four GetMany workers failing on the same outage
			var wg sync.WaitGroup
			for i := 0; i < getManyWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					fx.strikeLocalPeer(testLocalA, "test", errX)
				}()
			}
			wg.Wait()

			b, _ := fx.banOf(testLocalA)
			assert.Equal(t, 1, b.strikes)
			assert.Equal(t, start.Add(localPeerBanMin), b.until, "one outage is one 10 s ban")
			assert.Empty(t, fx.filterBannedPeers([]string{testLocalA}))

			// expiry lets the peer through but keeps the strike count
			time.Sleep(localPeerBanMin + time.Millisecond)
			assert.Equal(t, []string{testLocalA}, fx.filterBannedPeers([]string{testLocalA}))
			b, ok := fx.banOf(testLocalA)
			require.True(t, ok, "expired entry must be kept for the backoff")
			assert.Equal(t, 1, b.strikes)

			// a failure after expiry escalates
			fx.strikeLocalPeer(testLocalA, "test", errX)
			b, _ = fx.banOf(testLocalA)
			assert.Equal(t, 2, b.strikes)
			assert.Equal(t, time.Now().Add(20*time.Second), b.until)
		})
	})
	t.Run("a failure late in an active ban neither escalates nor extends it", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			start := time.Now()
			fx.strikeLocalPeer(testLocalA, "test", errX)
			time.Sleep(localPeerBanMin - time.Second)

			fx.strikeLocalPeer(testLocalA, "test", errX)

			b, _ := fx.banOf(testLocalA)
			want := localPeerBan{until: start.Add(localPeerBanMin), strikes: 1}
			assert.Equal(t, want, b)
		})
	})
	t.Run("each failure after expiry doubles up to the cap", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			wants := []time.Duration{
				10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second,
				160 * time.Second, 300 * time.Second, 300 * time.Second,
			}
			for i, want := range wants {
				fx.strikeLocalPeer(testLocalA, "test", errX)
				b, _ := fx.banOf(testLocalA)
				assert.Equal(t, i+1, b.strikes)
				assert.Equal(t, time.Now().Add(want), b.until, "ban after strike %d", i+1)
				time.Sleep(want + time.Millisecond)
			}
		})
	})
	t.Run("success resets", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			fx.strikeLocalPeer(testLocalA, "test", errX)
			time.Sleep(localPeerBanMin + time.Millisecond)
			fx.strikeLocalPeer(testLocalA, "test", errX)

			fx.resetLocalPeer(testLocalA)
			fx.requireNotStruck(t, testLocalA)
			fx.strikeLocalPeer(testLocalA, "test", errX)
			b, _ := fx.banOf(testLocalA)
			assert.Equal(t, 1, b.strikes)
			assert.Equal(t, time.Now().Add(localPeerBanMin), b.until)
		})
	})
	t.Run("localPeerBanMax of quiet after expiry resets", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			fx.strikeLocalPeer(testLocalA, "test", errX)
			time.Sleep(localPeerBanMin + time.Millisecond)
			fx.strikeLocalPeer(testLocalA, "test", errX)
			b, _ := fx.banOf(testLocalA)
			require.Equal(t, 2, b.strikes)

			time.Sleep(20*time.Second + localPeerBanMax + time.Second)
			fx.strikeLocalPeer(testLocalA, "test", errX)
			b, _ = fx.banOf(testLocalA)
			assert.Equal(t, 1, b.strikes)
			assert.Equal(t, time.Now().Add(localPeerBanMin), b.until)
		})
	})
}

// ---- GetMany ----

// connFactory hands out a new conn per acquire; conns are recorded for
// assertions
type connFactory struct {
	mu    sync.Mutex
	make  func(i int) *progressConn
	conns []*progressConn
}

func (f *connFactory) acquire(context.Context) (drpc.Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.make(len(f.conns))
	f.conns = append(f.conns, c)
	return c, nil
}

func (f *connFactory) all() []*progressConn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.conns)
}

func collect(t *testing.T, ch <-chan blocks.Block, limit time.Duration) []blocks.Block {
	t.Helper()
	var res []blocks.Block
	deadline := time.After(limit)
	for {
		select {
		case b, ok := <-ch:
			if !ok {
				return res
			}
			res = append(res, b)
		case <-deadline:
			t.Fatalf("GetMany did not finish within %v of fake time (got %d blocks)", limit, len(res))
		}
	}
}

func TestLocalPeer_GetManyFourWorkersStallTogether(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		factory := &connFactory{make: func(int) *progressConn {
			c := newProgressConn(0)
			c.script = silent()
			return c
		}}
		local := newFakePeer(testLocalA, factory.acquire)
		fx.pool.gets[testLocalA] = immediateGet(local)
		var cids []cid.Cid
		for i := 0; i < 8; i++ {
			cids = append(cids, testCid(string(rune('a'+i))))
		}

		start := time.Now()
		ch := fx.GetMany(fx.ctx(), cids)
		synctest.Wait()
		// exactly getManyWorkers fetches are in flight on distinct conns
		conns := factory.all()
		require.Len(t, conns, getManyWorkers)
		for _, c := range conns {
			assert.Equal(t, int32(1), c.inflight.Load())
		}

		got := collect(t, ch, 10*time.Second)

		require.Len(t, got, 8)
		for _, c := range factory.all() {
			assert.Len(t, c.cancellations(), 1)
		}
		assert.Len(t, factory.all(), getManyWorkers, "after the ban no worker touches the local peer")
		acquires, releases := local.stats()
		assert.Equal(t, getManyWorkers, acquires)
		assert.Len(t, releases, getManyWorkers)
		assert.Equal(t, 8, fx.pool.nodeCalls())
		// the four simultaneous stalls are one outage: one strike, one 10 s ban
		b, _ := fx.banOf(testLocalA)
		assert.Equal(t, 1, b.strikes)
		assert.Equal(t, start.Add(localPeerStallTimeout).Add(localPeerBanMin), b.until)
	})
}

func TestLocalPeer_GetManyOnlyTheStalledConnIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		want := []byte("from-local")
		factory := &connFactory{}
		factory.make = func(i int) *progressConn {
			c := newProgressConn(0)
			if i == 0 {
				c.script = silent()
			} else {
				// keeps progressing well past the silent conn's stall window
				c.script = progressing(c, 8*time.Second, 500*time.Millisecond, 4096, want)
			}
			return c
		}
		local := newFakePeer(testLocalA, factory.acquire)
		fx.pool.gets[testLocalA] = immediateGet(local)
		var cids []cid.Cid
		for i := 0; i < 8; i++ {
			cids = append(cids, testCid(string(rune('a'+i))))
		}
		start := time.Now()

		ch := fx.GetMany(fx.ctx(), cids)
		time.Sleep(localPeerStallTimeout + localPeerStallCheckInterval)
		synctest.Wait()

		// at 6 s: the silent conn was cut in its stall window while the healthy
		// transfers on the same peer are still running
		conns := factory.all()
		require.Len(t, conns, getManyWorkers)
		cancels := conns[0].cancellations()
		require.Len(t, cancels, 1, "the silent conn is cancelled")
		since := cancels[0].Sub(start)
		assert.GreaterOrEqual(t, since, localPeerStallTimeout)
		assert.LessOrEqual(t, since, localPeerStallTimeout+localPeerStallCheckInterval)
		for i, c := range conns[1:] {
			assert.Equal(t, int32(1), c.inflight.Load(), "healthy transfer %d must still be active", i+1)
			assert.Empty(t, c.cancellations(), "healthy transfer %d must not be cancelled", i+1)
			assert.Zero(t, c.closes.Load(), "healthy transfer %d's conn must stay open", i+1)
		}
		// the stall is a per-conn verdict: the peer and its other sub-conns
		// stay up
		assert.False(t, local.IsClosed(), "a stalled conn must not close the peer")
		assert.Zero(t, local.closeCount())

		got := collect(t, ch, 15*time.Second)

		require.Len(t, got, 8)
		var fromLocal int
		for _, b := range got {
			if string(b.RawData()) == string(want) {
				fromLocal++
			}
		}
		for i, c := range factory.all()[1:] {
			assert.Empty(t, c.cancellations(), "healthy transfer %d must complete", i+1)
		}
		// the stall struck the peer: the three healthy transfers complete, the
		// stalled block and the four not yet started go to the node
		assert.Equal(t, getManyWorkers-1, fromLocal)
		assert.Equal(t, 8-(getManyWorkers-1), fx.pool.nodeCalls())
		acquires, releases := local.stats()
		assert.Equal(t, acquires, len(releases))
		assert.False(t, local.IsClosed())
		var pooled int
		for _, r := range releases {
			if r.pooled {
				pooled++
			}
		}
		assert.Equal(t, getManyWorkers-1, pooled, "the healthy conns go back to the live peer")
	})
}

func TestLocalPeer_GetManyCancelWhileWorkersHoldBlocks(t *testing.T) {
	// Regression: the dispatcher used to close resultCh when ctx ended while
	// it waited for a worker slot, without waiting for the workers; a worker
	// that finished its fetch afterwards could send on the closed channel.
	for round := 0; round < 5; round++ {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			gate := make(chan struct{})
			var gateOnce sync.Once
			release := func() { gateOnce.Do(func() { close(gate) }) }
			// a failed check below must still let the workers finish, or the
			// bubble panics on deadlock instead of reporting the failure
			defer release()
			factory := &connFactory{make: func(int) *progressConn {
				c := newProgressConn(0)
				// ignores ctx on purpose: the fetch completes after the cancel
				c.script = func(context.Context) ([]byte, error) {
					<-gate
					return []byte("late"), nil
				}
				return c
			}}
			local := newFakePeer(testLocalA, factory.acquire)
			fx.pool.gets[testLocalA] = immediateGet(local)
			var cids []cid.Cid
			for i := 0; i < getManyWorkers+2; i++ {
				cids = append(cids, testCid(string(rune('a'+i))))
			}
			ctx, cancel := context.WithCancel(fx.ctx())

			ch := fx.GetMany(ctx, cids)
			synctest.Wait() // four workers blocked on the gate, dispatcher blocked on a slot
			cancel()
			synctest.Wait()
			// the dispatcher has seen the cancel; the workers still hold their
			// blocks, so the channel must still be open
			select {
			case _, ok := <-ch:
				require.True(t, ok, "result channel closed while workers were still running")
				t.Fatal("unexpected block before the workers were released")
			default:
			}
			release()

			// must not panic; the channel closes only once every worker returned
			collect(t, ch, time.Second)
			acquires, releases := local.stats()
			assert.Equal(t, getManyWorkers, acquires)
			assert.Len(t, releases, getManyWorkers)
		})
	}
}

// ---- watchdog unit tests ----

func TestWatchFetchProgress_StopJoinsAndNeverCancelsAfterwards(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		conn := newProgressConn(0)
		stop := watchFetchProgress(ctx, conn, cancel)
		time.Sleep(4 * time.Second) // one tick short of the window
		stop()
		stop() // idempotent
		polls := conn.polls.Load()
		time.Sleep(localPeerFetchFallbackTimeout + localPeerStallTimeout)
		assert.NoError(t, ctx.Err(), "a stopped watchdog must never cancel")
		assert.Equal(t, polls, conn.polls.Load(), "a stopped watchdog must not poll")
	})
}

// gatedPollConn blocks its second BytesRead call (the first is the
// watchdog's baseline) until gate is closed, simulating a poll in flight.
type gatedPollConn struct {
	*plainConn
	gate  chan struct{}
	polls atomic.Int32
}

func (c *gatedPollConn) BytesRead() int64 {
	if c.polls.Add(1) == 2 {
		<-c.gate
	}
	return 0
}

func TestWatchFetchProgress_StopWaitsForAPollInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		conn := &gatedPollConn{plainConn: newPlainConn(nil), gate: make(chan struct{})}
		stop := watchFetchProgress(ctx, conn, cancel)
		time.Sleep(localPeerStallCheckInterval) // first tick: the poll is now blocked on the gate
		synctest.Wait()
		require.Equal(t, int32(2), conn.polls.Load())

		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("stop returned while the watchdog was still polling")
		default:
		}

		close(conn.gate)
		synctest.Wait()
		select {
		case <-stopped:
		default:
			t.Fatal("stop did not return after the watchdog exited")
		}
		assert.NoError(t, ctx.Err())
	})
}

func TestWatchFetchProgress_ExitsWhenCallerCancels(t *testing.T) {
	// stallCancels counts the watchdog's own cancels: a goroutine that ignored
	// ctx.Done would keep polling and, at the end of its window, cancel
	watch := func(ctx context.Context, conn drpc.Conn, cancel context.CancelCauseFunc) (stop func(), stallCancels *atomic.Int32) {
		stallCancels = &atomic.Int32{}
		stop = watchFetchProgress(ctx, conn, func(cause error) {
			if errors.Is(cause, errLocalPeerStalled) {
				stallCancels.Add(1)
			}
			cancel(cause)
		})
		return stop, stallCancels
	}
	t.Run("counter", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			parent, parentCancel := context.WithCancel(context.Background())
			ctx, cancel := context.WithCancelCause(parent)
			defer cancel(nil)
			conn := newProgressConn(0)
			stop, stallCancels := watch(ctx, conn, cancel)
			time.Sleep(2 * localPeerStallCheckInterval)
			parentCancel()
			synctest.Wait()
			polls := conn.polls.Load()

			time.Sleep(localPeerStallTimeout + 3*localPeerStallCheckInterval)
			synctest.Wait()

			assert.Equal(t, polls, conn.polls.Load(), "polling must stop on the caller's cancel, before stop()")
			assert.Equal(t, int32(0), stallCancels.Load())
			stop()
			assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
			assert.NotErrorIs(t, context.Cause(ctx), errLocalPeerStalled)
		})
	})
	t.Run("fallback timer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			parent, parentCancel := context.WithCancel(context.Background())
			ctx, cancel := context.WithCancelCause(parent)
			defer cancel(nil)
			stop, stallCancels := watch(ctx, newPlainConn(nil), cancel)
			parentCancel()

			time.Sleep(localPeerFetchFallbackTimeout + localPeerStallCheckInterval)
			synctest.Wait()

			assert.Equal(t, int32(0), stallCancels.Load(), "the fallback timer must be abandoned on the caller's cancel, before stop()")
			stop()
			assert.NotErrorIs(t, context.Cause(ctx), errLocalPeerStalled)
		})
	})
}

// pollScriptConn answers every BytesRead with poll(n); n counts from 1, the
// watchdog's baseline read
type pollScriptConn struct {
	*plainConn
	n    atomic.Int32
	poll func(n int32) int64
}

func (c *pollScriptConn) BytesRead() int64 { return c.poll(c.n.Add(1)) }

func TestWatchFetchProgress_LateTickDoesNotBackdateProgress(t *testing.T) {
	// A ticker delivers the time a tick was scheduled, not the time it was
	// received. After the goroutine was held up (process suspension,
	// starvation), progress seen on a late tick must be stamped with the
	// actual time, or the next ticks measure the window from the past.
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		conn := &pollScriptConn{plainConn: newPlainConn(nil)}
		conn.poll = func(n int32) int64 {
			switch n {
			case 1, 2:
				if n == 2 {
					// the poll of the 1 s tick is held up until 4.5 s, so the
					// tick scheduled at 2 s is received at 4.5 s
					time.Sleep(3500 * time.Millisecond)
				}
				return 0
			default:
				return 1 // bytes arrived meanwhile: progress observed at 4.5 s
			}
		}
		start := time.Now()
		stop := watchFetchProgress(ctx, conn, cancel)
		defer stop()

		// the ticker keeps fake time moving, so a watchdog that never cancels
		// would not trip the bubble's deadlock detector: bound the wait
		select {
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
			t.Fatal("the watchdog did not cancel within 30 s of fake time")
		}

		since := time.Since(start)
		progressSeen := 4500 * time.Millisecond
		assert.GreaterOrEqual(t, since, progressSeen+localPeerStallTimeout, "stall measured from a backdated tick: %v", since)
		assert.LessOrEqual(t, since, progressSeen+localPeerStallTimeout+localPeerStallCheckInterval)
		assert.ErrorIs(t, context.Cause(ctx), errLocalPeerStalled)
	})
}

func TestWatchFetchProgress_StopWinsOverAStallDecidedConcurrently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		gate := make(chan struct{})
		conn := &pollScriptConn{plainConn: newPlainConn(nil)}
		// polls: baseline (1) then one per tick; poll 6 is the tick at exactly
		// localPeerStallTimeout, which decides the stall
		stallPoll := int32(localPeerStallTimeout/localPeerStallCheckInterval) + 1
		conn.poll = func(n int32) int64 {
			if n == stallPoll {
				<-gate
			}
			return 0
		}
		stop := watchFetchProgress(ctx, conn, cancel)
		time.Sleep(localPeerStallTimeout)
		synctest.Wait()
		require.Equal(t, stallPoll, conn.n.Load(), "the threshold poll must be in flight")

		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		synctest.Wait() // stop has begun and waits for the watchdog
		close(gate)
		<-stopped

		assert.NoError(t, ctx.Err(), "no stall cancel may happen once stop has begun")
	})
}

// ---- conn reuse ----

func TestLocalPeer_ConnReuse(t *testing.T) {
	t.Run("successful fetch re-pools the conn for the next fetch", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			want := []byte("from-local")
			factory := &connFactory{make: func(int) *progressConn {
				c := newProgressConn(0)
				c.script = answer(want)
				return c
			}}
			local := newFakePeer(testLocalA, factory.acquire)
			fx.pool.gets[testLocalA] = immediateGet(local)

			for _, k := range []string{"a", "b"} {
				got := runBounded(t, time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid(k))
				})
				require.NoError(t, got.err)
				assert.Equal(t, want, got.data)
			}

			conns := factory.all()
			require.Len(t, conns, 1, "the second fetch must reuse the first conn")
			assert.Equal(t, int32(2), conns[0].invokes.Load())
			assert.Equal(t, int32(0), conns[0].closes.Load(), "a healthy conn must not be closed")
			_, releases := local.stats()
			require.Len(t, releases, 2)
			for _, r := range releases {
				assert.True(t, r.pooled)
			}
		})
	})
	cases := []struct {
		name    string
		makeCtx func(ctx context.Context) (context.Context, context.CancelFunc)
	}{
		{
			name: "stalled fetch is not reused",
			makeCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return ctx, func() {}
			},
		},
		{
			name: "caller-cancelled fetch is not reused",
			makeCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				time.AfterFunc(time.Second, cancel)
				return ctx, cancel
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fx := newLocalFixture(t, testLocalA)
				want := []byte("from-local")
				factory := &connFactory{make: func(i int) *progressConn {
					c := newProgressConn(0)
					if i == 0 {
						c.script = silent()
					} else {
						c.script = answer(want)
					}
					return c
				}}
				local := newFakePeer(testLocalA, factory.acquire)
				fx.pool.gets[testLocalA] = immediateGet(local)
				ctx, cancel := tc.makeCtx(fx.ctx())
				defer cancel()

				got := runBounded(t, 7*time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(ctx, testSpaceId, testCid("a"))
				})
				require.Error(t, got.err)
				fx.resetLocalPeer(testLocalA) // lift a stall ban for the second fetch
				got = runBounded(t, time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("b"))
				})

				require.NoError(t, got.err)
				assert.Equal(t, want, got.data)
				conns := factory.all()
				require.Len(t, conns, 2, "the cut conn must not be handed out again")
				assert.Equal(t, int32(1), conns[0].invokes.Load())
				assert.Equal(t, int32(1), conns[0].closes.Load(), "the cut conn is closed")
				assert.Equal(t, int32(0), conns[1].closes.Load())
			})
		})
	}
}

// ---- review round 2 ----

func TestLocalPeer_AcquireFailureOfAClosedPeer(t *testing.T) {
	// pool.Flush closing the yamux session while AcquireDrpcConn waits for the
	// proto handshake answer surfaces as a raw io.EOF, not ErrConnClosed; with
	// the peer closed and our connect budget alive it says nothing about the
	// remote
	cases := []struct {
		name       string
		acquire    func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error)
		wantStrike bool
	}{
		{
			name: "io.EOF from a peer closed during acquire does not strike",
			acquire: func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error) {
				return func(context.Context) (drpc.Conn, error) {
					p.closed.Store(true)
					return nil, io.EOF
				}
			},
		},
		{
			name: "io.EOF from an open peer strikes",
			acquire: func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error) {
				return func(context.Context) (drpc.Conn, error) { return nil, io.EOF }
			},
			wantStrike: true,
		},
		{
			name: "connect budget expiring strikes even when the peer is closed",
			acquire: func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error) {
				return func(ctx context.Context) (drpc.Conn, error) {
					<-ctx.Done()
					p.closed.Store(true)
					return nil, io.EOF
				}
			},
			wantStrike: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fx := newLocalFixture(t, testLocalA, testLocalB)
				want := []byte("from-B")
				connB := newProgressConn(0)
				connB.script = answer(want)
				peerA := newFakePeer(testLocalA, nil)
				peerA.acquire = tc.acquire(peerA)
				fx.pool.gets[testLocalA] = immediateGet(peerA)
				fx.pool.gets[testLocalB] = immediateGet(peerWithConn(testLocalB, connB))

				got := runBounded(t, 7*time.Second, func() ([]byte, error) {
					return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
				})

				require.NoError(t, got.err)
				assert.Equal(t, want, got.data)
				if tc.wantStrike {
					fx.requireStruck(t, testLocalA, 1)
				} else {
					fx.requireNotStruck(t, testLocalA)
				}
				fx.requireNotStruck(t, testLocalB)
			})
		})
	}
}

func TestWatchFetchProgress_StaleSampleDoesNotStall(t *testing.T) {
	// The watchdog is descheduled between reading the counter and reading
	// the clock while bytes keep arriving: the stale, unchanged count must
	// not be compared against the fresh time.
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		start := time.Now()
		// one byte at 0.5 s, nothing until 2.5 s, then a byte every 500 ms
		bytesAt := func(now time.Time) int64 {
			elapsed := now.Sub(start)
			switch {
			case elapsed < 500*time.Millisecond:
				return 0
			case elapsed < 2500*time.Millisecond:
				return 1
			default:
				return int64(elapsed / (500 * time.Millisecond))
			}
		}
		conn := &pollScriptConn{plainConn: newPlainConn(nil)}
		conn.poll = func(n int32) int64 {
			sample := bytesAt(time.Now())
			if n == 3 {
				// the 2 s tick samples 1 (unchanged since the 1 s tick) and is
				// descheduled until 7 s, past the 5 s window, while bytes arrive
				time.Sleep(localPeerStallTimeout)
			}
			return sample
		}
		stop := watchFetchProgress(ctx, conn, cancel)

		time.Sleep(15 * time.Second)
		stop()

		assert.NoError(t, ctx.Err(), "a progressing fetch was cancelled on a stale sample")
		assert.Greater(t, conn.n.Load(), int32(3), "the watchdog must keep polling after the stale sample")
	})
}

func TestWatchFetchProgress_StopEndsTheFallbackTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		stop := watchFetchProgress(ctx, newPlainConn(nil), cancel)
		time.Sleep(time.Second)
		start := time.Now()

		stop()

		assert.Equal(t, time.Duration(0), time.Since(start), "stop must not wait out the fallback timer")
		time.Sleep(localPeerFetchFallbackTimeout + localPeerStallCheckInterval)
		assert.NoError(t, ctx.Err(), "a stopped fallback timer must never cancel")
	})
}

func TestLocalPeer_SuccessfulFetchWithoutCounterStopsTheFallbackTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		want := []byte("from-local")
		conn := newPlainConn(answer(want))
		local := peerWithConn(testLocalA, conn)
		fx.pool.gets[testLocalA] = immediateGet(local)
		start := time.Now()

		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.NoError(t, got.err)
		assert.Equal(t, want, got.data)
		assert.Equal(t, time.Duration(0), time.Since(start))
		time.Sleep(localPeerFetchFallbackTimeout + localPeerStallCheckInterval)
		_, releases := local.stats()
		require.Len(t, releases, 1)
		assert.True(t, releases[0].pooled, "the conn must be re-pooled")
		assert.Equal(t, int32(0), conn.closes.Load(), "nothing may cancel or close the conn later")
		assert.Empty(t, conn.cancellations())
	})
}

func TestLocalPeer_NonStrikingErrorsKeepStrikeHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		errX := errors.New("x")
		// two strikes, both bans expired
		fx.strikeLocalPeer(testLocalA, "test", errX)
		time.Sleep(localPeerBanMin + time.Millisecond)
		fx.strikeLocalPeer(testLocalA, "test", errX)
		time.Sleep(2*localPeerBanMin + time.Millisecond)
		seeded, _ := fx.banOf(testLocalA)
		require.Equal(t, 2, seeded.strikes)

		const (
			stepNotFound = iota
			stepCallerCancel
			stepEOF
		)
		var step atomic.Int32
		script := func(ctx context.Context) ([]byte, error) {
			switch step.Load() {
			case stepNotFound:
				return nil, drpcerr.WithCode(errors.New("remote: CID not found"), rpcerr.Code(fileprotoerr.ErrCIDNotFound))
			case stepCallerCancel:
				<-ctx.Done()
				return nil, ctx.Err()
			default:
				return nil, io.EOF
			}
		}
		factory := &connFactory{make: func(int) *progressConn {
			c := newProgressConn(0)
			c.script = script
			return c
		}}
		fx.pool.gets[testLocalA] = immediateGet(newFakePeer(testLocalA, factory.acquire))

		// an application error: the peer answered
		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})
		require.ErrorIs(t, got.err, format.ErrNotFound{})
		b, _ := fx.banOf(testLocalA)
		assert.Equal(t, seeded, b, "an application error must not touch the strike history")

		// the caller ends the fetch
		step.Store(stepCallerCancel)
		ctx, cancel := context.WithCancel(fx.ctx())
		time.AfterFunc(time.Second, cancel)
		got = runBounded(t, 3*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(ctx, testSpaceId, testCid("b"))
		})
		cancel()
		require.ErrorIs(t, got.err, context.Canceled)
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, seeded, b, "a caller cancellation must not touch the strike history")

		// the next real failure is the third strike
		step.Store(stepEOF)
		got = runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("c"))
		})
		require.ErrorIs(t, got.err, io.EOF)
		b, _ = fx.banOf(testLocalA)
		want := localPeerBan{until: time.Now().Add(4 * localPeerBanMin), strikes: 3}
		assert.Equal(t, want, b)
	})
}

func TestLocalPeer_NodeFallbackIsNotBoundedByLocalBudgets(t *testing.T) {
	// the node fetch runs under the caller's ctx only: a node answer slower
	// than every local budget must still arrive
	const nodeLatency = 20 * time.Second
	newSlowNodeFixture := func(t *testing.T) *localFixture {
		fx := newLocalFixture(t, testLocalA)
		fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) {
			return nil, errors.New("dial tcp: connection refused")
		}
		fx.setNodeScript(func(ctx context.Context) ([]byte, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(nodeLatency):
				return fx.nodeData, nil
			}
		})
		return fx
	}
	t.Run("Get", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newSlowNodeFixture(t)
			ctx, cancel := context.WithTimeout(fx.ctx(), time.Minute)
			defer cancel()
			start := time.Now()

			got := runBounded(t, 2*nodeLatency, func() ([]byte, error) {
				b, err := fx.Get(ctx, testCid("a"))
				if err != nil {
					return nil, err
				}
				return b.RawData(), nil
			})

			require.NoError(t, got.err)
			assert.Equal(t, fx.nodeData, got.data)
			assert.Equal(t, nodeLatency, time.Since(start))
		})
	})
	t.Run("GetMany", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newSlowNodeFixture(t)
			ctx, cancel := context.WithTimeout(fx.ctx(), time.Minute)
			defer cancel()
			cids := []cid.Cid{testCid("a"), testCid("b"), testCid("c")}

			got := collect(t, fx.GetMany(ctx, cids), 2*nodeLatency)

			require.Len(t, got, len(cids))
			for _, b := range got {
				assert.Equal(t, fx.nodeData, b.RawData())
			}
		})
	})
}

func TestLocalPeer_CallerCancelCoincidingWithAStallDoesNotStrike(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		ctx, cancel := context.WithCancel(fx.ctx())
		defer cancel()
		conn := newProgressConn(0)
		// the caller gives up right as the watchdog decides the stall: by the
		// time the fetch returns, the caller's ctx is done too
		conn.script = func(fetchCtx context.Context) ([]byte, error) {
			<-fetchCtx.Done()
			cancel()
			return nil, fetchCtx.Err()
		}
		fx.pool.gets[testLocalA] = immediateGet(peerWithConn(testLocalA, conn))

		got := runBounded(t, 7*time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(ctx, testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, errLocalPeerStalled, "the watchdog decided first")
		fx.requireNotStruck(t, testLocalA)
	})
}

func TestLocalPeer_CandidateOrderIsShuffled(t *testing.T) {
	t.Run("connect uses the shuffled order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA, testLocalB)
			var shuffled [][]string
			fx.shuffleLocalPeers = func(ids []string) {
				shuffled = append(shuffled, slices.Clone(ids))
				slices.Reverse(ids)
			}
			want := []byte("from-B")
			connA, connB := newProgressConn(0), newProgressConn(0)
			connA.script = answer([]byte("from-A"))
			connB.script = answer(want)
			fx.pool.gets[testLocalA] = immediateGet(peerWithConn(testLocalA, connA))
			fx.pool.gets[testLocalB] = immediateGet(peerWithConn(testLocalB, connB))

			got := runBounded(t, time.Second, func() ([]byte, error) {
				return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
			})

			require.NoError(t, got.err)
			assert.Equal(t, want, got.data)
			assert.Equal(t, [][]string{{testLocalA, testLocalB}}, shuffled)
			assert.Empty(t, fx.pool.getCallsFor(testLocalA), "A comes second after the shuffle")
		})
	})
	t.Run("the default shuffle permutes", func(t *testing.T) {
		s := newStore(nil, nil)
		seen := map[string]bool{}
		for i := 0; i < 200 && len(seen) < 2; i++ {
			ids := []string{testLocalA, testLocalB}
			s.shuffleLocalPeers(ids)
			seen[ids[0]] = true
		}
		assert.Len(t, seen, 2, "both peers must come first sometimes")
	})
}

// ---- review round 3 ----

func (fx *localFixture) nodeRequests() []*fileproto.BlockGetRequest {
	fx.nodeMu.Lock()
	defer fx.nodeMu.Unlock()
	var res []*fileproto.BlockGetRequest
	for _, c := range fx.nodeConns {
		res = append(res, c.requestsSeen()...)
	}
	return res
}

func (fx *localFixture) setGet(id string, fn func(ctx context.Context) (peer.Peer, error)) {
	fx.pool.mu.Lock()
	defer fx.pool.mu.Unlock()
	fx.pool.gets[id] = fn
}

func TestLocalPeer_WaitWhenAvailable(t *testing.T) {
	// the downloader and the gateway ask the node to wait for a block it does
	// not have yet (ContextWithWaitAvailable); a local peer is never asked to
	// wait, it is only a shortcut
	t.Run("local success does not forward the wait flag", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fx := newLocalFixture(t, testLocalA)
			want := []byte("from-local")
			conn := newProgressConn(0)
			conn.allowWait = true // record the flag instead of rejecting it
			conn.script = answer(want)
			fx.pool.gets[testLocalA] = immediateGet(peerWithConn(testLocalA, conn))
			ctx := ContextWithWaitAvailable(fx.ctx())

			got := runBounded(t, time.Second, func() ([]byte, error) {
				b, err := fx.Get(ctx, testCid("a"))
				if err != nil {
					return nil, err
				}
				return b.RawData(), nil
			})

			require.NoError(t, got.err)
			assert.Equal(t, want, got.data)
			reqs := conn.requestsSeen()
			require.Len(t, reqs, 1)
			assert.False(t, reqs[0].Wait, "a local fetch must not ask the peer to wait")
			assert.Equal(t, 0, fx.pool.nodeCalls())
		})
	})
	nodeCases := []struct {
		name     string
		makeCtx  func(ctx context.Context) context.Context
		wantWait bool
	}{
		{name: "node fallback forwards the wait flag", makeCtx: ContextWithWaitAvailable, wantWait: true},
		{name: "node fallback without the flag does not wait", makeCtx: func(ctx context.Context) context.Context { return ctx }},
	}
	for _, tc := range nodeCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{"Get", "GetMany"} {
				t.Run(method, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						fx := newLocalFixture(t, testLocalA)
						fx.pool.gets[testLocalA] = func(context.Context) (peer.Peer, error) {
							return nil, errors.New("dial tcp: connection refused")
						}
						ctx := tc.makeCtx(fx.ctx())

						if method == "Get" {
							got := runBounded(t, time.Second, func() ([]byte, error) {
								b, err := fx.Get(ctx, testCid("a"))
								if err != nil {
									return nil, err
								}
								return b.RawData(), nil
							})
							require.NoError(t, got.err)
							assert.Equal(t, fx.nodeData, got.data)
						} else {
							got := collect(t, fx.GetMany(ctx, []cid.Cid{testCid("a"), testCid("b")}), time.Second)
							require.Len(t, got, 2)
						}

						reqs := fx.nodeRequests()
						require.NotEmpty(t, reqs)
						for _, r := range reqs {
							assert.Equal(t, tc.wantWait, r.Wait, "node request wait flag")
						}
					})
				})
			}
		})
	}
}

func TestLocalPeer_ConnectExemptionsKeepStrikeHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		errX := errors.New("x")
		// two strikes, both bans expired
		fx.strikeLocalPeer(testLocalA, "test", errX)
		time.Sleep(localPeerBanMin + time.Millisecond)
		fx.strikeLocalPeer(testLocalA, "test", errX)
		time.Sleep(2*localPeerBanMin + time.Millisecond)
		seeded, _ := fx.banOf(testLocalA)
		require.Equal(t, 2, seeded.strikes)
		require.Equal(t, []string{testLocalA}, fx.filterBannedPeers([]string{testLocalA}))

		acquireFails := func(acquire func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error)) func(ctx context.Context) (peer.Peer, error) {
			p := newFakePeer(testLocalA, nil)
			p.acquire = acquire(p)
			return immediateGet(p)
		}
		steps := []struct {
			name    string
			get     func(ctx context.Context) (peer.Peer, error)
			makeCtx func(ctx context.Context) (context.Context, context.CancelFunc)
			wantErr error
		}{
			{
				name: "caller cancel during dial",
				get:  blockingGet(nil),
				makeCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
					ctx, cancel := context.WithCancel(ctx)
					time.AfterFunc(time.Second, cancel)
					return ctx, cancel
				},
				wantErr: context.Canceled,
			},
			{
				name:    "pool closing",
				get:     func(context.Context) (peer.Peer, error) { return nil, ocache.ErrClosed },
				wantErr: ocache.ErrClosed,
			},
			{
				name:    "foreign shared-load Canceled",
				get:     func(context.Context) (peer.Peer, error) { return nil, context.Canceled },
				wantErr: context.Canceled,
			},
			{
				name:    "foreign shared-load DeadlineExceeded",
				get:     func(context.Context) (peer.Peer, error) { return nil, context.DeadlineExceeded },
				wantErr: context.DeadlineExceeded,
			},
			{
				name: "acquire foreign Canceled",
				get: acquireFails(func(*fakePeer) func(ctx context.Context) (drpc.Conn, error) {
					return func(context.Context) (drpc.Conn, error) { return nil, context.Canceled }
				}),
				wantErr: context.Canceled,
			},
			{
				name: "acquire foreign DeadlineExceeded",
				get: acquireFails(func(*fakePeer) func(ctx context.Context) (drpc.Conn, error) {
					return func(context.Context) (drpc.Conn, error) { return nil, context.DeadlineExceeded }
				}),
				wantErr: context.DeadlineExceeded,
			},
			{
				name: "acquire ErrConnClosed",
				get: acquireFails(func(*fakePeer) func(ctx context.Context) (drpc.Conn, error) {
					return func(context.Context) (drpc.Conn, error) { return nil, transport.ErrConnClosed }
				}),
				wantErr: transport.ErrConnClosed,
			},
			{
				name: "acquire on a peer closed locally",
				get: acquireFails(func(p *fakePeer) func(ctx context.Context) (drpc.Conn, error) {
					return func(context.Context) (drpc.Conn, error) {
						p.closed.Store(true)
						return nil, io.EOF
					}
				}),
				wantErr: io.EOF,
			},
		}
		for i, step := range steps {
			fx.setGet(testLocalA, step.get)
			ctx, cancel := fx.ctx(), context.CancelFunc(func() {})
			if step.makeCtx != nil {
				ctx, cancel = step.makeCtx(ctx)
			}
			got := runBounded(t, 7*time.Second, func() ([]byte, error) {
				return fx.getFromLocalPeers(ctx, testSpaceId, testCid(fmt.Sprint(i)))
			})
			cancel()

			require.ErrorIs(t, got.err, step.wantErr, step.name)
			b, ok := fx.banOf(testLocalA)
			require.True(t, ok, "%s: the strike history must survive", step.name)
			assert.Equal(t, seeded, b, "%s: a non-striking connect failure must not touch the strike history", step.name)
		}

		// the next genuine failure is the third strike: a 40 s ban
		fx.setGet(testLocalA, func(context.Context) (peer.Peer, error) {
			return nil, errors.New("dial tcp: connection refused")
		})
		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("genuine"))
		})
		require.ErrorIs(t, got.err, net.ErrUnableToConnect)
		b, _ := fx.banOf(testLocalA)
		want := localPeerBan{until: time.Now().Add(4 * localPeerBanMin), strikes: 3}
		assert.Equal(t, want, b)
	})
}
