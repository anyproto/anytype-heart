# Design: LAN peer unreachable for 20–60 s after an interface loss

Date: 2026-10-12. Ticket GO-7583. Follows an investigation handoff (not in the repository).
Code: develop at a1d14db00 and any-sync v0.13.7.
Implemented on branch `go-7583-lan-peer-pushes-after-iface-loss`. A prototype patch preceded it;
the implementation departs from it where §7 says, and the sections below describe what was built.

## 1. Root cause (verified against the code and the dumps)

The handoff's mechanism is right in substance. Precisions and corrections are marked **[corr]**.

1. **Recovery flush empties every list.** `netmonitor.checkInterfaces` fires `triggerInterfaceLost`
   on the 5 s tick after en0 lost its address (14:05:28.065) → `recover()` → `pool.Flush` swaps the
   cache pair and cancels the old one (the 14:05:28.065 `rpc ended by the sub conn closing: context
   canceled` lines are that cancel, including the healthy cable streams to the iPhone) →
   `runConnectivityHooks` → `provider.onConnectivityChange` → `signalRebuild` on all 83 managers →
   `SyncAllSpaceHeads`. `GetResponsiblePeers` filters `IsClosed()` peers, so every space's served list is
   empty from here until its rebuild publishes.

2. **The rebuild serialises the local peers behind the node walk.** `fetchResponsiblePeers`
   (`manager.go:342`) calls `pool.GetOneOf(n.ctx, nodeIds)` first, unbounded by design (GO-7409/7410),
   and runs the local `pool.Get` loop only after it returns. The node is published early when the walk
   succeeds; when it fails the local peers wait for the whole walk.

3. **Offline, the walk is long and serialised across spaces. [corr]** Node addresses are hostnames
   (`quic://prod-any-sync-nodeN.anyclub.org:5430`). `quicTransport.Dial` resolves with
   `net.ResolveUDPAddr` (no ctx → cgo `getaddrinfo`, not cancellable), and with only utun4/bridge100/
   en5-link-local left the resolver has no working upstream, so each attempt hangs to its own timeout.
   The dumps show that of the 83 `fetchResponsiblePeers` goroutines only 11–13 are dialling (in
   `lookupIPAddr`); 41 wait in `GetOneOf → getIfActive → ocache.Pick → waitLoad` and 29 in
   `ocache.Get → waitLoad`, i.e. parked on another manager's single-flight dial of the same node (the
   known "ocache.Pick blocks on a loading entry" behaviour, GO-7333/D6). The handoff's "36→66 goroutines
   in DNS lookup" overcounts: real resolver goroutines (`singleflight.doCall`) are 7→16, the rest are
   waiters. Consequence: managers finish their walks in **waves** when a shared dial fails — the log has
   `can't get node peers` in batches at 14:05:48 (3), 14:05:58 (6), 14:06:28 (9), 14:06:38 (5) — and each
   manager publishes its local peers only when *its own* walk ends. That spread is the "20–60 s".

4. **The iPhone was servable the whole time. [confirmed with evidence]** It reconnected inbound at
   14:05:28.3 (`updated local peer … [fe80::…%28]:62654`, 77 shared spaces). `pool.Get` serves an incoming
   connection from the fast path without dialling (`pool.go` `fast`/`getIncoming`). Proof in the log: at
   14:05:48.12, immediately after the first `can't get node peers`, the first managers' local loops
   logged `failed to get local from net pool` for the two Wi-Fi-era peers (`12D3KooWH8YP…`
   192.168.103.245 and `12D3KooWFMJe…` 192.168.103.144, `network is unreachable`) but **not** for the
   iPhone: `Get(iPhone)` succeeded instantly — 20 s after it had become available.

5. **The parked send workers are a symptom, not a multiplier. [corr]** All 4 `DialQueueWorkers` are
   parked in `waitResponsiblePeers` in every dump, but the broadcast deadline is absolute from enqueue
   (`BroadcastMessage` stamps `now+30s` before `streamPool.Send`), so a broadcast queued behind parked
   ones expires at its own enqueue+30 s regardless; parking does not compound beyond 30 s. The 48
   `peer find deadline exceeded` drops (14:05:59→14:06:34) are broadcasts enqueued 14:05:29→14:06:04 whose
   space had no live peer within 30 s. Also, `KeepAlive` — the only periodic broadcast — runs only at the
   end of a *successful* `diffSyncer.Sync`, so spaces without peers do not generate parked broadcasts on
   their own; only edits in such a space do. The send path is therefore not the first thing to fix.

