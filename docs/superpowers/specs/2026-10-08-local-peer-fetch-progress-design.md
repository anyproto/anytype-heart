# Local peer block fetch: bound the connect, cancel only stalled fetches

Ticket: GO-3192. Package: `core/files/filestorage/rpcstore`.

## Problem

`store.getFromLocalPeers` (`core/files/filestorage/rpcstore/store.go:217-236`) wraps the
peer dial **and** the block fetch in one `context.WithTimeout(ctx, localPeerTimeout)`
with `localPeerTimeout = time.Second` (`store.go:71`), and bans the peer for
`localPeerBanTTL = 5 * time.Minute` (`store.go:72`) when the fetch fails with
`net.ErrUnableToConnect`, `context.DeadlineExceeded`, `context.Canceled` or
`net.ErrClosed` (`store.go:230`). The timeout was introduced in 725ea9bdc (GO-6771)
with no recorded rationale.

Numbers:

- A file leaf block is up to `fileservice.ChunkSize = 1 << 20` = 1 MiB
  (any-sync `commonfile/fileservice/fileservice.go:20`).
- `GetMany` runs `getManyWorkers = 4` fetches concurrently (`store.go:70`,
  `store.go:275`), all over the same peer link.
- So up to 4 MiB must arrive within 1 s: about 34 Mbit/s sustained. A congested
  2.4 GHz Wi-Fi or a phone in Wi-Fi power-save often delivers 5-20 Mbit/s between two
  clients.
- On the first miss the peer is banned for 5 minutes and every block falls back to the
  file node. On a LAN-only or offline setup there is no node: the fetch fails.
- `context.Canceled` from the **caller** (a cancelled download, an expired
  `filedownloader` per-task timeout, `core/files/filedownloader/cache.go:50`) also bans
  the peer, which is wrong: it says nothing about the peer.

PR #3305 only makes the two numbers configurable. The fix is the semantics: bound only
the part that has a known upper bound (connecting), and cancel a fetch only when it
demonstrably makes no progress.

## Verified facts (any-sync v0.13.7 as pinned in go.mod, drpc v1.0.0, yamux v0.1.2)

Module cache paths: `$(go env GOMODCACHE)/github.com/anyproto/any-sync@v0.13.7`,
`$(go env GOMODCACHE)/storj.io/drpc@v1.0.0`,
`$(go env GOMODCACHE)/github.com/hashicorp/yamux@v0.1.2`.

Connecting:

- `pool.GetOneOf` (`net/pool/pool.go:633-662`) first returns any already-connected
  peer among the ids (`getIfActive`, non-blocking `fast` path over all ids first,
  `pool.go:599-631`), else dials the ids in random order with the caller's ctx and, if
  all fail, replaces the last error with `net.ErrUnableToConnect` unless it is a
  `handshake.HandshakeError` (`pool.go:656-659`). **A ctx error during the dial is also
  replaced**, and no peer id comes back, so the caller cannot tell which peer failed nor
  why.
- `pool.Get` (`pool.go:332-382`): fast path for a live peer, else an ocache load that
  dials. The load runs under the first caller's ctx; concurrent waiters retry up to
  `maxLoadRetries = 3` times if that load is aborted and otherwise receive the aborted
  load's error although their own ctx is alive (`app/ocache/ocache.go:178,218-235`).
  `pool.lookup` returns `ocache.ErrClosed` when the pool is closing or the pair was
  swapped under a cancelled caller (`pool.go:182-204`).
- `pool.Pick` (`pool.go:754-770`): `fast` path for a live peer; otherwise `lookup` →
  `ocache.Pick` → `entry.waitLoad(ctx)` which **waits for an in-flight load**
  (`ocache.go:239-252`, `app/ocache/entry.go:81-100`). `waitLoad` returns a completed
  load even under a done ctx and returns `ctx.Err()` at once for a still-loading entry.
  So `Pick` with a *pre-cancelled* ctx is a non-blocking "is it connected" probe, and
  `Pick` with a live ctx can block behind another subsystem's dial.
- The pool caches a dial verdict only for `handshake.ErrIncompatibleVersion`
  (`net/pool/poolservice.go:111-114`, 20 min). Every other dial failure is retried on
  the next `Get`. The rpcstore ban is therefore the **only** backoff for a stale mDNS
  peer.
