# Peer dial rework — research & design (v2)

Date: 2026-08-03. Follow-up to `docs/DialStrategyResearchHandoff.md` (§
references below are to that handoff). **v2** incorporates the 5-lens
adversarial review — findings, verdicts and traceability in
`docs/DialStrategyReview.md` (referenced below as R0/C*/M*). A Fable
meta-review pass (same doc, "Meta-review" section; F1-F10) audited the v2
resolutions, and a second Fable review over five previously-unexamined
lenses — hostile LAN, mobile resource economics, Transport-API shape,
observability, post-edit coherence — produced the G1-G12 amendments (same
doc, "Second focused review" section), including the new D10. Verified
against:

| repo | worktree | rev |
|---|---|---|
| anytype-heart | `/Users/roman/anytype/anytype-heart-dial-research` | c396cb040 (`dial-strategy-research`) |
| any-sync | `/Users/roman/anytype/any-sync-dial-research` | 9cd8cd75 (v0.13.0-alpha.6+1) |

Branch topology (verified, corrects v1): any-sync **`main` is the 0.12 line**
(tip = v0.12.18; v0.12.17/18 are payments-only). `v0.13.x` is a separate
branch carrying a protocol-epoch migration (ProtoVersion 12→13, drops compat
v9, files-v2, SpaceExchangeV2). All dial-path files (`quic.go`, `yamux.go`,
`pool.go`, `poolservice.go`, `peerservice.go`) are **byte-identical** between
the two lines, so the dial work lands on `main`, merges forward to v0.13.x,
and reaches heart as **v0.12.20** without touching the protocol migration.

## 0. Corrections to the handoff baseline

1. **`DialTimeoutSec` 10s → 3s is NOT landed** (handoff §2 says done; tree
   and history say 10s at `core/anytype/config/config.go:648`). Dropped
   deliberately: concurrency supersedes it, and 3s wall-clock punishes lossy
   600ms-RTT links.
2. **The 2026-07-27 spec file does not exist** anywhere (uncommitted session).
3. **The `tests/nodedial` harness does not exist** — recreate it (§6).
4. `PreferQuic` is *not* dead API: heart calls `PreferQuic(true)` unless the
   `PeferYamuxTransport` escape hatch is set (`config.go:321-324`). Clients
   dial quic-first; server binaries yamux-first. Both defaults — and the
   escape hatch's **exclusion** semantics (M8) — must be preserved.

## 0b. New root-cause lead (R0) — investigate before anything else

A server config with `quic.listenAddrs` set but `dialTimeoutSec` **unset**
makes the accept path build `WithTimeout(Background, 0)` (`quic.go:190`): the
server completes the QUIC/TLS handshake, instantly abandons `AcceptStream`
**without closing the conn** (leak), and the client's `HandshakeOutbound`
hangs forever — kept alive by keepalives, invisible to any firewall repro.
This matches the original self-hosted incident's symptoms and would explain
why the July repro never survived scrutiny. Stock dockercompose emits
`dialTimeoutSec: 10`; the exposed population is hand-edited configs.
**Actions:** obtain the affected self-hoster's node `config.yml` quic block;
re-run the black-hole repro against a node with `dialTimeoutSec` deleted.

## 1. Verified current-state map

### The dial stack (client)

```
clientPeerManager.fetchResponsiblePeers          heart, per space, 20s/5s/2m cadence
  └─ pool.GetOneOf(n.ctx /*unbounded*/, nodeIds) any-sync
       ├─ getIfActive → ocache.Pick              BLOCKS on in-flight loads (ocache.go:197)
       └─ shuffled sequential pool.Get per node
            └─ ocache.Get single-flight          load runs under FIRST caller's ctx
                 └─ peerService.Dial             sequential schemes × addrs walk
                      └─ quic.Dial / yamux.Dial  per-attempt transport caps (partial)
  └─ per local (mDNS) peer: Pick → udpprobe → pool.Get (5s per-dial bound)
```

### Load-bearing facts

- **quic.Dial** (`net/transport/quic/quic.go:108-165`): unconnected UDP
  socket per dial (no ICMP fail-fast — the GO-7421 probe exists because of
  this); ctx-less `ResolveUDPAddr`; QUIC handshake bounded by
  `HandshakeIdleTimeout` (idle) **and** quic-go's `2×` wall-clock
  (`handshakeTimeout()`, so 20s at our 10s setting — v1 understated this);
  `OpenStreamSync`+`HandshakeOutbound` ctx-only → deadline-free callers can
  hang forever. No `DialTimeoutSec <= 0` guard (yamux has one); a zero value
  breaks the **accept** path too (R0). `MaxIdleTimeout` not exposed → 30s
  quic-go default; with 10s keepalives, dead-flow detection is a hard ~30s
  floor and ceiling.
- **Deadline-free callers exposed**: heart `fetchResponsiblePeers`
  (deliberately unbounded) and `getExactPeer` (Background); any-sync
  `consensusclient`/`subscribeclient` `openStream`, `nodeconf` refresh
  (periodicsync timeout=0).
- **ocache**: `Get` single-flights under the first caller's ctx; failures
  are never cached (except the `errObject` below); `Pick` **blocks** on
  loading entries; no `TryPick`. On v0.13.x only, 851d38f4 adds abort-retry
  (`maxLoadRetries=3`) keyed off the *first caller's* ctx (this matters — C2).