6. The 92/89/85 head-sync waiters are the post-flush sweep (`runSyncAllSpaceHeads`, 15 s peer-find
   deadline, 10 workers) and the per-space periodic rounds (`periodicsync` timeout 1 min): the
   `periodic call error: context deadline exceeded` burst at 14:06:28 is exactly 60 s after the flush, for
   spaces still peerless then. Consistent with 3.

**Root cause in one sentence:** after a flush, each space's local (LAN) peers are published only after
that space's unbounded node walk ends, and with the internet gone the walk takes 20–70 s per space
(DNS + cross-space single-flight), while the reachable LAN peer's live connection sits in the pool the
whole time.

A latent defect found on the way: closing a manager mid-fetch (space close) makes every pool call fail
with the manager's own `context.Canceled`, and the local loop then `RemoveLocalPeer`s every LAN peer of
that space from the **global** store. The fix below guards it (and pins it with a test).

## 2. Recommended fix (heart only, `space/spacecore/peermanager`)

### 2.1 Publish the two sources independently (the fix)

Split `fetchResponsiblePeers` into a node lookup that runs on its own goroutine and a local pass that
runs on the manage loop; each publishes the moment it has a result. The served list is composed from
the two latest results. No ordering between them, so neither waits for the other:

- interface loss with a surviving LAN path (this bug): the LAN peer is served in ≤ a few ms (existing
  inbound connection) or ≤ 2 s (cable dial), while the node walk fails in the background;
- switch to cellular with stale Wi-Fi-era mDNS entries (GO-7379): the node is served the moment its
  dial lands, while the stale entries burn their 2 s budgets on the loop — same guarantee as today's
  "node first", without the reverse dependency.

Changed functions (all in `manager.go`):