- `peerService.Dial` (`net/peerservice/peerservice.go:143-275`) tries the peer's
  addresses sequentially, yamux first for local addresses (`orderAddrs`). Per address:
  TCP connect with `net.Dialer.Timeout = DialTimeoutSec` plus a secure handshake under
  another `DialTimeoutSec` (`net/transport/yamux/yamux.go:108-130`); QUIC with
  `HandshakeIdleTimeout = DialTimeoutSec` (`net/transport/quic/quic.go:63`). The heart
  sets `DialTimeoutSec: 10` for both (`core/anytype/config/config.go:670,684`). A
  blackholed address costs 10 s before the next address is tried; any outer bound below
  10 s never reaches the second address of such a peer.
- `peer.AcquireDrpcConn` (`net/peer/peer.go:269-344`) takes an idle sub-conn or opens
  a new one: `MultiConn.Open(ctx)` (yamux `Session.Open` in a ctx-honouring helper,
  `net/transport/yamux/conn.go:63-127`; QUIC `OpenStreamSync(ctx)`,
  `net/transport/quic/conn.go:120`), then the proto handshake under ctx
  (`peer.go:483-507`). Opening waits `(count-10) x 100 ms` once more than 10 sub-conns
  are open/opening/closing (`peer.go:293`, `net/peer/limiter.go:20`). A closed peer
  returns `transport.ErrConnClosed` (`peer.go:284-285`); handshake failures return raw
  handshake errors (`peer.go:491-493`).

Fetching:

- `peer.DoDrpc` (`peer.go:467-476`) = `AcquireDrpcConn(ctx)`, `do(conn)`,
  `ReleaseDrpcConn(ctx, conn)` with **one** ctx.
- `ReleaseDrpcConn` with `checkReleased` (`peer.go:370-465`): if the ctx passed to
  Release is done, the conn is never reused and is closed in the background
  (`closeAsync`, `peer.go:541-558`); if the conn is `doomed` (gc took it), it is
  dropped; otherwise it waits up to 200 ms for the drpc manager's `Unblocked` and
  re-pools the conn. A conn whose RPC was cancelled mid-flight is safe to release as
  long as the ctx handed to Release is the one that was cancelled.
- any-sync builds drpc conns without `SoftCancel` (`peer.go:496-501`), so a ctx cancel
  while a stream is unfinished **terminates the transport** of that sub-conn
  (`drpcmanager/manager.go:366-372`): the sub-stream dies, the peer connection does
  not. `Invoke` returns `context.Canceled`; `subConn.connLost` (`peer.go:192-207`)
  leaves it untouched because `ctx.Err() != nil`. The blocked `MsgRecv` is released
  promptly; a `RawWrite`/`CloseSend` blocked on a congested yamux send queue is bounded
  by `ConnectionWriteTimeout = WriteTimeoutSec = 10 s` (`yamux.go:69`), not by the ctx,
  so "cancelled" and "returned" can be up to 10 s apart in the worst case.
- The conn returned by `AcquireDrpcConn` is `*peer.subConn` (`peer.go:103-116`), which
  embeds `encoding.ConnUnblocked` (`drpc.Conn` + `Unblocked()`,
  `net/rpc/encoding/connection.go:40-43`) and `*connutil.LastUsageConn`
  (`net/connutil/usage.go:13-18`). `BytesRead()` exists only on `LastUsageConn` at
  depth 1, so it is promoted: `conn.(interface{ BytesRead() int64 })` holds for every
  production sub-conn. (`Close` is unambiguous: depth 1 via `ConnUnblocked`, depth 2 via
  `LastUsageConn.net.Conn`.)
- `LastUsageConn.Read` (`usage.go:29-36`) adds `n` to `bytesRead` after every
  underlying `Read` that returned bytes; `lastUsageUnixNano` is stamped **on entry**,
  before the read blocks, so `LastUsage()` must not be used for progress.
- The `LastUsageConn` wraps the **sub-stream** (`peer.go:488`): the counter is per
  sub-conn, cumulative across the handshake and every RPC that reused the sub-conn, and
  one sub-conn carries one stream at a time. A baseline taken at the start of the fetch
  makes it usable.
- drpc's reader (`drpcwire/reader.go:62-74,100-114`) calls the stream's `Read` into a
  buffer that grows from 4 KiB, appending frames until the message is complete; the
  manager's `manageReader` goroutine does this continuously
  (`drpcmanager/manager.go:214`), independent of the caller's `MsgRecv`. The sender
  splits a message into frames of at most 64 KiB (`drpcwire/split.go:38`) and writes
  each frame as one `Write` (`drpcwire/writer.go:58-70`).