- **Only failure memory in `net/`**: `poolservice.go:57-67` caches
  `ErrIncompatibleVersion` as a 20-min `errObject`. Everything else is
  forgotten instantly.
- **Prod topology**: every node = 2 yamux TCP (:443, :1443) + 1 quic UDP
  (:5430, the corporate-firewall population). Client quic-first ⇒ a UDP
  black hole costs a full handshake timeout before the first TCP attempt.
- **nodeStatus lives in heart**; sole writer is the *return* of the unbounded
  `GetOneOf` → offline detection latency = walk duration.
- **Duplicate-conn contract** (C1): `pool.AddPeer` on `ErrExists` closes the
  **existing** peer and keeps the newest (`pool.go:169-183`). Any dial scheme
  that lets two secure handshakes complete for one logical dial makes the
  remote keep the opposite conn from the dialer → mutual teardown.
- **Heart network signals** (merged): mobile `NetworkState.Set`
  (`{type, networkId}` — iOS iface+gateway digest, Android networkHandle,
  desktop nothing), 5s interface-diff + clock-jump monitor (`monitorGen`),
  `triggerRecovery` → `pool.Flush` + connectivity hooks + head-sync sweep.
  Hooks run **serially under `hookMu`, shared with `StateChange`** (M5).
  SSID is unobtainable and unnecessary.
- **Local peers carry zero network context**; nothing removes them on network
  change; the p2p UI counts peerstore entries, not live conns. **mDNS
  re-discovery of a known peer takes up to 1 hour** (TTL 3600, no
  re-broadcast, zeroconf sentEntries suppression) — any wrongful eviction
  severs LAN sync for that long (C7). `SetPeerAddrs(peerId, nil)` acts as a
  clear (verified byte-identical on pinned v0.12.16).

### Latent bugs in the blast radius (PR targets per row — `AddPeer` moved to PR-D per F5, `fileobject` is heart-1)

| bug | site | effect |
|---|---|---|
| accept-path zero timeout + missing `CloseWithError` on error paths | `quic.go:190-204` | R0: silent-server hang + server conn leak |
| `AddPeer` closes existing live conn on duplicate id | `pool.go:169-183` | C1 remote-side of mutual teardown |
| `pool.pick` bare `v.(peer.Peer)` assert | `pool.go:198` | panics on cached `errObject` |
| `getIfActive` early `return nil` on bad entry | `pool.go:117,127` | one bad entry aborts the scan |
| `Dial` leaks `mc` on peerId mismatch AND on `NewPeer` failure | `peerservice.go:134-141` | conn + socket leak |
| `nameserviceclient.doClientAA` indexes `[0]` before err check | `nameserviceclient.go:102` | panic on empty list |
| `fileobject` stamps 1-min peer-find deadline into a long-lived ctx | heart `fileobject/service.go:176-179` | C6-class bug already live in LocalOnly |

libp2p grounding (verified): ranker delays 250ms public / 30ms private, TCP
anchored after last QUIC; BH counter public-only, N=100/MinSuccesses=5,
skip-when-Blocked with 1-in-N probing — and an explicit warning that race
losers recorded as failures need a large N (C10); backoff per (peer,addr),
5s+1s·n², cleared per peer on success; in-flight dials run to completion.
We copy the *shapes*, not the parameters (C4) and not the IP-class scoping
(C5).

## 2. Design principles

1. **Concurrency over budgets** — make the walk concurrent and self-bounding;
   bounds become windows, not truncation hazards (GO-7409 lesson).
2. **Fail-fast evidence stays outside quic-go** — probe beside the dial,
   statistics above the transport.
3. **Nodes and local peers never share fate** (§5.1) — separate loops,
   separate failure state keyed by **provenance** (nodeconf vs discovered),
   never by IP class (C5); locals are capped so they cannot compete for
   radio/fd budget (M6).
4. **Network changes are events** (§5.2) — tag at discovery, invalidate on
   signal, reset statistics on signal.
5. **(new, C3) A dial success is not a health verdict** — connection-lifetime
   evidence (early death, inbound silence) must flow back into dial policy,
   or middleboxes that kill flows post-handshake defeat everything above.
6. **(new, C1) At most one secure handshake per logical dial** — the pool's
   one-conn-per-peer contract makes racing *handshakes* a protocol error;
   only transport establishment may race.

## 3. Target design

### D1. Detached dial ownership — an explicit ocache feature (enabler)

A new `ocache.WithDetachedLoad(parentCtx, window)` option, used by the pool's
`outgoing` cache (window ≈ 30s node / ≈ 10s local, parent = pool
`closingCtx`). Semantics (C2, M4):

- The load runs under `WithTimeout(parentCtx, window)`; callers only bound
  their *wait* via `waitLoad(ctx)`.
- The abort flag (`loadAborted`, v0.13.x) is computed from the **load's own
  ctx**, so a caller departing no longer marks the load aborted and the
  retry-storm path (4×window per peer) cannot arise. On `main` there is no
  abort-retry to reconcile.
- Every loading entry keeps a **cancel handle**; `Flush` bumps a pool
  generation, **cancels in-flight loads**, and evicts via `RemoveSame` on the
  snapshotted value — never by bare id, never blocking on `waitLoad` (M4).
  A load completing after its generation was flushed discards its conn.