```
type clientPeerManager struct {
    ...
    nodePeer   peer.Peer     // latest node verdict, nil after a failed lookup   (under n.Mutex)
    localPeers []peer.Peer   // what the local pass has found                   (under n.Mutex)
    nodeLookupRunning atomic.Bool      // one lookup in flight at a time
    nodeLookupDone    chan struct{}    // cap 1; one signal per completed lookup
    rebuildGen        atomic.Uint64    // bumped by every signalRebuild
}

Init: nodeLookupDone = make(chan struct{}, 1)

manageResponsiblePeers():
    for { fetchResponsiblePeers(); if !waitNextFetch() { return } }

waitNextFetch() bool:                      // replaces the inline select
    timer := time.NewTimer(nextCheckInterval()); defer timer.Stop()
    for { select {
        case <-timer.C:                    return true
        case <-rebuildResponsiblePeers:    return true
        case <-nodeLookupDone:             timer.Reset(nextCheckInterval())   // the verdict decides the cadence
        case <-ctx.Done():                 return false } }

fetchResponsiblePeers():
    startNodeLookup()        // non-blocking; no-op while one is in flight
    fetchLocalPeers()        // bounded 2 s per peer, publishes incrementally

startNodeLookup():
    if !nodeLookupRunning.CompareAndSwap(false, true) { return }
    startGen := rebuildGen.Load()          // every rebuild counted so far is behind this lookup
    go func() {
        defer func() { nodeLookupRunning.Store(false); refetchAfterLookup(startGen); non-blocking send on nodeLookupDone }()
        lookupNode()
    }()

refetchAfterLookup(startGen):              // once per lookup, never after Close; can start a new lookup now
    if rebuildGen.Load() != startGen || the node now served is closed { signalFetch() }

lookupNode():                              // the old node half of fetchResponsiblePeers, verbatim logic
    prev := nodeStatus.GetNodeStatus(spaceId)
    p, err := pool.GetOneOf(n.ctx, getNodeIds())      // still UNBOUNDED (GO-7409/7410)
    if n.ctx.Err() != nil { return }                   // closed meanwhile: verdict void
    if err == nil { failures=0; SetNodesStatus(Online); setNodePeer(p); if prev==ConnectionError { kickDiffSyncOnReconnect() } }
    else          { failures++; log "can't get node peers"; SetNodesStatus(ConnectionError); setNodePeer(nil) }
    spaceSyncService.Refresh(spaceId)

fetchLocalPeers():                         // the old local half, plus incremental publish and the close guard
    ids := peerStore.LocalPeerIds(spaceId); peers := nil
    for id in ids:
        p, err := getLocalPeer(id)
        if err != nil { if n.ctx.Err() != nil { return }   // closing: do not evict from the shared store
                        peerStore.RemoveLocalPeer(id); warn; continue }
        peers = append(peers, p); serveLocalPeer(p)         // serve each success as it lands
    setLocalPeers(peers)                                     // the list becomes exactly what the pass found

getLocalPeer(id):                          // one 2s budget for both calls
    p, err := pool.Get(ctx, id); if err == nil { return p }
    if !closing { if p, ok := pool.Pick(ctx, id) { return p } }   // connected in during the failed dial
    return wrapped err

setNodePeer(p) / setLocalPeers(peers): under n.Mutex, store, then publishLocked()
serveLocalPeer(p): under n.Mutex, replace the entry with p's id or append; publishLocked()
                   (peers the pass has not reached yet stay served until it ends)
serveLocalPeer: if p is closed already, signalFetch() once (no watcher will report it)
publishLocked(): list = [nodePeer?] + localPeers, live peers only; start watchPeer once per id;
                 swap responsiblePeers; close availableResponsiblePeers if len>0
                 (= old publishResponsiblePeers, composed)
signalFetch(): the non-blocking channel send without resetting consecutiveFetchFailures
               (used by watchPeer and by 2.2)
signalRebuild(): reset counter; rebuildGen.Inc(); signalFetch()
watchPeer(p): wait for close or ctx; release p's id in watchingPeers; then signalFetch() unless closed
              (released first: the fetch may serve a reconnected peer at once, and it needs a watcher)
```

Why the re-arm on `nodeLookupDone` matters: today the loop evaluates `nextCheckInterval()` *after* the
walk, so "retry 5 s after a failed lookup" counts from the failure. With the lookup off the loop the
timer would otherwise have been armed with the pre-verdict cadence (20 s) and the fast retry would slip
by up to 20 s. Re-arming from the verdict reproduces today's cadence exactly (test
`Test_manageResponsiblePeers_retryCountsFromNodeVerdict`, on fake time under `testing/synctest`).
A connectivity rebuild that arrives while a lookup is in flight finds the single-flight slot taken and
runs only the local pass. If the walk is still inside `GetOneOf`, the flush that precedes every
connectivity hook cancels the old pool pair and `pool.lookup` restarts the walk on the new pair, so
the verdict is post-change. But a lookup that has already returned and is still applying its verdict
(e.g. paused in `Refresh`) holds a pre-change result; without a replay a node reachable again waits for
the 5 s fast retry. So every rebuild bumps `rebuildGen`, each lookup reads it when it takes the slot,
and a lookup that finds it moved when it ends signals one fetch (`refetchAfterLookup`). Only rebuilds
count: replaying plain fetch signals or cadence ticks would follow a long offline walk with another at
once.