- **Granularity differs per transport.** QUIC `Read` returns whatever bytes have
  arrived (`quic-go/receive_stream.go`), so the counter advances in small steps. yamux
  `readData` copies a whole yamux data frame into `recvBuf` under `recvLock` before
  `Read` can see any of it (`yamux/stream.go:478-510`, `stream.go:107-132`), and the
  yamux frame is the drpc 64 KiB frame (bounded by the 256 KiB window). So on yamux the
  counter advances in 64 KiB steps: a stall window of 5 s is a throughput floor of about
  64 KiB / 5 s ≈ 105 kbit/s. Below that, bytes arrive but are not counted before the
  window expires. A `Read` also sends a window update before returning
  (`stream.go:132`), which on a saturated send queue can block up to the write timeout.
- Snappy is **not** negotiated between heart peers: `Config.GetDrpc()` leaves
  `rpc.Config.Snappy` false (`core/anytype/config/config.go:581-586`), and the server
  then uses the no-Snappy checker (`peer.go:609-612`). Irrelevant to the counter either
  way: it counts wire bytes.
- Transport-side protection that exists: per-write deadlines of `WriteTimeoutSec = 10`
  on the sender (`connutil.TimeoutConn`, `net/connutil/timeout.go`), yamux
  `ConnectionWriteTimeout`/`StreamCloseTimeout` = 10 s, keepalives every 10 s on both
  transports (`config.go:669-689`), `pool.Flush` on wake/network change
  (`core/device/recoveryworker.go:329`), which closes every pooled peer and so turns an
  in-flight fetch into `transport.ErrConnClosed`. There is **no read-side per-stream
  progress detection** anywhere in `net/` (grep "stall" finds comments only); the
  cleanup stall-escalation of 2aab1b5a was removed in 950bebaf (verified by diffing
  `net/peer/cleanup_test.go` between the two pseudo-versions in the module cache).

Current rpcstore behaviour worth recording:

- A **dial** failure never bans today: `p` is nil on the `GetOneOf` error path
  (`store.go:224-227`). Each `Get` re-pays the (1 s) dial attempt.
- `getFromNodePeer` (`store.go:238-244`) uses the caller's ctx and `Wait: true` when
  requested. Untouched by this change.
- `GetMany` (`store.go:269-311`) returns from the dispatcher without `wg.Wait()` when
  the ctx ends while it waits for a worker slot, then `close(resultCh)` runs while a
  worker may still be in `select { case <-ctx.Done(): case resultCh <- b: }`; with both
  cases ready the select may pick the send and panic. Pre-existing; fixed here because
  the cancellation paths are what this change is about.

## Design

### Constants

```go
const getManyWorkers = 4

// var (not const) so the real-transport integration test can shorten them.
var (
	// localPeerConnectTimeout bounds one attempt to get a usable sub-conn to one
	// local peer: pool lookup/dial (TCP or QUIC + secure handshake) plus sub-conn
	// open + proto handshake. One budget, not one per stage.
	localPeerConnectTimeout = 5 * time.Second
	// localPeerStallTimeout: a fetch is cancelled when the sub-conn's BytesRead
	// has not advanced for this long.
	localPeerStallTimeout = 5 * time.Second
	// localPeerStallCheckInterval is the watchdog tick.
	localPeerStallCheckInterval = time.Second
	// localPeerFetchFallbackTimeout bounds a fetch on a conn without BytesRead
	// (tests, foreign peer implementations): 1 MiB at ~300 kbit/s.
	localPeerFetchFallbackTimeout = 30 * time.Second
	// localPeerBanMin is the first ban; each consecutive strike doubles it up to
	// localPeerBanMax. A successful fetch, or localPeerBanMax of quiet after a
	// ban expired, resets the strike count.
	localPeerBanMin = 10 * time.Second
	localPeerBanMax = 5 * time.Minute
)
```

Why one 5 s connect budget: on a LAN a healthy connect completes in well under a
second; the bound only decides how long a *stale* mDNS entry costs. Today that cost is
1 s per block but the peer is never banned, so every block pays it. With 5 s and a ban
the cost is paid once per ban period. 5 s leaves headroom for a phone whose Wi-Fi radio
has to leave power-save (hundreds of ms per round trip under load) and for the open
limiter. It is below the 10 s per-address transport timeout, which means a peer whose
*first* address is blackholed while a later one works cannot be connected from this
path; for mDNS peers every address is the same host, so that case means the host is
gone. Other subsystems dial with longer budgets and the connected peer is then picked
up by the non-blocking scan.

