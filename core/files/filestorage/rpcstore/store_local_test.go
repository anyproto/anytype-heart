package rpcstore

import (
	"context"
	"errors"
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
	return fn(ctx)
}

func (p *fakePool) Pick(ctx context.Context, id string) (peer.Peer, error) {
	p.mu.Lock()
	p.pickCalls = append(p.pickCalls, pickCall{id: id, ctxDone: ctx.Err() != nil})
	fn := p.picks[id]
	p.mu.Unlock()
	if fn == nil {
		return nil, ocache.ErrNotExists
	}
	return fn(ctx)
}

func (p *fakePool) GetOneOf(ctx context.Context, peerIds []string) (peer.Peer, error) {
	p.mu.Lock()
	p.getOneOfCalls++
	node := p.node
	p.mu.Unlock()
	if node == nil {
		return nil, net.ErrUnableToConnect
	}
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
	conn  drpc.Conn
	err   error
	cause error
}

type fakePeer struct {
	id      string
	acquire func(ctx context.Context) (drpc.Conn, error)

	mu       sync.Mutex
	acquires int
	releases []releaseRec
}

func newFakePeer(id string, acquire func(ctx context.Context) (drpc.Conn, error)) *fakePeer {
	return &fakePeer{id: id, acquire: acquire}
}

// peerWithConn is a peer that hands out the same conn on every acquire
func peerWithConn(id string, conn drpc.Conn) *fakePeer {
	return newFakePeer(id, func(context.Context) (drpc.Conn, error) { return conn, nil })
}

func (p *fakePeer) Id() string               { return p.id }
func (p *fakePeer) Context() context.Context { return context.Background() }
func (p *fakePeer) IsClosed() bool           { return false }
func (p *fakePeer) CloseChan() <-chan struct{} {
	return nil
}
func (p *fakePeer) SetTTL(time.Duration)                 {}
func (p *fakePeer) TryClose(time.Duration) (bool, error) { return false, nil }
func (p *fakePeer) Close() error                         { return nil }
func (p *fakePeer) AcquireDrpcConn(ctx context.Context) (drpc.Conn, error) {
	p.mu.Lock()
	p.acquires++
	p.mu.Unlock()
	return p.acquire(ctx)
}

func (p *fakePeer) ReleaseDrpcConn(ctx context.Context, conn drpc.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releases = append(p.releases, releaseRec{conn: conn, err: ctx.Err(), cause: context.Cause(ctx)})
}

func (p *fakePeer) DoDrpc(ctx context.Context, do func(conn drpc.Conn) error) error {
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return err
	}
	defer p.ReleaseDrpcConn(ctx, conn)
	return do(conn)
}

func (p *fakePeer) stats() (acquires int, releases []releaseRec) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acquires, slices.Clone(p.releases)
}

// blockScript decides what an Invoke of BlockGet does. It must block only on
// ctx, channels or timers.
type blockScript func(ctx context.Context) ([]byte, error)

// plainConn is a drpc.Conn without BytesRead
type plainConn struct {
	script blockScript

	invokes  atomic.Int32
	inflight atomic.Int32

	mu          sync.Mutex
	cancelledAt []time.Time // fake-time stamps of invokes that ended with a done ctx
}

func newPlainConn(script blockScript) *plainConn {
	return &plainConn{script: script}
}

func (c *plainConn) Invoke(ctx context.Context, _ string, _ drpc.Encoding, in, out drpc.Message) error {
	c.invokes.Add(1)
	c.inflight.Add(1)
	defer c.inflight.Add(-1)
	req := in.(*fileproto.BlockGetRequest)
	data, err := c.script(ctx)
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

func (c *plainConn) NewStream(context.Context, string, drpc.Encoding) (drpc.Stream, error) {
	return nil, errors.New("unexpected NewStream")
}
func (c *plainConn) Close() error            { return nil }
func (c *plainConn) Closed() <-chan struct{} { return nil }
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
	nodeConn  *plainConn
	nodeData  []byte
	// baseCtx is cancelled in t.Cleanup so an operation a failed test left
	// behind unwinds instead of tripping the bubble's deadlock detector
	baseCtx context.Context
}