Why the hand-off is race-free: every lookup runs its check *after* releasing the slot, and decides only
from what it saw at its own start; nothing shared is consumed. Whoever holds the slot when the fetch it
signals lands started after that release, and runs the same check when it ends, against its own start.
So no rebuild and no closed node seen by one lookup can be lost to another, however their tails
overlap. (A consumable flag, the first version, failed this: an older lookup paused between release
and check consumed its successor's flag.) A fetch woken while a lookup is in flight runs only the local
pass (cheap: `Get` on a connected peer is the fast path; stale peers were evicted on first failure).

Properties kept, explicitly:
- **GO-7409/7410** — the node lookup keeps `n.ctx`, no deadline (test
  `Test_fetchResponsiblePeers_nodeLookupIsNotTruncated` unchanged). Running it off the loop is what makes
  an unbounded walk affordable: nothing waits for it any more (locals, broadcasts, head-sync all proceed
  on the local peers).
- **GO-7379** — the node is published the instant `GetOneOf` returns, before any local dial settles
  (test `Test_fetchResponsiblePeers_nodePublishedBeforeLocalDials` unchanged); per-dial 2 s budgets and
  eviction unchanged (`Test_fetchResponsiblePeers_boundsEachLocalDialSeparately`); the reconnect diff
  kick fires on the same ConnectionError→Online edge, now from the lookup goroutine
  (`Test_fetchResponsiblePeers_reconnectKick`).
- **GO-3192** — `localPeerDialTimeout` = 2 s per LAN dial, untouched.
- **Offline cadence** — `nextCheckInterval` untouched (2 min offline without LAN peers, 20 s with,
  5 s×3 fast retry); a lookup that fails fast (LocalOnly: empty node ids) signals immediately and the
  loop re-arms with the same values as today.
- **Mobile** — same code; one extra goroutine per manager while a lookup is in flight (the loop
  goroutine stays, parked in its select; the CAS prevents stacking); one timer per wait, as before.
- **Flush races** — a lookup started before a `Flush` returns a peer from the *current* pair
  (`pool.lookup` retries on swap); if it is closed by publish time it is left out of the list, and the
  lookup, once it has released the single-flight slot, signals one fetch (`refetchAfterLookup`),
  standing in for the watcher that signalled it on develop. The same check covers a node that closes
  before the lookup has finished: its watcher's fetch found the lookup still in flight and ran only the
  local pass. Signalling from the setter instead loses the retry whenever the loop takes the signal
  first (e.g. the lookup is paused in `Refresh`), leaving a space without LAN peers nodeless until the
  next tick (20 s online, 2 min offline); develop's synchronous loop could not take it early.
- **Closed peers are never published.** A source keeps its last peer until its next result, so after a
  flush the closed node stays in `nodePeer` until the replacement lookup returns (20–70 s offline).
  Composing it in would give it a fresh watcher on every local pass, which would find it closed at once
  and signal another fetch: a tight loop until the verdict, with waiters woken for nothing. Its own
  watcher signals the one fetch its closure needs; `GetResponsiblePeers` still filters peers that close
  after publishing.

### 2.2 Serve a LAN peer the moment it joins a space (complement, small)

Today a peer that (re)appears through mDNS or an inbound space exchange is used only at the next
cadence tick (≤ 20 s), and a peer evicted by a failed 2 s dial comes back the same way. Register a
peerstore observer in `provider.Init` (`provider.go`):

```
p.peerStore.AddObserver(p.onLocalPeerUpdated)

onLocalPeerUpdated(peerId, before, after, removed):
    if removed { return }                               // removals are the manager's own doing (watchPeer / failed dial)
    for m in managers: if m.spaceId ∈ after and ∉ before { stats.localPeerSignals++; m.signalFetch() }
```

`signalFetch` (not `signalRebuild`) so a LAN event does not reopen the node fast-retry window. The
store notifies only on a real change (`UpdateLocalPeer` early-returns on an equal list), so the 2–3 min
cable re-exchanges of GO-7581 cost nothing. With 2.1 the signalled fetch serves the new connection at
once. Without 2.1 this signal would be useless in the observed stall (the loop was inside the walk), which
is why candidate 4 alone is rejected below.

Note that in the observed incident the iPhone's post-flush exchange republished the same 77 spaces, so
the observer does not fire there; 2.1 alone serves it. The observer matters when a peer is new to a
space or returns after an eviction.

Observers read the `before`/`after` lists outside the store lock, and `UpdateLocalPeer` used to sort the
stored list in place for its equality check: a data race with every observer still reading the previous
update's list (reproduced under `-race`). The store now sorts a copy of the caller's list once on entry,
stores it and hands it to the observers, so a stored list is never reordered.

### 2.3 Not in this PR: stop broadcasts parking the shared workers (follow-up ticket)

With 2.1 the workers park only for spaces that have no reachable peer at all, and only while edits
happen in such a space (see §1.5). The remaining defect — up to 4 broadcasts for peerless spaces can hold
the send path of every space for up to 30 s — is real but independent of this incident. Sketch for the
follow-up: in `BroadcastMessage`, if the served list is empty, append the message to a per-manager
pending buffer (cap ~100, coalesce by pointer for the keep-alive) instead of entering `streamPool.Send`;
drain the buffer into `Send` from `publishLocked` when the list becomes non-empty; drop entries older
than `broadcastPeerFindDeadline` on a per-manager timer. Same 30 s semantics, zero worker parking,
no head-of-line blocking across spaces.

## 3. Rejected alternatives

- **Bound the node walk** (shorter `GetOneOf` ctx): the GO-7409 regression — any bound smaller than
  nodes×schemes×addresses truncates the walk before the yamux fallback; and here the blocking DNS resolve
  is not ctx-cancellable anyway.
- **Local peers first, then the node**: holds a healthy node behind up to N×2 s of stale mDNS dials after
  a switch to cellular (GO-7379 in reverse); concurrency has no such trade-off.
- **`pool.Pick` the locals (no dial) before the walk**: fixes only the inbound-reconnect case of this
  log, not the case where the Mac has to dial the phone; `ocache.Pick` also blocks on a loading entry.
  Subsumed by 2.1.
- **Flush only connections on the lost interface** (candidate 2): the pool has no per-connection
  interface; quic dials from `0.0.0.0` so even `LocalAddr()` is unspecified; needs any-sync socket
  binding per interface — that is the parked QUIC path-migration work, not a fix for this.
- **Send path only** (candidate 3): lists would still be empty for 20–70 s; and the absolute deadline
  means parking does not lengthen the stall (§1.5). Kept as the follow-up in 2.3.
- **React to the inbound connection only** (candidate 4): the signal is consumed only after the walk
  returns; useless without 2.1. Kept as the complement 2.2.
- **any-sync changes** (ocache `TryPick`/non-blocking `getIfActive`, concurrent happy-eyeballs dialling,
  ctx-aware resolve in `quicTransport.Dial`): each shortens the walk or unparks waiters but leaves the
  locals gated on it; none is needed for this fix. The ctx-aware resolve
  (`net.DefaultResolver.LookupIPAddr(ctx, host)` instead of `net.ResolveUDPAddr`) is a cheap, worthwhile
  follow-up on its own: today a bounded dial cannot stop inside the resolve.

## 4. Tests (deterministic: gomock pool, channels for timing, `testing/synctest` for the cadence; no network)

In `space/spacecore/peermanager/manager_test.go` unless named otherwise; all pass 3× under `-race`
(`go test -p 1 -count=3 -race ./space/spacecore/peermanager/ ./space/spacecore/peerstore/`).

Fixture changes: `nodeLookupDone` and `rebuildResponsiblePeers` channels; helpers `fetchAndWaitNode(t)`
(fetch, then wait on `nodeLookupDone`), `waitNodeVerdict(t)` and `waitFetchSignal(t)`. Existing tests
that asserted right after a synchronous `fetchResponsiblePeers()` now call `fetchAndWaitNode(t)`; the
two GO-7379 regression tests keep their shape (the node-before-stale-local one adds a `waitNodeVerdict`
at the end so the mocked `Refresh` is accounted for). Every failing local `Get` now also expects a
`Pick` (§6).

New:
1. `Test_fetchResponsiblePeers_localPeersServedWhileNodeLookupBlocks` — the field failure: `GetOneOf`
   parks on a channel; `Get("cablePeer")` returns at once; `GetResponsiblePeers` with a 1 s deadline
   returns `[cablePeer]` while the walk is parked; after the walk fails the list still holds the local
   peer and the node status is `ConnectionError`. Against develop's node-first order it fails in 1 s.
2. `Test_fetchResponsiblePeers_nodeLookupSingleFlight` — three fetches while a lookup is parked →
   `GetOneOf` called once, `Get` three times (one local pass per fetch); after the verdict the next fetch
   starts a new lookup.
3. `Test_fetchResponsiblePeers_nodeVerdictAndLocalPassAreIndependent` — success → `[node, local]`;
   failure → `[local]` (failures=1); success → `[node, local]` (failures=0).
4. `Test_fetchLocalPeers_noEvictionWhenClosing` — cancelled manager ctx, `Get` returns `ctx.Err()`,
   the store still lists the peer. Superseded by 19.
5. `Test_manageResponsiblePeers_retryCountsFromNodeVerdict` — the real loop on fake time: a 30 s
   failing walk started while Online; the 20 s tick starts no second lookup; the retry starts at 35 s
   (5 s after the verdict), not 40 s.
6. `Test_provider_onLocalPeerUpdated` — through `provider.Init` and the real store: signals only the
   managers of spaces newly joined (first join, added space, return after eviction), nothing for an
   unchanged list, a left space or a removal; does not reset `consecutiveFetchFailures`; counts
   `localPeerSignals`.
7. `Test_fetchLocalPeers_servesPeersAsTheyLand` — `peerA` newly found, previous pass served
   `[stalePeer (closed), peerB]`; while the new pass is parked on `stalePeer`, both `peerA` (found) and
   `peerB` (not reached yet) are served; afterwards the list and the store hold `[peerA, peerB]`.
8. `Test_fetchLocalPeers_servesPeerThatConnectedDuringFailedDial` — `Get` fails, `Pick` returns the
   inbound connection: it is served and the peer stays in the store.
9. `Test_watchPeer_reconnectedPeerIsWatched` — no fetch signal while the manager mutex is held (the
   watcher releases its slot first); a reconnected instance with the same id then gets its own watcher.
10. `peerstore.TestUpdateLocalPeerKeepsObserverListsIntact` — a list handed to an observer is not
    reordered by the next update.
11. `peerstore.TestUpdateLocalPeerDoesNotTakeOwnershipOfInput` — the caller's slice keeps its order, and
    reusing it reaches neither the observers' lists nor what `RemoveLocalPeer` cleans up.
12. `Test_manageResponsiblePeers_closedNodeCostsOnePass` — the real loop on fake time, with no LAN peers
    and with a live cable peer: the served node closes while its replacement lookup walks on; exactly
    one pass follows, the closed node is not served, and the next pass comes on the 20 s tick. A store
    that counts passes stops the manager past 10, so a regression fails instead of spinning.
13. `Test_manageResponsiblePeers_closedNodeIsLookedUpAgainAtOnce` — the real loop on fake time, the
    first lookup held in `Refresh`; its node is closed on arrival, or closes before the lookup finishes:
    the replacement lookup starts at once (not at 20 s) and its node is served.
14. `Test_lookupNode_resultAfterCloseIsVoid` — a node result that lands after `Close` leaves the status,
    the served list, `Refresh` and the reconnect kick alone.
15. `Test_manageResponsiblePeers_closedLocalPeerIsFetchedAgainAtOnce` — the LAN twin, with the node
    lookup still walking: a connection closed by the time the pass serves it triggers another pass at
    once, which serves the new connection.
16. `Test_manageResponsiblePeers_rebuildDuringLookupIsReplayed` — the real loop on fake time, a failed
    lookup held in `Refresh`: a rebuild signalled meanwhile starts the replacement lookup at 0 s, a
    plain fetch signal does not; afterwards a rebuild while idle costs exactly one lookup.
17. `Test_fetchLocalPeers_pickSharesTheDialBudget` — on fake time, `Get` and `Pick` of a stale peer both
    block until their ctx is done: the pass ends, with the healthy peer behind it served, at exactly 2 s.
18. `Test_manageResponsiblePeers_overlappingLookupTailsKeepTheRebuild` — a test hook pauses lookup A
    between releasing the slot and its check; a fetch starts B, which fails and is held in `Refresh`; a
    rebuild lands; A then B finish: the third lookup starts at 0 s. The consumable-flag protocol fails it.
19. `Test_fetchLocalPeers_noEvictionWhenClosedMidDial` (replaces the close-before-the-pass version) — the
    manager closes while the phone's dial is in flight: the phone stays listed for this space and for
    the other space it shares.
20. `Test_fetchLocalPeers_peerLeavingTheSpaceIsNoLongerServed` — a still-connected peer leaves its last
    space in this manager: the next (empty) pass stops serving it.

Each test that pins a fix (1, 2, 4, 5, 7–20) was checked to fail with that fix reverted.

## 5. On-device check

Setup as in the handoff: Mac on Wi-Fi + USB-C cable to the iPhone (iPhone Wi-Fi/cellular off), logs at
localdiscovery+spacecore DEBUG, peermanager/streampool INFO. Build with the fix, open a shared space on
both, then turn the Mac's Wi-Fi off and keep editing on the Mac.

Expect, relative to `net interfaces configuration changed` (the flush):
- `failed to get local from net pool` for the Wi-Fi-era peers within ~2–4 s (today: ≥ 20 s), none for
  the iPhone;
- edits reach the iPhone within ~1–2 s throughout; no `streampool … peer find deadline exceeded` for
  spaces shared with the iPhone (a handful for unshared spaces with local edits is expected);
- no `headsync periodic call error … context deadline exceeded` burst at +60 s for shared spaces;
- `can't get node peers` still appears 20 s+ later, in waves — unchanged and expected;
- debugstat `peermanager`: `managers` = loaded spaces; `localPeerSignals` grows only when a peer joins a
  space (first discovery, return after an eviction), not on a re-exchange with the same list.
Then Wi-Fi back on: node status Online, `reconnectDiffKicks` increments, nothing dropped.

Regression checks (one run each):
- GO-7379: iPhone on Wi-Fi with another LAN device discovered, switch to cellular: node published
  within its dial time, stale entries evicted after ≤ 2 s each, no 20–40 s stall.
- GO-7410: self-hosted client.yml with a black-hole `quic://127.0.0.1:1011` entry ahead of the yamux
  address: sync still establishes after the walk (the walk is still unbounded); LAN peers, if any, sync
  immediately meanwhile.
- Offline cadence: airplane mode, no LAN peers: debugstat shows the 2 min cadence, no dial storm; with a
  LAN peer present: 20 s cadence, LAN sync unaffected.

## 6. Risks and notes for the implementer

- `Refresh` and `SetNodesStatus` now run on the lookup goroutine; both were already called concurrently
  from 83 loops, no new contention. `prevStatus` is read at lookup start; only the lookup writes the
  status, and lookups are single-flight, so the edge detection is unchanged.
- The lookup goroutine outlives `Close` until `GetOneOf` returns (the pool honours `n.ctx`; the cgo
  resolve does not, but the pool returns without it). Same as the loop goroutine today. The
  `n.ctx.Err()` guard keeps a post-close verdict from touching status/peers.
- `set responsible peers` (Debug) fires once per local success now; harmless.
- An unbounded `GetOneOf` from another call site (`GetNodePeers`, rpcstore, subscribeclient) still parks
  behind the single-flight dial exactly as before; out of scope.
- The pass a flush triggers now dials a LAN peer while that peer is still reconnecting to us, and the
  pool does not hand an inbound connection to a dial already under way. A failed dial is therefore
  checked with `pool.Pick` (never dials, same 2 s budget) before the peer is evicted; otherwise a peer
  whose listener is dead (GO-7577) but which reconnected inbound would be evicted with a live
  connection and, its exchange already done, stay out until the next one.
- A back-to-back flush inside the 2 s local budget can still make a healthy LAN peer's `Get` fail with
  `DeadlineExceeded` and evict it if it has not reconnected yet; 2.2 brings it back on its next exchange.
- No any-sync change, no bump, no proto change.

## 7. Departures from the prototype

- No injectable `timeAfter`: `waitNextFetch` keeps one `time.Timer` and `Reset`s it on a verdict, and
  the cadence test runs the real loop on fake time (`testing/synctest`, already used in rpcstore).
- The local pass replaces or adds peers by id as they land instead of publishing the prefix of its
  successes, which hid the peers it had not reached yet behind a stale entry's 2 s dial.
- `watchPeer` releases its id before signalling (§2.1); `getLocalPeer` picks before evicting (§6).
- The peer store no longer sorts lists in place (§2.2).
- The observer computes the joined spaces once, before taking the provider lock.
- `publishLocked` composes only live peers (§2.1 "Closed peers are never published"); the prototype
  republished a retained closed node on every local pass and spun the fetch loop until the verdict.