Why 5 s for the stall window: the peer answers `BlockGet` from its local flat-file store
(`core/files/filestorage/rpchandler.go:63-77`, `flatstore.go:51`), typically
milliseconds. 5 s without a single byte on a LAN means the peer is suspended, the link
is dead, or the yamux throughput is below ~105 kbit/s; in all three the node is the
better source. The watchdog is a policy ("minimum useful progress"), not a proof that
the peer is broken; the ban backoff below keeps the cost of a misfire at 10 s.

Why exponential bans: dial failures become bannable with this change, and a 5-minute
first ban would turn a phone that was briefly asleep into 5 minutes of node-only (or,
LAN-only, failed) downloads. 10 s → 20 s → ... → 5 min keeps a flapping peer from
costing more than one connect budget per ban period while a peer that comes back is
retried within seconds.

### Ban state

```go
type localPeerBan struct {
	until   time.Time
	strikes int
}
bannedLocalMap map[string]localPeerBan
```

- `strikeLocalPeer(id, reason)`: if `now > until + localPeerBanMax` reset `strikes`;
  `strikes++`; `until = now + min(localPeerBanMin << (strikes-1), localPeerBanMax)`;
  one `log.Info` per strike with peerId, phase, reason, strikes, ban duration.
- `filterBannedPeers`: skip ids whose `until` is in the future; expired entries are
  kept (the strike count survives expiry).
- `resetLocalPeer(id)`: delete the entry after a successful fetch.

### Context structure

```
ctx (caller)
├─ probeCtx  = WithCancel(ctx), cancelled before use   // non-blocking Pick scan
├─ connectCtx = WithTimeout(ctx, localPeerConnectTimeout)  // per candidate: pool.Get + AcquireDrpcConn
└─ fetchCtx  = WithCancelCause(ctx)                     // BlockGet + ReleaseDrpcConn; cancelled by the
                                                        // watchdog with errLocalPeerStalled
```

### Connect phase: `connectLocalPeer(ctx, ids) (peer.Peer, drpc.Conn, error)`

1. Shuffle `ids` (parity with `GetOneOf`; no discovery-order bias).
2. Non-blocking scan: `probeCtx, c := WithCancel(ctx); c()`; for each id
   `pool.Pick(probeCtx, id)`; the first live peer is moved to the front of the
   candidate list. This relies on `pool.fast` ignoring ctx and `entry.waitLoad`
   returning at once under a done ctx (verified above); it is the only way to get
   `getIfActive`'s "all fast paths before any wait" behaviour through the public API.
3. For each candidate id: `connectCtx` (5 s); `p, err := pool.Get(connectCtx, id)`
   (fast path for the picked one); then `conn, err := p.AcquireDrpcConn(connectCtx)`;
   cancel `connectCtx` (does not affect the conn). Success → return.