func newLocalFixture(t *testing.T, localIds ...string) *localFixture {
	t.Helper()
	nodeData := []byte("from-node")
	nodeConn := newPlainConn(answer(nodeData))
	node := peerWithConn(testNodeId, nodeConn)
	pool := newFakePool(node)
	ps := &fakePeerStore{local: localIds, nodes: []string{testNodeId}}
	s := newStore(pool, ps)
	s.shuffleLocalPeers = func([]string) {} // deterministic candidate order
	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &localFixture{store: s, pool: pool, peerStore: ps, node: node, nodeConn: nodeConn, nodeData: nodeData, baseCtx: baseCtx}
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
		// the dial of A succeeds after 3 s; the acquire must then get only the
		// remaining 2 s, not a fresh 5 s
		fx.pool.gets[testLocalA] = func(ctx context.Context) (peer.Peer, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
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

func TestLocalPeer_ConnectAllFailedReportsUnableToConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		// no Get script: the fake pool answers net.ErrUnableToConnect

		got := runBounded(t, time.Second, func() ([]byte, error) {
			return fx.getFromLocalPeers(fx.ctx(), testSpaceId, testCid("a"))
		})

		require.ErrorIs(t, got.err, net.ErrUnableToConnect)
		fx.requireStruck(t, testLocalA, 1)
	})
}

// ---- ban backoff ----

func TestLocalPeer_BanBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newLocalFixture(t, testLocalA)
		err := errors.New("x")
		start := time.Now()

		fx.strikeLocalPeer(testLocalA, "test", err)
		b, _ := fx.banOf(testLocalA)
		assert.Equal(t, 1, b.strikes)
		assert.Equal(t, start.Add(10*time.Second), b.until)

		// a second strike while banned (another worker) doubles
		fx.strikeLocalPeer(testLocalA, "test", err)
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, 2, b.strikes)
		assert.Equal(t, start.Add(20*time.Second), b.until)
		assert.Empty(t, fx.filterBannedPeers([]string{testLocalA}))

		// expiry lets the peer through but keeps the strike count
		time.Sleep(20*time.Second + time.Millisecond)
		assert.Equal(t, []string{testLocalA}, fx.filterBannedPeers([]string{testLocalA}))
		b, ok := fx.banOf(testLocalA)
		require.True(t, ok, "expired entry must be kept for the backoff")
		assert.Equal(t, 2, b.strikes)
		fx.strikeLocalPeer(testLocalA, "test", err)
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, 3, b.strikes)
		assert.Equal(t, time.Now().Add(40*time.Second), b.until)

		// success clears everything
		fx.resetLocalPeer(testLocalA)
		fx.requireNotStruck(t, testLocalA)
		fx.strikeLocalPeer(testLocalA, "test", err)
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, 1, b.strikes)
		assert.Equal(t, time.Now().Add(10*time.Second), b.until)

		// localPeerBanMax of quiet after expiry also resets
		time.Sleep(10*time.Second + localPeerBanMax + time.Second)
		fx.strikeLocalPeer(testLocalA, "test", err)
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, 1, b.strikes)

		// the cap
		for i := 0; i < 10; i++ {
			fx.strikeLocalPeer(testLocalA, "test", err)
		}
		b, _ = fx.banOf(testLocalA)
		assert.Equal(t, 11, b.strikes)
		assert.Equal(t, time.Now().Add(localPeerBanMax), b.until)
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
				c.script = progressing(c, 2*time.Second, 500*time.Millisecond, 4096, want)
			}
			return c
		}
		local := newFakePeer(testLocalA, factory.acquire)
		fx.pool.gets[testLocalA] = immediateGet(local)
		var cids []cid.Cid
		for i := 0; i < 8; i++ {
			cids = append(cids, testCid(string(rune('a'+i))))
		}

		got := collect(t, fx.GetMany(fx.ctx(), cids), 15*time.Second)

		require.Len(t, got, 8)
		var cancelled, fromLocal int
		for _, c := range factory.all() {
			cancelled += len(c.cancellations())
		}
		for _, b := range got {
			if string(b.RawData()) == string(want) {
				fromLocal++
			}
		}
		assert.Equal(t, 1, cancelled, "only the silent conn is cancelled")
		assert.Equal(t, 7, fromLocal, "the progressing conns keep serving")
		assert.Equal(t, 1, fx.pool.nodeCalls(), "only the stalled block goes to the node")
		acquires, releases := local.stats()
		assert.Equal(t, acquires, len(releases))
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
			close(gate)

			// must not panic; the channel closes only once every worker returned
			for range ch {
			}
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
	synctest.Test(t, func(t *testing.T) {
		parent, parentCancel := context.WithCancel(context.Background())
		ctx, cancel := context.WithCancelCause(parent)
		defer cancel(nil)
		conn := newProgressConn(0)
		stop := watchFetchProgress(ctx, conn, cancel)
		parentCancel()
		synctest.Wait()
		stop() // must not block: the goroutine already left on ctx.Done
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
		assert.NotErrorIs(t, context.Cause(ctx), errLocalPeerStalled)
	})
}