- Eviction watchers are armed via an `onStored` hook after the value is
  committed, closing the `evictOnClose`-before-store gap (M10).

Consequence for heart (documented, unchanged): a caller-side wait timeout no
longer proves dial failure; local-peer eviction rules change accordingly (D7).

### D2. Two-phase ranked racing dial in `peerService.Dial`

Replaces the schemes×addrs walk. **The single-handshake invariant is the
load-bearing change** (C1): candidates race connection establishment, and
the any-sync secure handshake runs on **one candidate at a time**, in rank
order of readiness. No duplicate ever reaches the remote's `AddPeer`, which
makes client-side racing safe against **old servers** — removing what would
otherwise be a fleet-wide sequencing gate.

**Implementation shape (revised per second-review G7): prototype the
handshake-permit variant before any `Transport` interface change.** The
invariant is achievable with zero interface surgery: a per-logical-dial
permit carried in ctx and acquired immediately before the secure-handshake
step of each transport (`OpenStreamSync` in quic, `SecureOutbound` in yamux,
`HandshakeOutbound` in webtransport); a cancelled loser fails as
`context.Canceled`, which is D2.5's "neutral" for free. If the ranker turns
out to need explicit phase-1 results, add an `OnEstablished(scheme, addr,
identity?)` callback on the permit rather than restructuring
`transport.Transport` — a split interface touches three transports
(webtransport twice: `_native` + `_js` build tags, and it is one of the two
files diverging between the 0.12/0.13 lines), mockgen mocks and rpctest. If
a split is still chosen, the correct seam for yamux is **inside
secureservice** (libp2p-TLS `SecureOutbound` yields `RemotePeer()` before
the any-sync handshake), making quic and yamux symmetric; and identity at
phase 1 is **tri-state** — known (quic cert), late (yamux post-TLS),
never-before-handshake (webtransport, `InsecureSkipVerify`) — the ranker's
peerId check must handle all three. Honest costing (G7 corrects M3): quic
losers still cost the remote a full server TLS handshake in phase 1; what
the invariant removes is the *any-sync* `CheckCredential` amplification and
the duplicate-`AddPeer` hazard.

1. **Candidates**: flatten to `{addr, scheme, transport, ipClass,
   provenance}`. `preferQuic=true` (clients) anchors quic first;
   `preferQuic=false` keeps today's **exclusion** semantics — quic candidates
   are appended only after every yamux candidate has terminally failed (M8).
   Webtransport last.
2. **Delays**: preferred-scheme candidate *i* at `i×stagger` (30ms
   loopback/private, 250ms public); first fallback candidate at
   `lastPreferredDelay + 250ms` (30ms private); webtransport `+1s`.
   **In-flight cap ≤3 per Dial** and a length cap on discovery-sourced
   candidate lists (M6 — mDNS advertises one addr per interface, VPN/docker
   included).
3. **Winner & grace** (C10/M3; hardened per meta-review F4/F9): first
   transport-established candidate is handshaken — except when a
   fallback-scheme candidate establishes while the preferred scheme is still
   in flight: hold `max(500ms, ~1× smoothed handshake time)` — seeded from
   a cross-epoch prior or the candidates' own phase-1 timings, since a
   purely per-epoch sample is empty on the first post-switch dial, exactly
   where the adaptive grace matters (G12) — for
   the preferred candidate before handshaking the fallback (a fixed 500ms
   under-serves exactly the >500ms-RTT links it exists for). Each secure
   handshake attempt gets its **own budget ≈ min(4s, softCap/2)** with
   **promote-on-budget**, not just promote-on-failure — otherwise a hung
   handshake (the R0 server shape: transport phase succeeds, stream never
   answered) consumes the whole Dial with an established yamux candidate
   sitting unused. The per-epoch "quic won here last time" bit zeroes the
   anchor delay on subsequent dials (required, not optional — the entire win
   on high-RTT links), but is set only on **full dial success**, never on a
   phase-1 win — a hung-handshake server must not keep re-anchoring quic.