4. Failure classification (only with the caller's `ctx.Err() == nil`; otherwise return
   `ctx.Err()` at once, no strike):

   | connect-phase error                                          | strike | continue to next id |
   |--------------------------------------------------------------|--------|---------------------|
   | `ocache.ErrClosed` (pool closing / pair swapped)             | no     | no, return error    |
   | `connectCtx` deadline hit (slow dial, limiter wait, slow open) | yes  | yes                 |
   | `context.Canceled` with `connectCtx` alive (another caller's aborted shared load, `ocache.go:231`) | no | yes |
   | `transport.ErrConnClosed` from Acquire (peer closed under us: Flush, gc) | no | yes |
   | anything else (dial refused/unreachable, handshake error, open error) | yes | yes |

5. All candidates failed → `fmt.Errorf("connect local peer: %w", net.ErrUnableToConnect)`
   (plus the last error, joined).

### Fetch phase: `fetchBlockFromLocalPeer(ctx, p, conn, spaceId, k)`

```go
fetchCtx, cancel := context.WithCancelCause(ctx)
defer cancel(nil)
defer p.ReleaseDrpcConn(fetchCtx, conn)   // 2nd: after the watchdog has stopped
stop := watchFetchProgress(fetchCtx, conn, cancel)
defer stop()                              // 1st (LIFO): joins the watchdog goroutine
resp, err := fileproto.NewDRPCFileClient(conn).BlockGet(fetchCtx, req)
stop()                                    // join before reading the cause
if err != nil {
	if cause := context.Cause(fetchCtx); errors.Is(cause, errLocalPeerStalled) {
		return nil, fmt.Errorf("local peer block get: %w", errLocalPeerStalled)
	}
	err = rpcerr.Unwrap(err)
	if errors.Is(err, fileprotoerr.ErrCIDNotFound) { return nil, format.ErrNotFound{Cid: k} }
	return nil, fmt.Errorf("local peer block get: %w", err)
}
return resp.Data, nil
```

Why Release gets `fetchCtx`: `checkReleased` reads `ctx.Done()` to decide "never reuse,
close in the background". `fetchCtx` is done exactly when the RPC was cut short
(watchdog or caller), which is when drpc has terminated the sub-conn's transport.
`connectCtx` would be wrong: it is cancelled right after Acquire on the success path
and would make every successful fetch discard its conn. `stop()` **joins** the watchdog
goroutine before Release runs, so a tick that was in flight cannot cancel `fetchCtx`
between a successful RPC and the Release check.

Fetch-phase classification (only with `ctx.Err() == nil`): strike if the error matches
`errLocalPeerStalled`, `net.ErrClosed` (covers `transport.ErrConnClosed`, which unwraps
to it, `net/transport/transport.go:26`), `context.Canceled`, `context.DeadlineExceeded`
(drpc's stand-ins for the sub-conn ending under us, `peer.go:183-191`) or `io.EOF`
(unary RPC closed without a response). No strike for `ErrCIDNotFound` or any other
application error: the peer answered. Success resets the peer's strikes.

### Watchdog mechanics

```go
var errLocalPeerStalled = errors.New("local peer fetch stalled: no bytes received")

type bytesReader interface{ BytesRead() int64 }

// watchFetchProgress cancels ctx with errLocalPeerStalled when conn stops
// receiving bytes. stop joins the goroutine; safe to call more than once.
func watchFetchProgress(ctx context.Context, conn drpc.Conn, cancel context.CancelCauseFunc) (stop func()) {
	done, exited := make(chan struct{}), make(chan struct{})
	br, hasCounter := conn.(bytesReader)
	go func() {
		defer close(exited)
		if !hasCounter {
			t := time.NewTimer(localPeerFetchFallbackTimeout)
			defer t.Stop()
			select {
			case <-done:
			case <-ctx.Done():
			case <-t.C:
				cancel(errLocalPeerStalled)
			}
			return
		}
		ticker := time.NewTicker(localPeerStallCheckInterval)
		defer ticker.Stop()
		last, lastProgress := br.BytesRead(), time.Now()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if cur := br.BytesRead(); cur != last {
					last, lastProgress = cur, now
					continue
				}
				if now.Sub(lastProgress) >= localPeerStallTimeout {
					cancel(errLocalPeerStalled)
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		<-exited
	}
}
```

Lifetime: one goroutine per fetch; it exits on `stop()`, on `ctx.Done()` (caller or
itself), and `stop()` blocks until it has exited, so after `stop()` returns nothing can
cancel `fetchCtx` any more. No shared mutable state: `last`/`lastProgress` are
goroutine-local, `BytesRead` is an atomic load, `cancel` is idempotent.

Granularity: a stall is detected between `localPeerStallTimeout` and
`localPeerStallTimeout + localPeerStallCheckInterval` after the last counted byte (on
yamux: after the last complete 64 KiB frame).

### `getFromLocalPeers`

```go
ids := s.filterBannedPeers(s.peerStore.LocalPeerIds(spaceId))
if len(ids) == 0 { return nil, errNoLocalPeers }
p, conn, err := s.connectLocalPeer(ctx, ids)      // strikes inside, per candidate
if err != nil { return nil, err }
data, err := s.fetchBlockFromLocalPeer(ctx, p, conn, spaceId, k)
if err != nil {
	if ctx.Err() == nil && isLocalPeerFetchFailure(err) { s.strikeLocalPeer(p.Id(), "fetch", err) }
	return nil, err
}
s.resetLocalPeer(p.Id())
return data, nil
```

`Get`/`GetMany` keep "local first, then node"; when the local attempt fails and the node
is tried, the local error is logged at debug level with the cid so a slow open can be
explained from logs. The returned error stays the node's (callers match on it).

### `GetMany` fix

The dispatcher's deferred function becomes `wg.Wait(); close(resultCh)` on every exit
path, so no worker can send on a closed channel.

### Error wrapping

All new error returns use `fmt.Errorf("<operation>: %w", err)`; the sentinel stays
reachable through `%w`.

## Not changed

- `getFromNodePeer`, `getBlock` for the node, `doNodeDrpc`, `doNodeReserved`,
  `reservedCallTimeout`, `getManyWorkers = 4`, local-then-node order.
- No any-sync change. A generic `peer.DoDrpcWithProgress` for other bulk RPCs
  (`BlockPush`, `BlockPushMany`, sync payloads) would move the watchdog next to
  `DoDrpc` and drop the type assertion; out of scope, noted as a follow-up.
- No node race: the node is still tried only after the local attempt fails. Starting a
  competing node fetch after ~1 s would hide slow peers at the cost of duplicate traffic
  and battery; it is a product decision outside this ticket (see disposition).
- PR #3305's config knobs are not adopted.

## Test plan

Fixtures (hand-written, in `store_local_test.go`): `fakePool` (`Pick`/`Get`/`GetOneOf`
scripted per id; records every `Get` call with the ctx deadline it saw, and whether
`Pick` was called with a done ctx), `fakePeer` (`AcquireDrpcConn` returns the next
scripted conn or error; `ReleaseDrpcConn` snapshots `ctx.Err()`, `context.Cause(ctx)`
and the conn identity **at release time**), `fakePeerStore`, and two conns:
`progressConn` (`drpc.Conn` + `BytesRead`, atomic counter, nonzero baseline) and
`plainConn` (`drpc.Conn` only). `Invoke` fills `*out.(*fileproto.BlockGetResponse)` and
blocks only on channels/timers/ctx, so the whole fixture is bubble-safe. Unit tests run
under `testing/synctest` with the production constants; the operation under test runs in
a goroutine with a result channel and the test selects against an independent fake-time
limit (7 s for counter conns, 31 s for plain conns), so a hang fails with a message
instead of relying on bubble deadlock detection.

For every test: how the fixture fails if the implementation is wrong.

1. **progressing fetch (40 s, +64 KiB every 500 ms, baseline 1234) completes** —
   expect data, no strike, Release saw `Err() == nil`, zero node calls. Fails if the
   watchdog ignores progress (cancel at ~5 s), if the fallback cap is applied although
   `BytesRead` exists (cancel at 30 s < 40 s), or if Release gets a done ctx.
2. **progress then silence** — advances for 3 s, then stops; Invoke blocks until ctx.
   Expect the Invoke ctx cancelled at 8-9 s of fake time (5-6 s after the last
   increment), cause `errLocalPeerStalled`, strike, node serves the block. Fails if the
   watchdog resets on the first byte only, never fires, or fires on the wall clock.
3. **silent fetch** — never advances. Expect cancellation in [5 s, 6 s], cause
   `errLocalPeerStalled`, strike (`filterBannedPeers` drops the id), Release saw a done
   ctx with that cause, node fallback succeeds, the next `Get` within the ban does not
   call the local pool at all. Fails if the window is wrong (upper bound rejects the
   30 s cap), the cause is missing, or the ban is skipped.
4. **plain conn uses the fallback cap** — expect cancellation at [30 s, 30 s + tick],
   cause stall, strike. Fails if cancelled at 5 s (missing counter treated as no
   progress) or never.
5. **caller cancel / caller deadline during fetch** — two cases; Invoke blocks. Expect
   the error wraps `context.Canceled` / `DeadlineExceeded`, **no** strike, Release saw
   the done ctx. Fails if the `ctx.Err() == nil` guard is missing.
6. **connect: dial timeout strikes only the slow peer** — `Pick` misses for A and B,
   A's `Get` blocks until its ctx ends, B's connects. Expect the block from B, A's `Get`
   ctx had a deadline of `localPeerConnectTimeout`, `filterBannedPeers([A,B]) == [B]`,
   first ban of A is 10 s. Fails if the dial is unbounded (hits the test limit), if the
   whole list is banned, or if A is not struck.
7. **connect: non-blocking scan prefers a connected peer** — A's `Pick` honours ctx and
   would block (simulating a load in flight), B's `Pick` returns a peer. Expect B used
   with zero fake time elapsed and A's `Pick` called with an already-done ctx. Fails if
   the scan uses a live ctx (A blocks → time advances / test limit) or skips the scan.
8. **connect: shared-load abort does not strike** — A's `Get` returns
   `context.Canceled` immediately (its ctx alive). Expect A not struck, B used.
9. **connect: `ocache.ErrClosed` returns at once without striking.**
10. **connect: immediate dial error strikes** — A's `Get` returns a transport error at
    once. Expect A struck, B used.
11. **connect: acquire timeout strikes, zero releases** — `AcquireDrpcConn` blocks until
    ctx. Expect strike, deadline on the acquire ctx within the same 5 s budget as the
    dial (the test's `Get` consumes 3 s first, so the acquire ctx must expire at 5 s,
    not 8 s), no Release call. Fails if two budgets are used or the acquire is unbounded.
12. **connect: `ErrConnClosed` from acquire does not strike** and the next candidate is
    used.
13. **connect: caller deadline during dial** — `Get` blocks, caller ctx 1 s. Expect
    `DeadlineExceeded`, no strike. Fails if the caller's ctx is not checked (the error
    would be classified as a slow dial).
14. **fetch: `io.EOF` and `net.ErrClosed` strike; a coded application error does
    not** — the coded error is built with `rpcerr`'s registered code for
    `ErrCIDNotFound` so removing `rpcerr.Unwrap` fails the `format.ErrNotFound` match.
15. **ban backoff** — two strikes → 10 s then 20 s; expiry keeps the strike count (third
    strike after expiry → 40 s); success resets; `localPeerBanMax` of quiet after expiry
    resets; cap at 5 min after 6 strikes.
16. **GetMany: four workers, four conns** — 8 cids, 4 distinct silent conns, node
    fine. After `synctest.Wait()` exactly 4 local Invokes are in flight; after the stall
    all 8 blocks arrive, the channel closes, exactly 4 local Invokes happened in total
    (the ban is visible to the remaining workers), acquires == releases.
17. **GetMany: mixed** — one conn silent, three progressing. Only the silent one is
    cancelled; all 8 blocks arrive. Fails if progress is shared across conns.
18. **GetMany: cancel while a worker holds a block** — 6 cids, the first worker's send
    is gated; cancel the ctx while the dispatcher waits for a slot; release the worker.
    Expect no panic and the channel closed only after all workers returned. Fails on the
    pre-existing close-before-wait bug (run with `-race`).
19. **integration (real drpc over `net.Pipe`, real time, shortened vars)** — the
    existing `rpctest` fixture plus `UpdateLocalPeer("local", [space])`; the `BlockGet`
    hook blocks the first call under `ctx.Done()` (hook read under the mutex, invoked
    after unlocking). Assert through a wrapper peer that the conn returned by the real
    `AcquireDrpcConn` satisfies `bytesReader` (fails the day any-sync stops promoting
    `BytesRead`), that `getFromLocalPeers` returns `errLocalPeerStalled` within a
    real-time bound, that the peer is struck, and that `Get` then succeeds via the node
    (the second `BlockGet` call is not blocked). Peers are closed in cleanup. Kept
    outside synctest (real goroutines, real timers).

Mutation checks before committing: watchdog never cancels (3, 4, 19 fail); drop the
`ctx.Err() == nil` guard (5, 13 fail); strike the whole list on dial failure (6 fails);
release with `connectCtx` (1 fails); two separate connect budgets (11 fails); stop
polling after first progress (2 fails); share one conn across workers (16/17 fail);
remove `wg.Wait` (18 fails under `-race`).

## Risks and open questions

- **yamux granularity.** Progress is visible per 64 KiB frame; the stall window is
  effectively a ~105 kbit/s floor on yamux. QUIC has byte granularity. If field data
  shows healthy-but-slow LAN peers being cut, raise `localPeerStallTimeout` (the ban
  backoff bounds the damage meanwhile).
- **Blackholed first address.** A peer reachable only through its second address is
  never connected from this path (10 s per-address transport timeout > 5 s budget); it
  is used once another subsystem connects it. Accepted; mDNS peers share one host.
- **Flush during a fetch.** `pool.Flush` on wake closes the peer under the RPC →
  `ErrConnClosed` → strike → 10 s ban. Accepted: one short ban per recovery.
- **Fallback-to-node latency.** First block after a ban expiry: up to 5 s connect + 5-6 s
  stall before the node is tried (today: 1 s). Each `GetMany` worker pays it once per
  ban period. With the backoff a dead peer settles at one such episode per 5 min.
- **Cancel ≠ return.** After the watchdog cancels, `Invoke` normally returns at once,
  but a request write blocked on a congested yamux send queue can hold it up to 10 s.
- **ocache single-flight abort.** A 5 s connect budget cancels a shared load; other
  waiters retry up to 3 times. No worse than the 1 s bound today.
- **Open:** should a stall count as a QUIC-demotion strike? The pool has no API for it;
  out of scope.

## Review disposition

Four independent Codex (gpt-6-astra, xhigh) reviews of the first draft, each verified
against the code before acting.

Accepted:

- **`pool.Pick` with the caller ctx blocks behind another subsystem's dial** (lenses 1,
  2, 3; P0). Verified at `pool.go:754-770`, `ocache.go:239-252`, `entry.go:81-100`.
  Fixed by the pre-cancelled probe ctx (non-blocking by construction) and by giving the
  picked peer the same bounded connect path as a dialed one.
