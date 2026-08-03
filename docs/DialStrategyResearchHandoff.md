# Dial strategy research — handoff

Date: 2026-08-03
Author: session handoff (Roman + Claude), follow-up to
`docs/superpowers/specs/2026-07-27-quic-dial-budget-starvation-design.md`

Worktrees for this research:

| repo | worktree | branch | base |
|---|---|---|---|
| anytype-heart | `/Users/roman/anytype/anytype-heart-dial-research` | `dial-strategy-research` | `go-7421-udp-probe-local-peers` (c396cb040) |
| any-sync | `/Users/roman/anytype/any-sync-dial-research` | `dial-strategy-research` | 9cd8cd75 (v0.13.x) |

Heart pins `any-sync v0.12.16` in go.mod — the any-sync worktree is *newer* than
what heart consumes; check version skew before relying on any-sync-side code.

## 1. Problem recap

A QUIC address that black-holes (accepts UDP, never replies) burns a full
handshake timeout per attempt because any-sync dials on an *unconnected* UDP
socket — the kernel has nowhere to deliver ICMP port-unreachable, quic-go has no
ECONNREFUSED handling, it just retransmits Initials. Shared dial budgets layered
on top of the sequential scheme×addr walk turned this into total sync loss
(GO-7379 regression → GO-7409). Full analysis and design: the 2026-07-27 spec
above. This handoff supersedes the spec's *tactics* where they conflict; the
spec's evidence and root-cause sections remain authoritative.

## 2. Implementation status (verified 2026-08-03)

### Done in anytype-heart