4. **Probe integration**: for probeable candidates, fire `udpprobe`
   concurrently at t=0 under **its own 300ms ctx**, cancellable (the current
   `Probe` blocks in `Read` on a socket deadline and ignores ctx — must be
   fixed in the relocation, see PR-C2). `Dead` requires **two** refusals,
   and demote-then-verify applies to **all** candidate classes — including
   local peers (G2 inverts v2.0's node-only scoping: local peers are
   precisely the ones on hostile LANs where ICMP is forgeable, so a probe
   verdict must never be *destructive* eviction evidence on its own; see
   D7's eviction rule and the typed-error contract in D2.6).
5. **Outcome recording rules** (C10, M11): a candidate records a failure
   only on its **own terminal error** (connect timeout, refusal, handshake
   failure). Cancelled losers are **neutral** — never fed to backoff or the
   BH counter (libp2p's own footgun). Success clears backoff **per-addr**,
   as the final write of the Dial.
6. **Errors** (M1): always return
   `errors.Join(net.ErrUnableToConnect, attemptErrors…)` so existing
   `errors.Is` consumers keep matching unconditionally.
   `HandshakeError`/`ErrIncompatibleVersion` classification requires
   **unanimity** across attempts (a single stale endpoint must not blackball
   a peer — the 20-min `errObject` becomes 60s on client builds). In the
   **same commit** (meta-review F6): `nodeconf`'s bare `switch err` equality
   becomes `errors.Is`, and `pool.GetOneOf`'s bare
   `lastErr.(handshake.HandshakeError)` type assert (`pool.go:163`) becomes
   `errors.As` with joined-error pass-through — that assert otherwise
   flattens the unanimous classification to `ErrUnableToConnect` before any
   downstream consumer sees it. Both cases go into the compat test.
   Additionally (G11): when every candidate was **provably dead** (probe
   verdict, ECONNREFUSED), join a typed **`ErrAllAddrsDead`** alongside the
   sentinel — after PR-C2 relocates the probe, this is the only channel
   through which heart's eviction rule (D7: "definitive evidence only") can
   still distinguish proven-dead from wait-timed-out, which D1 forbids
   treating as failure.
7. **Failure latency** (M2; mechanism fixed per meta-review F3): the **soft
   cap ≈8s releases the *waiters*** with the joined error while the ocache
   load itself **stays open** under its D1 window and generation — the entry
   is not deleted on the soft-cap error, so a late success completes into
   the still-live entry (never routed through `AddPeer`, which would re-enter
   the C1 duplicate contract via the incoming cache), a late failure records
   its outcome into D4/D5, and Flush's existing cancel handle covers the
   straggler. `GetOneOf` staggers the (shuffled) node walk ~2s apart instead
   of strictly sequentially → an offline verdict in ≤ ~12s, which is also
   the new nodeStatus flip latency.
8. **Lock discipline** (M5): snapshot addrs + preference + BH/backoff state
   under a short RLock; dial entirely lock-free; record outcomes under a
   separate fine-grained mutex. `SetPeerAddrs`/`ResetDialState` must never
   block behind a walk.
9. **Kill switch** (M9; sharpened per meta-review): a flag (default on)
   falling back to the sequential walk. "Read at Init" alone is **not** a
   fleet-emergency mechanism — heart client builds cannot roll back, so the
   flag must be togglable without an app update: at minimum via the heart
   config file / env (self-hosters and support can disable per device), and
   the design accepts that a *fleet-wide* remote disable has no existing push
   channel — if one is wanted it must ride the nodeconf/coordinator config
   refresh, decided before PR-C ships in a store build.

### D3. Per-attempt caps inside transports (correctness floor)

- **Guard first**: `DialTimeoutSec <= 0 → 10` in quic `Init`; plus
  `CloseWithError` on every accept error path (R0's leak); plus a unit test
  asserting the guard so a derived `WithTimeout` can never regress either
  branch line.
- quic `Dial`: resolve **before** socket creation via ctx-aware
  `net.Resolver`; post-handshake phases (`OpenStreamSync` +
  `HandshakeOutbound`) get a fresh derived window from a **new config field**
  (`postHandshakeTimeoutSec`, default = `DialTimeoutSec`) — not the same
  knob, because operators already tune `DialTimeoutSec` (heart's netcheck
  ships 60) and its idle-vs-wall meanings must not silently double (M9).
- Expose **`MaxIdleTimeoutSec`** in `quic.Config` (today hardwired to the
  30s quic-go default); heart sets **15s** (see D9 — 12s was rejected per
  meta-review F8) so dead flows are detected in ~15s
  instead of 30s (C3).
- webtransport `Dial`: same derived cap. yamux: already self-bounding.

### D4. UDP black-hole detection — recalibrated (C4, C5)

- **Keying**: (network epoch, scheme=UDP, provenance=node). Provenance —
  peerId resolved via `nodeConf.PeerAddresses` = node; via `peerAddrs`
  (mDNS/push) = excluded — NOT IP class: self-hosted RFC1918 nodes are the
  original incident population and must be covered; LAN neighbours must not
  pollute node state. IP class is used only for stagger timing.
- **Parameters sized to our dial volume**: window N≈8 with sample aging
  (≈10min), MinSuccesses≈2. libp2p's N=100 needs ~11 minutes of failure to
  engage at our ~9 outcomes/min and stalls entirely once a pooled yamux conn
  stops dial traffic.
- **Blocked behavior**: quic candidates are **skipped** (not demoted — a
  demoted candidate anchored behind yamux can never win, latching the state),
  with one unanchored quic-first **probe dial per 60-120s** (time-based; the
  count-based 1-in-N probe would take hours at our volume).
- **Inputs**: terminal dial outcomes (D2.5) **and early-death events**
  (D9). **Successes are provisional** (meta-review F1): a dial success
  counts toward MinSuccesses / flips Blocked→Allowed only once the conn has
  survived ~60s or shown inbound stream activity; an early death
  retro-converts its own dial's success sample into a failure. Without this,
  the post-handshake-kill trap (every dial "succeeds", flow dies ~15s later)
  re-latches Allowed on every cycle and Blocked never engages.
- **Reset**: only on a genuine network-epoch change — NOT on every
  `recover()` (foreground resume >15s would otherwise wipe the window
  constantly). An epoch change **invalidates rather than erases** (G3): the
  window is marked stale and re-verified on next use, with a cross-epoch
  prior retained — full erasure would let a LAN attacker who can churn the
  interface-address delta (rogue RAs, DHCP churn) permanently cold-start
  every learned state. The epoch itself is damped (see D7).
- **Suspension taint — per-event, not detector-gated** (G5): stamp each dial
  attempt *and each conn establishment* with wall+monotonic clocks; discard
  any outcome whose wall−monotonic drift exceeds ~1s. The v2.0 rule ("the
  clock-jump detector fired mid-dial") missed sub-30s iOS freezes entirely
  and raced the 5s tick on resume; and conns idling out *during* suspension
  must not surface as same-epoch early-death failures on foreground.
- **Unblock rule** (G12, was unspecified): Blocked→Allowed requires
  `MinSuccesses` **confirmed** (post-provisional) successes; with probe
  dials every 60-120s and the ~60s survival requirement, realistic recovery
  from a false Block is **2-4 minutes** — Risk 5's bound is restated
  accordingly.

### D5. Per-(peer, addr) dial backoff

Quadratic `3s + 1s·n²` capped at 2min (base ≠ any heart cadence constant —
the 5s==5s resonance made A/B numbers irreproducible, M11). Cleared per-addr
on that addr's success (not peer-wide — the winner's clear must not erase the
losers' evidence ordering), by `ResetDialState()`, and by `SetPeerAddrs` for
that peer. Behavior inside a Dial: **reorder only for private/local
candidates** (their full candidate set costs a few LAN RTTs; pruning can
strip a multi-homed peer's one good addr); public candidates whose backoff
fired ≥3× in the current epoch are skipped unless last-remaining.
Never fails a Dial by itself; heart's cadence stays the retry authority.

### D6. `TryPick` — on `pool.Pool`, not just inside it (C8)

- ocache: non-blocking `TryPick` (ErrNotExists while loading); loading-safe
  `TryRemove`/GC (closes GO-7333).
- pool: `getIfActive` uses `TryPick` + `continue`; `pick` uses `getPeer`;
  **`TryPick` exported on the `Pool` interface** — heart's local fast path
  (`manager.go:394`) currently calls `Pick(n.ctx, …)` which blocks unbounded
  on an in-flight load; under D1 that block would grow to the full detached
  window. Heart switches to `TryPick` (heart-2).
- Expectation: fixes warm-case parking; cold-start is fixed by D1+D2.

### D7. Network tags + invalidation in heart — conservative edition (C7, M7)

- `device.NetworkState` exports `NetworkEpoch() int64` (type change +
  non-empty networkId change + monitor loss/regain). Opaque, equality-only.
  **Damped** (G3): a monitor-driven delta must persist ≥2 ticks and epoch
  bumps have a minimum ~30s spacing with exponential damping — the
  interface-address delta is the most forgeable signal on a shared LAN
  (rogue IPv6 RAs / DHCP churn), and v2.0 elected it as the authority for
  state erasure and `pool.Flush` at the 6s suppression floor.
- peerstore stamps `{epoch, firstSeen, lastSuccess}` **per address**, and
  discovery writes are a **union, never a replace** (G2):
  `SetPeerAddrs`/`SetLocalPeerAddrs` are whole-map replaces today, so one
  spoofed mDNS announcement carrying a victim's peerId displaces its entire
  address list until re-announce. A discovery write must never remove
  addresses of a peer holding a live pooled conn; stale addrs age out via
  epoch/lastSuccess instead. (`SpaceExchange`'s `LocalServer.Ips` is
  likewise validated against the connection's remote subnet — see D10.)
- **Invalidation is gated on evidence the local L2/L3 actually changed**
  (netmonitor address delta), not on an opaque path-id change alone: iOS
  re-attach and Android networkHandle churn bump ids while the LAN is
  untouched, and a wrongful eviction costs up to an hour of LAN sync (C7).
  On id-only changes: **demote + re-verify by probe** instead of delete.
  Android (no interface provenance, no re-announce control): demote-only.
- Invalidation is atomic across layers (M7): peerstore prune +
  `peerService.SetPeerAddrs(peerId, nil)` + **`pool.Remove` of the live
  entry** + cancel of in-flight dials via D1 handles; the winner path
  re-checks the epoch before caching. The hook **collects under `hookMu` and
  applies after release** — `SetPeerAddrs` contention must never reach the
  foreground `StateChange` RPC (M5).
- **Re-announce path** (prerequisite for any eviction policy): **both**
  halves are mandatory, not alternatives (meta-review F10) — the publish TTL
  drop to ~120s heals only the *remote's* view of us, while recovering *our
  own* view after a wrongful eviction requires the forced browse-client
  restart (the teardown+rebuild machinery already exists in
  `refreshInterfaces`/`onNetworkStateChanged`). Today a known peer is not
  re-announced for up to 1 hour, which falsified v1's "re-adds within
  seconds" three times over. **Cost correction (G1)**: the real price of TTL
  120s is not multicast chatter (the browse query cadence is TTL-independent
  and the server announces once per generation) — it is that zeroconf's
  `sentEntries` dedup expires with the TTL, so each LAN peer is
  **re-delivered every ~2 minutes**, and today's `PeerDiscovered` path runs
  a full dial + `SpaceExchange` DRPC round trip per delivery with no dedup
  (~10 RPC/min on an idle phone in a 20-peer office). **Mandatory
  mitigation**: suppress the `PeerDiscovered` action path when the peerId is
  already pooled with an unchanged address set — only a changed addr set or
  a dead pool entry triggers dial/exchange. The TTL drop must not ship
  without this.
- Eviction on dial failure (heart loop): only on **definitive** evidence —
  the typed `ErrAllAddrsDead` from D2.6 (G11: after the probe relocates into
  any-sync, heart cannot observe probe verdicts directly, and the joined
  sentinel alone cannot distinguish proven-dead from wait-timed-out) —
  never on the caller's wait-timeout, which under D1 no longer implies
  failure. Probe `Dead` alone demotes-and-re-verifies; it is not eviction
  evidence on a hostile LAN (G2). A periodic reconciler re-adopts
  pooled-but-unlisted local peers into the peerstore so the pool and the
  p2p UI cannot diverge for minutes (M7).
- **Offline-cadence interaction** (G4): the 2-minute offline cadence gates
  on `len(LocalPeerIds) == 0`, which demote-instead-of-delete would keep
  permanently non-empty — the phone would dial demoted, unroutable peers at
  20s cadence on cellular forever. The gate counts only **epoch-current,
  non-demoted** peers, and demoted peers are not dialed while the device
  believes it is offline.

### D8. Space-load deadline — rewritten (C6) and re-sequenced

- The peer-find deadline becomes a **duration**, resolved inside
  `waitResponsiblePeers` at wait time (or stamped per attempt inside
  `loadingSpace.load`) — v1's absolute timestamp in the spaceloader's
  once-created ctx made every retry fail instantly after 30s, turning
  "eventually loads" into "never loads" for LAN-only spaces.
- `ErrPeerFindDeadlineExceeded` maps to (or joins) `net.ErrUnableToConnect`
  so `spaceservice`'s retry loop keeps polling instead of aborting.
- The bound must also cover the ocache single-flight path (`SyncAllSpaceHeads`
  winning the load race bypassed v1's bound entirely).
- **Ships in heart-2, after the any-sync bump** — with today's sequential
  walk a 30s bound livelocks black-holed-network space loads (each attempt
  dies mid-walk and restarts from zero; v0.12.16 has no abort-retry).
- Independent immediate fixes (heart-1): the existing `fileobject` 1-min
  deadline-in-long-lived-ctx bug; pessimistic `ConnectionError` flip on the
  offline signal; `RegisterSpace`/`UnregisterSpace` ordering and
  `watchingPeers` keyed by peer identity (M10).

### D9. Connection liveness (new — C3)

Dial success is not health. Two additions close the "handshake succeeds,
flow killed" loop that would otherwise defeat D2+D4 permanently
(steady-state ~5s useful / ~30s dead, with the BH counter voting Allowed):

- **Early-death events**: a conn closing with an idle/keepalive cause within
  ~60s of establishment emits a failure event for its (addr, scheme) into D4
  and D5 — the redial then anchors the other scheme first. Hardening
  (meta-review F7/F8): events are **stamped with the epoch at
  establishment** and discarded cross-epoch (a pre-switch conn dying just
  after the switch must not pollute the new epoch's statistics — mirrors the
  suspension-taint discard); feeding **D5 backoff** additionally requires
  ≥2 consecutive idle-cause deaths on the same addr, so loss-induced churn
  on marginal links doesn't penalize quic exactly where yamux is the worse
  transport.
- **Dead-window shrink**: delivered by `MaxIdleTimeoutSec ≈ 15s` (D3) alone
  — 15s, not 12s: at 12s the effective keepalive halves to 6s and PTO chains
  on 600ms-RTT lossy links can plausibly exceed the window, churning healthy
  conns with full re-handshake cost. **Stated cost** (G10): quic-go caps the
  effective keepalive at idle/2, so 15s idle drops the per-conn PING period
  from today's 10s to 7.5s — ~25% more steady-state radio wakeups per
  connection. Accepted for the 2× faster dead-flow detection; revisit if
  battery telemetry objects.
- **Serve gating — respecced** (meta-review F2): v2.0's
  `BytesRead`-idle gate measured the wrong layer (quic `BytesRead` counts
  stream payload only — PING frames are invisible — while yamux counts its
  own keepalives; and a 15s idle gate on a 12-15s transport idle timeout can
  only ever fire on **healthy idle** quic conns). The gate is
  **request-scoped** instead: a peer with an outstanding request and no
  response bytes for >T (~2× write timeout) is treated as not-servable and
  triggers rebuild + status flip. Idle conns are left to the transport idle
  timeout, which now bounds the dead window at ~15s.

### D10. Observability, shadow rollout & adversarial hardening (new — G6/G8/G9)

The design adds six state machines (ranker, BH, backoff, epoch,
provisional-success, early-death) in a client where **every field channel is
closed by default**: logs run `*=WARN` (both existing dial log lines are
below it), any-sync prometheus is never exported by heart (empty
`metric.Config`), and telemetry has no network events. The only channel that
reaches support is debugstat. Therefore:

- **Minimal observability set (ships with PR-C, not after)**: a
  `net.peerservice` debugstat provider exposing the BH windows per (epoch,
  scheme, provenance) with transition timestamps, the backoff table, and
  counters {early-deaths, provisional successes, retro-conversions, soft-cap
  releases, straggler outcomes, budget promotions}; `peer.Stat` extended
  with `{scheme, addr, phase1Ms, handshakeMs, epoch}`; `net.peerservice=INFO`
  added to heart's default log levels with one structured line per dial
  attempt. All of it rides the existing debug-export path — zero new
  plumbing.
- **Shadow mode first** (G9): D4/D5 compute and export via the provider but
  **do not act** in their first shipped release; enforcement is enabled once
  field data shows acceptable false-Block / false-penalty rates. This is the
  only way to validate the state machines against production traffic — §6's
  e2e latency metrics cannot.
- **R0 field detection** (G9): the zero-guard must not silently normalize
  the misconfiguration it fixes — log a startup WARN naming the defaulted
  field, and stamp accept-error logs/counters with elapsed time so ~0ms
  aborts (the R0 signature) are distinguishable from genuine 10s timeouts.
- **Discovery-input hardening** (G6): global LRU cap on discovery-sourced
  peers and a device-wide dial-concurrency cap (all v2.0 caps were
  per-Dial); cardinality + TTL bounds on the backoff/BH tables (keyed by
  attacker-choosable (peer, addr)); `SpaceExchange.LocalServer.Ips`
  validated against the connection's remote subnet. Note: heart runs
  `noVerifyChecker` inbound and `SpaceExchange` returns all space ids to any
  LAN peer — the real fix is v0.13's token-based SpaceExchangeV2, which the
  v0.12.20 decoupling defers; until then the caps above bound the exposure
  that the 30× faster discovery loop (D7) would otherwise multiply.

### Explicitly dropped / deferred

- 3s `DialTimeoutSec` (never landed; wrong tool). Loopback 300ms cap
  (probe's best case). Race-and-remember (§4 Q4; the per-epoch
  "quic-won-here" bit provides the useful half). Connected-socket QUIC
  dialing (quic-go not trustworthy on platform errors). 0-RTT/session
  resumption (future latency work). dockercompose `127.0.0.1` fix (separate
  repo, trivial, still to do).

## 4. Answers to the handoff's §7 open questions

**Q1 — network identity.** An opaque epoch counter over existing signals
(RPC type/id change, monitor iface-diff, clock jump); equality-only, so the
platform-specific id contents don't matter and SSID (unobtainable) is
unnecessary. v2 adds: *destructive* reactions additionally require
address-delta evidence; id-only changes demote and re-verify (C7).

**Q2 — where does the BH counter live.** any-sync peerservice dialer, beside
the ranker; reset exported via `ResetDialState()`, called by heart only on
genuine epoch change. Scoping is by **provenance** (node vs discovered), not
public-vs-private (C5).

**Q3 — fan-out × single-flight.** No thundering herd: single-flight
collapses per peerId; the multiplier is capped at ≤3 in-flight per Dial (M6).
The v1 arithmetic missed local peers' unbounded interface lists and the
retry-storm path — both now capped/removed (C2, M6).

**Q4 — scheme racing policy.** Two-phase strict-preference racing with a
grace window; not Chromium race-and-remember. The remembered bit exists but
is scoped per epoch and only zeroes the anchor delay.

**Q5 — version skew.** All dial PRs on `main` (0.12 line) → v0.12.19/20;
merge forward to v0.13.x (only `webtransport_native.go` and `ocache.go`
diverge and need per-line treatment). Heart bumps to v0.12.20 — decoupled
from the ProtoVersion-13 migration, which is a separate coordinated rollout
with its own compile-break checklist (heart `netcheck.go` implements
`nodeconf.NodeConf`).

**Q6 — Windows `SIO_UDP_CONNRESET`.** The untested Windows risk is already
live in heart via GO-7421 (whole Windows desktop population); relocation adds
only (near-empty) Windows-server exposure. Plan of record: probe relocation
is its own PR (C2 below) with the Windows path compiled to `Inconclusive`
until a real-Windows validation run; heart's probe strip is a hard checklist
item on the bump PR so the double-probe window is zero-width.

## 5. Sequencing (v2 — corrects inverted branch labels)

Terminology: **`main` = 0.12 line** (what heart consumes); `v0.13.x` =
protocol-migration branch. Every PR below lands on `main` first and merges
forward.

| # | contents | release |
|---|---|---|
| A | quic zero-guard + accept `CloseWithError` fixes (R0) + `pool.pick`/`getIfActive`/`Dial`-leak/nameservice fixes + guard unit test. webtransport ctx cap as a per-line commit. **Server release with self-hoster comms — R0 is a server-side bug fix.** (`AddPeer` change moved to PR-D — see F5 note below.) | v0.12.19 |
| B | ocache: `TryPick` + loading-safe `TryRemove`/GC (GO-7333) + `WithDetachedLoad` option (D1). Per-line: on v0.13.x reconcile with 851d38f4 (abort flag from load ctx); on `main` there is no abort-retry. `Pool.TryPick` export. | with C |
| C | D2 two-phase ranked racing dial + D5 backoff + error policy (M1) + soft caps + lock discipline + kill switch. `postHandshakeTimeoutSec` + `MaxIdleTimeoutSec` config fields (D3 remainder). | v0.12.20 |
| C2 | `net/udpprobe` relocation: ctx-cancellable, own 300ms budget, 2-refusal `Dead`, Windows dark until validated. | v0.12.20 |
| D | D4 BH counter + `ResetDialState()` + D9 early-death/liveness plumbing + `AddPeer` duplicate tiebreak. | v0.12.20/21 |

**anytype-heart:**

1. *heart-1 (now, v0.12.16-compatible)*: D7 epoch + tags + conservative
   invalidation (incl. mDNS re-announce fix, `pool.Remove` pairing,
   hook-order test, apply-outside-`hookMu`); `fileobject` deadline bug fix;
   pessimistic offline flip; optionally bump to v0.12.19 for PR-A.
2. *heart-2 (bump to v0.12.20)*: strip GO-7421 probe wiring (checklist item);
   wire `ResetDialState()` into epoch changes; switch manager fast path to
   `TryPick`; D8 duration-based space-load deadline; `watchingPeers`/
   `RegisterSpace` ordering fixes; shrink `broadcastPeerFindDeadline` to
   **~10s** (must stay ≥ the 8s Dial soft cap or broadcasts give up just
   before the dial that would serve them completes — G10);
   update the GO-7409 comment.
3. *Anytime*: dockercompose `127.0.0.1` fix.

Ordering constraints: PR-A's server release **precedes** any heart build with
racing enabled (R0 hygiene; the phase split is what makes racing safe against
old servers). D8 never ships before the v0.12.20 bump (C6 livelock). Windows
validation gates only C2's Windows path going live.

**F5 note (meta-review) — why the `AddPeer` change moved out of PR-A:** a
naive keep-alive-existing inversion bounces the *most common* reconnect —
app restart / NAT rebind, where the server still holds a dead-but-undetected
conn (server idle timeout 30s default) and would reject the fresh inbound
one for up to ~30s of 5s-cadence redials. Keep-newest (today's behavior) is
what makes restart-reconnects instant. The correct tiebreak — keep the
existing conn only when it is *demonstrably alive* (recent inbound activity),
else keep the newest — needs the D9 liveness signal, so it ships with PR-D.
Until then, the phase split alone protects against C1's dial-race duplicates;
mutual simultaneous dials keep today's (rare, tolerable) behavior.

## 6. Test & validation plan

- **any-sync unit**: ranker tables (delays, exclusion semantics, backoff
  reorder/skip, BH skip+time-probe); two-phase race semantics (single
  handshake invariant — assert the remote sees ≤1 `HandshakeInbound` per
  Dial; grace window; promote-on-handshake-failure; cancelled-loser
  neutrality); detached-load semantics (Flush cancels + `RemoveSame`;
  no abort-retry storm; `onStored` eviction arming; soft-cap releases
  waiters while the load entry stays live and a late success lands in it);
  BH provisional-success / early-death retro-conversion; error-shape compat
  test (joined error matches `ErrUnableToConnect` always; `HandshakeError`
  only on unanimity, surviving `pool.go:163`); zero-guard test. All `-race`.
- **heart unit**: epoch invalidation (evidence-gated deletion vs demote;
  pool.Remove + SetPeerAddrs(nil) observed; hook ordering); TryPick fast
  path; D8 duration deadline across retries; reconciler.
- **e2e — recreate `tests/nodedial`** (own package; `InitialSetParameters`
  first; never await AccountSelect; poll via ANYPROF pprof; count `streams`
  not `total_size`; always `-count=1`; **interleave A/B arms**). Scenarios:
  UDP black-hole node; **R0 server (dialTimeoutSec deleted)**; post-handshake
  flow kill (drop UDP after N packets — validates D9); dead local port;
  Wi-Fi→cellular stale locals; LAN-only space load with late-appearing peer
  (validates D8); full offline cold start. Metrics: time-to-first-conn,
  time-to-ConnectionError (target ≤ ~12s), AccountSelect responsiveness,
  socket high-water mark, radio-active time on mobile profiles.
- **Windows**: `net/udpprobe` suite on real Windows before C2's path goes
  live.

## 7. Risks

1. **Error-shape compatibility** remains the sharpest edge (M1): unanimity
   classification + always-join-the-sentinel is the contract; the compat test
   is non-negotiable. `nodeconf`'s bare `switch err` fix rides PR-C.
2. **Two-phase dial is a `Transport` interface change** (connect/secure
   split) — more invasive than v1's fan-out, but it buys the single-handshake
   invariant that makes racing deployable against old servers. Mocks and the
   webtransport implementation must follow.
3. **Detached dials outlive interest** — bounded by window + generation
   cancel; a stale-generation success is discarded, not cached (M4).
4. **Server blast radius**: yamux-first preserved; accept paths unchanged in
   code *and* now in load (phase split, M3); nodeconf-refresh unbounded ctx
   fixed by D3. Accept goroutines parked ≤10s by cancelled losers: accepted,
   noted for capacity review.
5. **BH mis-blocking**: skip-when-Blocked + time-based probe + epoch
   invalidation bound the damage of a false Block; with provisional
   successes (F1) the realistic recovery is **2-4 minutes** (probe cadence
   60-120s + ~60s survival confirmation — G12 corrected v2.0's "≤2min").
   False Allowed remains impossible. Shadow mode (D10) measures the actual
   false-Block rate before enforcement.
6. **Repro debt**: the July repro was confounded (firewall anomaly) and R0
   offers a competing root cause — re-establish the repro (both variants,
   interleaved arms) before trusting any improvement numbers.
7. **Local-peer conservatism cuts both ways**: demote-instead-of-delete keeps
   genuinely-dead peers costing one bounded probe+dial per cycle longer than
   v1 would have — the price of never severing a healthy LAN for an hour
   (C7). The re-announce fix (TTL ↓) is what eventually makes deletion safe.