- **Two separate 5 s budgets (dial, then acquire) make ~10 s before the fetch starts**
  (lenses 1, 2, 3; P1). Fixed: one `connectCtx` per candidate covers both.
- **A 5-minute ban on a first dial failure is a regression for LAN-only setups**
  (lens 3, P0; lens 2's blackholed-address case). Fixed by the 10 s → 5 min backoff
  with reset on success. The blackholed-first-address case itself is accepted as a
  limitation (needs a ≥ 10 s budget to reach the second address).
- **`stop()` must join the watchdog, otherwise a tick in flight can cancel `fetchCtx`
  after a successful RPC and make Release discard a healthy conn** (lenses 1, 2, 4).
  Fixed: `stop()` waits on `exited`; the fallback timer lives in the same goroutine.
- **Connect-phase errors need their own classification** (lenses 1, 2, 4):
  `ocache.ErrClosed`, another caller's aborted shared load (`Canceled` with our ctx
  alive), `ErrConnClosed` from a locally closed peer, and raw handshake errors from
  `AcquireDrpcConn` were either over- or under-banned by the sentinel list. Fixed by the
  connect-phase table; `io.EOF` added to the fetch-phase list.
- **yamux frame granularity** (lens 2, P1). Verified at `yamux/stream.go:478-510`.
  Documented as a throughput floor; constants unchanged.