- **GO-7409** (merged, PR #3224): `responsibleNodeDialTimeout` deleted —
  `pool.GetOneOf` gets unbounded `n.ctx` (`space/spacecore/peermanager/manager.go:355`).
  `localPeerDialTimeout` was NOT deleted but converted from one shared budget to
  a **per-dial 5s bound** (`manager.go:409`) — the shared budget was wrongly
  evicting healthy local peers when the first stale peer expired it.
- **GO-7421** (c396cb040, HEAD of base branch, not pushed): pre-dial UDP
  liveness probe for **local (mDNS) peers only** — `net/udpprobe`. Connected
  socket surfaces ICMP unreachable as ECONNREFUSED within ms on loopback/LAN.
  Verdict is asymmetric by construction: `Dead` only on proof (all addresses),
  `Inconclusive` on any doubt → falls through to normal dial. Windows:
  `SIO_UDP_CONNRESET` re-enabled per socket (`net/udpprobe/sockopt_windows.go`,
  Go runtime disables it — golang/go#5834). **Untested on real Windows.**
- **quic `DialTimeoutSec` 10s → 3s** (`core/anytype/config/config.go:663`):
  process-wide per-attempt cap on the QUIC transport-handshake phase; covers all
  seven any-sync callers because it's transport config.

### NOT done in any-sync (all spec items still open)

- **Item 1a/1b**: `quic.Dial` post-handshake phases (`OpenStreamSync`,
  `HandshakeOutbound`) unbounded for a deadline-free caller
  (`net/transport/quic/quic.go:108-165`); `net.ResolveUDPAddr` ctx-less. After
  GO-7409 the responsible-node path IS deadline-free, so this gap is
  load-bearing. NOTE: quic `Init` has **no `DialTimeoutSec <= 0` default guard**
  (yamux does, `yamux.go:49`); adding a derived `WithTimeout(ctx, 0)` would
  instantly break every deployment with the field unset. Guard first.
- **Item 2**: no `TryPick` in `app/ocache`; `pool.getIfActive` still blocks on
  `Pick` behind in-flight loads. Related-but-different: 851d38f4 on v0.13.x
  ("ocache: retry a load killed by another caller's context").
- **Item 4** (concurrent dial): not implemented — sequential scheme×addr walk.
- **Item 5** (loopback 300ms cap): not implemented; partially obsoleted by the
  probe direction (see §5).
- **Item 6** (dockercompose stops emitting `127.0.0.1` into client.yml): no
  evidence it was done.
- **Out-of-scope-but-filed**: `waitResponsiblePeers` unbounded on the
  space-load path → offline `AccountSelect` hangs holding
  `application.Service.lock`, freezing every RPC. Still the most user-visible
  residual bug.

## 3. Spec-review risks (from this session's review)

1. **Offline detection latency**: `nodeStatus` flips to `ConnectionError` only
   when `GetOneOf` *returns* (`manager.go:371-374`). Without budgets, a full
   sequential walk can take tens of seconds → sync-status UI lag, delayed
   fast-retry cadence. Concurrent dialing fixes this properly.
2. **`TryPick` alone does not kill head-of-line parking**: when *nothing* is
   connected yet (the Wi-Fi→cellular case), `GetOneOf`'s dial loop still
   single-flights through `ocache.Get` and blocks behind the in-flight dial.
   Only concurrent dialing truly compresses the cold-start walk.
3. **Per-attempt wall-clock vs idle timeout**: 3s is safe as an *idle* timeout
   (resets per packet). A 3s *wall-clock* cap over open-stream + secure
   handshake is a different quantity on a lossy 600ms-RTT cellular link. Keep
   the two distinct when implementing 1a.
4. **Server-side**: quic transport code is shared. Dial-side changes reach
   any-sync-node/coordinator/filenode outbound dials on the next bump. The
   accept path is untouched by all proposed changes. The `DialTimeoutSec` zero
   guard is the one server-breaking landmine.
5. **Loopback 300ms (item 5) can flake under CPU starvation** (throttled mobile
   app, CI container) — acceptable (retry next cycle) but expect flakes; 1s
   loses nothing once budgets are gone.

## 4. Ecosystem research (web, 2026-08-03)

**go-libp2p smart dialing** (since v0.28) is the closest production-proven
design for exactly this problem shape and should be the primary reference:

- **Dial ranker + staggered concurrent dial**: rank a peer's addresses, dial
  staggered, first success cancels the rest. QUIC before TCP; TCP delayed
  relative to the *last* QUIC dial. Delays: 250ms public / 30ms private (QUIC
  and TCP alike), relay +500ms, other transports +1s. RFC 8305 style.
- **UDP black-hole detection**: `BlackHoleSuccessCounter` — sliding-window
  success rate across *all* UDP dials on the current network; states
  Unknown → Blocked → Probing. When Blocked, QUIC dials are skipped entirely
  (straight to TCP) until a probe succeeds. **This covers the case an ICMP
  probe cannot**: a firewall silently dropping UDP is `Inconclusive` to a probe
  every time, but statistically obvious after a few dials. The original
  self-hosted incident (server dropping inbound UDP) is exactly this shape.
- **Per-peer dial backoff**: quadratic, 5s base → 5min max, cleared on success.
  Sits at the swarm/pool layer so all callers share it.

**Chromium**: races QUIC vs TCP happy-eyeballs style, caches "QUIC works on
this network/origin". Cautionary tale: imperfect fallback on silent UDP drop →
user-visible hangs instead of transparent TCP fallback.

**quic-go**: nothing to lean on for fast-fail. No API surfaces ICMP
unreachable as a dial failure; platform error handling in the read loop is
fragile (issues #3956 Windows `WSAEMSGSIZE` non-Temporary panics, #1737 Windows
resets; CVE-2024-53259 shows ICMP handling is security-sensitive). Conclusion:
**do not** switch the dial itself to a connected socket expecting quic-go to
react correctly — keep fail-fast logic outside quic-go (probe before dial, or
statistical detection above the transport).

Sources: go-libp2p swarm pkg docs (DefaultDialRanker, DialBackoff,
BlackHoleSuccessCounter), go-libp2p v0.29 release notes, RFC 8305,
chromium proto-quic threads on QUIC/TCP racing, quic-go issues #3956/#1737,
CVE-2024-53259.

## 5. Product constraints (from Roman, 2026-08-03 — normative)

1. **P2P/local peers are complementary, never prioritized.** They exist to
   speed up sync and tolerate network problems (offline LAN, flaky internet) —
   they must never delay or compete with node connectivity. Implications:
   - Node dial path must never wait on local-peer dialing (heart already
     publishes node peers before the local loop — preserve this).
   - A libp2p-style ranker must NOT borrow libp2p's "private addresses first"
     grouping — for us it is the opposite: nodes first, local peers dialed
     independently/in parallel, never in the same ranked list ahead of nodes.
   - Black-hole/backoff state for local peers must be tracked separately from
     nodes (a dead LAN neighbour says nothing about node reachability, and
     vice versa).
2. **Mobile clients already signal network changes** (iOS/Android call into
   heart on connectivity transitions). Use this: **tag peers with network
   context at discovery time** — e.g. "local peer discovered on Wi-Fi network
   X" / "connection established over interface Y". On a network-change signal:
   - Local peers tagged with the vanished network are invalidated (or heavily
     deprioritized) *immediately*, without spending probes or dials on them.
   - Tags can also scope the black-hole counter ("UDP blocked on *this*
     network"), resetting it on network change instead of slowly re-probing.
   - Research question: what identifies "the network"? Interface name + local
     IP/subnet is available today; SSID may not be accessible on all
     platforms/permissions. Existing signal plumbing: network-change handling
     already reaches heart (see `refreshInterfaces` / localdiscovery, and the
     GO-3958 recovery-pipeline work for prior art on reacting to these
     signals).

## 6. Recommended approach stack (ranked by leverage)

1. **Dial ranker + staggered concurrent dial in any-sync `peerservice`**
   (spec item 4, libp2p model): replaces the sequential scheme×addr walk.
   ~30ms stagger for private/LAN, ~250ms public, yamux delayed after quic,
   first success cancels losers (`MultiConn.Close()`), joined errors. Makes any
   single black hole cost ~250ms added latency instead of a full timeout, and
   collapses the offline-mobile worst case from ~180s sequential to one
   bounded concurrent window.
2. **UDP black-hole success counter** (new, from libp2p — not in the spec):
   shared across dials per network context; when Blocked, skip quic and go
   straight to yamux until UDP proves itself. Handles silent-drop black holes
   that probes cannot see. Scope per network tag (§5.2), separate node vs
   local-peer state (§5.1).
3. **Generalize the GO-7421 connected-socket probe into the any-sync quic
   transport** (`quicTransport.Dial`, after address resolution): advisory-only,
   `Dead`-on-proof → instant error, `Inconclusive` → normal dial. Covers node
   dials, all seven client callers, and server outbound dials. Bring
   `sockopt_windows.go`. Consider running probe concurrent with the dial
   (abort on proof) so `Inconclusive` costs zero added latency. Junk probe
   datagram to a live node is harmless (quic-go drops non-QUIC packets).
   Once landed, **strip the heart-side probe** on the next any-sync bump —
   don't probe twice. This also mostly obsoletes spec item 5 (loopback cap):
   dead loopback ports are the probe's best case (synchronous ECONNREFUSED).
4. **Per-attempt ctx caps in the transport** (spec 1a/1b): the correctness
   floor. Derived `WithTimeout` for post-handshake phases; ctx-capable DNS
   resolve (`net.Resolver.LookupNetIP`); **`DialTimeoutSec <= 0` default guard
   first**. Keep wall-clock cap distinct from (and looser than) the 3s idle
   timeout.
5. **Peer network tags + network-change invalidation in heart** (§5.2): local
   peers die instantly with their network instead of being probed/dialed;
   black-hole state resets per network.
6. **`TryPick` in ocache** (spec item 2): still worth it for the warm case
   (skip loading entries when another peer is already active), and fixes
   GO-7333. Just don't expect it to fix cold-start parking (§3.2).
7. **Bound `waitResponsiblePeers` on the space-load path** (heart): the
   remaining frozen-app path for genuinely offline users. Independent of all
   dial work; arguably the most user-visible fix in this list.
8. **dockercompose: stop writing `127.0.0.1` into client.yml** (spec item 6).

## 7. Open questions for the research agent

- Network identity for tags: interface+subnet vs SSID availability per
  platform; what do iOS/Android actually deliver through the existing
  network-change API into heart today?
- Where does the black-hole counter live — any-sync pool (shared, all callers)
  or heart (has network-change signals)? libp2p keeps it in the swarm
  (= pool-equivalent); network-change reset would then need a reset hook
  exported to heart.
- Happy-eyeballs fan-out interaction with `pool` single-flight per peerId:
  fan-out is per-peer bounded, but N spaces × 3 nodes still share dials —
  verify no thundering herd on reconnect signals.
- Scheme racing policy: strict quic-preferred with yamux delayed (libp2p
  style), or race-and-remember (Chromium style, cache "quic works here" per
  network tag)? The tag infrastructure (§5.2) makes the latter cheap.
- Version skew: fixes land in any-sync ≥ current v0.13.x while heart pins
  v0.12.16 — sequencing of the bump vs stripping heart-side mitigations
  (GO-7421 probe, 3s DialTimeoutSec rationale comment).
- Windows validation of `SIO_UDP_CONNRESET` re-enable (GO-7421) on a real
  Windows machine — still outstanding, becomes more important if the probe
  moves into any-sync where servers also run it.

## 8. Key file/commit pointers

- Spec: `docs/superpowers/specs/2026-07-27-quic-dial-budget-starvation-design.md`
- heart: `space/spacecore/peermanager/manager.go` (fetchResponsiblePeers,
  local loop, probe hook `localPeerProvedDead`), `net/udpprobe/`,
  `core/anytype/config/config.go:643` (GetQuic, 3s rationale comment),
  commits c396cb040 (GO-7421), 0b7abd765 (GO-7409), 663acdd91 / cf6701922
  (GO-7379 origin of the regression).
- any-sync: `net/transport/quic/quic.go` (Dial, no ctx caps, no zero-guard),
  `net/transport/yamux/yamux.go` (the self-bounding reference),
  `net/pool/pool.go` (GetOneOf/getIfActive), `app/ocache/` (no TryPick),
  `net/peerservice/` (scheme×addr walk — ranker goes here).
- e2e repro: heart `tests/nodedial/` (see spec "Harness notes" — always
  `-count=1`, never await AccountSelect, poll via ANYPROF pprof).