- **Heart does not enable Snappy** (lens 2). Verified at `config.go:581-586`; spec
  corrected.
- **`GetMany` close-before-wait panic on cancel** (lens 1, P1). Verified at
  `store.go:271-309`; fixed with a one-line change and test 18.
- **Test plan** (lens 4, all P1s): bounded harnesses with fake-time limits, 40 s
  progressing fetch (beyond the 30 s cap), "progress then silence", nonzero baseline,
  Release-time snapshots, acquisition-failure rows, coded application error, four
  distinct conns for `GetMany`, mixed stall case, `-race`.
- **Diagnosability** (lens 3): one structured log per strike (phase, reason, strikes,
  duration) and a debug log of the local error on fallback.
- Wording fixes: "5 s is a quarter of today's best case" (today's bound is 1 s), "the
  counter only sees this RPC's bytes" (it is cumulative; baseline taken), "live Release
  ctx proves re-pooling" (it proves the conn was offered for reuse).

Rejected:

- **Race a node fetch after ~1 s while the local fetch progresses** (lens 3). Doubles
  traffic and battery on every slow block and changes the product rule "local first";
  the user's direction is dial bound + progress watchdog. Noted under "Not changed".
- **Separate first-byte (10-15 s) and inter-byte windows** (lens 3). The serving side
  reads a flat file; 5 s without a byte on a LAN is already generous, and the backoff
  caps a misfire at 10 s. One window keeps the mechanism explainable.
- **Bounded reacquisition after Flush closes the selected peer** (lenses 1, 2). The
  connect phase already skips `ErrConnClosed` without a strike and moves on; a fetch
  cut by Flush costs one 10 s ban. Not worth a retry loop inside the fetch path.
- **Use gomock `mock_pool`/`mock_peer` instead of hand-written fakes** (lens 4, P2).
  The fakes need per-call ctx-deadline capture, Release-time snapshots and scripted
  blocking; hand-written is shorter and clearer here.
- **Real-transport throttled 1 MiB tests with Snappy on/off** (lens 2). Heart does not
  negotiate Snappy, and the `rpctest` fixture is `net.Pipe` without flow control, so a
  throttled transport test would test the fake, not yamux. The yamux behaviour is
  documented from source instead.
- **Run the integration test inside synctest** was never planned; lens 4's yamux
  timer-pool argument does not apply (`rpctest` is `net.Pipe`, no yamux) but the test
  stays real-time anyway.
