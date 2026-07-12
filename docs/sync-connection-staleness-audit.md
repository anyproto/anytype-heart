# Sync connection staleness audit & fix program

Scope: how anytype-heart + any-sync (v0.12.14) recover sync connectivity after
macOS sleep/network change and mobile background/network switch; where stale
connections hide; where we burn network/CPU while clearly offline.

Verified against: any-sync v0.12.14 (`net/`, `commonspace/` identical to current
any-sync main), heart `develop`.

## How staleness happens (mechanics)

There is **no liveness probing above the transports**. A connection is "alive"
until the transport says otherwise:

- **yamux/TCP**: hashicorp/yamux keepalive ping. Heart leaves
  `KeepAlivePeriodSec` unset → 30s default interval, +10s pong wait
  (`ConnectionWriteTimeout`) → a blackholed conn (sleep, IP change, NAT reset)
  is detected in **30–40s**. No TCP_USER_TIMEOUT; OS TCP keepalive irrelevant.
- **QUIC**: `MaxIdleTimeout` is not settable through any-sync's config → quic-go
  default **30s**. Keepalive PINGs (effective 15s) don't reset the idle timer —
  only received packets do. Detection ≈ **30s**. Upside: the UDP socket is
  wildcard-bound, so QUIC *can* survive an IP change via server-side NAT
  rebinding when the path still works.
- **Pool** (`net/pool`): `Get` returns any cached peer whose `IsClosed()` is
  false — i.e. every stale conn is served for the full detection window.
  `Get` prefers *incoming* conns over dialing out. Eviction after close is
  immediate (`evictOnClose`), and `pool.Flush` genuinely closes both directions
  and makes the next `Get` re-dial — it is the right recovery lever.
- **Sync layers**: periodic diff-sync (headsync, 20s/space, 1min round ctx) is
  the self-healing backstop; streampool "syncstream" broadcast is best-effort
  (drops silently on overflow/dial failure) and is re-opened by `KeepAlive` at
  the end of each diff round. `HeadSync.DiffSync` (heart: `SyncAllSpaceHeads`)
  is the out-of-band kick; `pool.Flush` the conn reset.

Recovery signals that exist today:

| Signal | Source | Effect (before this work) |
|---|---|---|
| `AppSetDeviceState(FOREGROUND)` | mobile (and desktop clients that send it) | pool.Flush if backgrounded >15s; SyncAllSpaceHeads if >20s; refresh opened objects |
| `DeviceNetworkStateSet(type)` | mobile | mDNS restart + file-downloader Wi-Fi gate. **No pool flush, no sync kick** |
| — (desktop plain network change / wake without client event) | nobody | nothing — transport timeouts only |

## Prioritized issues

### Critical

- **C1 — Network-type change never resets connections or kicks sync.**
  `core/device/networkstate.go`: `SetNetworkState` only runs the mDNS +
  file-downloader hooks. Wi-Fi↔cellular with the app in foreground rides dead
  conns for 30–40s while the UI shows *Synced* (node status only flips when
  `fetchResponsiblePeers` fails, and it keeps getting the stale conn).
  **Fix:** connectivity-recovery pipeline: flush pool → rebuild responsible
  peers → `SyncAllSpaceHeads`, debounced/suppressed against event bursts.

- **C2 — Desktop has no wake/network-change detection in Go.**
  No OS network monitor exists (`NWPathMonitor`/netlink/route-socket: zero
  hits). Wake helps only if the client app sends `AppSetDeviceState`. A plain
  Wi-Fi switch, VPN toggle, or cable unplug produces no signal at all; if the
  IP stays the same even mDNS isn't rebuilt.
  **Fix:** Go-side net monitor in `core/device`: 5s interface-addr snapshot
  diff (reuses `net/addrs`) + monotonic-vs-wall clock-jump sleep detector →
  same recovery pipeline. Works on all platforms; on Android iface enumeration
  needs the client-injected getter (already exists for localdiscovery), else
  clock-jump + RPC signals still apply. Also catches Wi-Fi→Wi-Fi (same type)
  switches that C1's type-dedupe cannot see.

### High

- **H1 — Dead-conn detection window too long.** yamux 30s ping default, QUIC
  30s idle. **Fix (config-only):** `KeepAlivePeriodSec: 10` for both in
  `core/anytype/config` → yamux detection ≤20s. (QUIC idle stays 30s until
  any-sync exposes `MaxIdleTimeout` — see M1.)

- **H2 — No offline gating; dial churn when the interface is clearly down.**
  `GetNetworkState()` has zero callers. `manageResponsiblePeers` dials every
  20s per space (10s timeouts), the any-sync pool has no failed-dial backoff or
  negative cache → N spaces ⇒ N full-timeout dials / 20s, indefinitely, on a
  dead radio. **Fix:** when offline (reported NOT_CONNECTED **or** monitor sees
  no usable interface) stretch the peer-manager cadence 20s → 2min (probe, not
  halt — self-corrects a wrong offline belief); snap back + rebuild instantly
  on the reconnect hook.

- **H3 — No-deadline blocking waits park sync workers while offline.**
  `BroadcastMessage` swaps in `context.Background()`; `GetResponsiblePeers`
  with no peers blocks forever → each offline broadcast parks one of only 4
  streampool dial workers; ≥4 concurrent broadcasts freeze the entire send
  path until reconnect. `SyncAllSpaceHeads` similarly parks up to 10
  goroutines on `s.ctx`. **Fix:** peer-find deadlines (30s broadcast, 1min
  head-sync) via `ContextPeerFindDeadlineKey`.

- **H4 — filesync node-usage poller never slows down on errors.**
  `core/files/filesync/stats.go`: 10s ticker; only a *successful* idle check
  switches to 1min — errors keep it at 10s forever (offline mid-upload ⇒
  file-node RPC every 10s, indefinitely). **Fix:** enter slow mode on error.

### Medium (any-sync changes, need a version bump to reach heart)

- **M1 — QUIC `MaxIdleTimeout` not configurable** (30s floor for silent-death
  detection). Expose `MaxIdleTimeoutSec` in `net/transport/quic` config.
- **M2 — QUIC dial under-bounded after handshake**: `OpenStreamSync` /
  `HandshakeOutbound` run without a deadline (up to ~30s on a half-open peer);
  yamux double-bounds the equivalent path at 10s+10s.
- **M3 — `pool.Get` prefers a possibly-stale incoming conn** over a fresh
  outgoing dial (`net/pool/pool.go:70-73`). Mitigated by C1/C2 flush.
- **M4 — No failed-dial backoff/negative cache in the pool** (only
  `ErrIncompatibleVersion` is cached, 20min). Mitigated heart-side by H2.
- **M5 — streampool silent drops**: `Send` swallows dial errors, `SendById`
  returns nil when all writes fail, 300-deep queues drop on overflow. Benign
  only because periodic diff-sync re-converges; worth surfacing errors.
- **M6 — nodeconf update loop has no per-call timeout; aclwaiter polls the
  coordinator every 5s with no backoff** while a join is pending.

### Low / notes

- L1 — One wildcard UDP socket per QUIC peer; no shared `quic.Transport` (fd
  growth, no coordinated path handling).
- L2 — zeroconf recv busy-spin after network change: **already fixed** in the
  pinned fork (backoff 50ms→5s present in `v2.2.1-0.20260709…`). The client
  browse goroutine still exits silently on socket errors and is only revived
  by an addr-change rebuild — the 5s periodicCheck + C2 monitor cover it.
- L3 — Fixed-cadence pollers that ignore offline state but are low-rate:
  configfetcher 300s, inbox 30s, identity 5min, push 5min, payments forced
  refresh 10s (deadline-bounded). Candidates for the same offline gate later.
- L4 — First-ever foreground event fires the >15s/>20s branches (zero-time
  `lastDeviceStateChange`) — harmless at startup.

## Fixes shipped in this branch (heart)

1. **Connectivity recovery pipeline** — `core/device`: one entry point used by
   all signals (foreground resume, network-type RPC, interface change, sleep
   wake): `pool.Flush` → connectivity hooks (peer-manager rebuild) →
   `SyncAllSpaceHeads` (skipped when offline). Leading-edge execution with a 5s
   suppression window; a suppressed event schedules one trailing recovery.
2. **Net monitor** — `core/device/netmonitor.go`: 5s tick; interface-addr
   snapshot diff + clock-jump (>30s wall-vs-mono drift ⇒ wake) + link-down
   tracking for `IsOffline()`.
3. **Offline-aware peer manager** — `space/spacecore/peermanager`: rebuild
   signal on every connectivity event; 2min dial cadence while offline.
4. **Deadlines** — broadcast peer-find 30s; `SyncAllSpaceHeads` peer-find 1min.
5. **Keepalive tuning** — yamux/QUIC `KeepAlivePeriodSec: 10`.
6. **filesync stats poller** — slow mode on error.
7. **Mobile integration guide** — `docs/mobile-network-integration.md`
   (NWPathMonitor / ConnectivityManager wiring, exact call sequences).

## Follow-ups (any-sync)

M1+M2 are implemented as a patch branch in the any-sync repo (see
`docs/mobile-network-integration.md` appendix note); M3–M6 are documented
here for scheduling. After an any-sync release, set
`MaxIdleTimeoutSec: 20` in `GetQuic()`.

## Expected outcomes

| Scenario | Before | After |
|---|---|---|
| MacBook wake (client sends foreground) | ≤10s flush+sync (was already OK) | same, minus double-flush thrash |
| MacBook wake (no client event) | 30–40s+ (transport timeouts) | ≤5s (clock-jump) + dial |
| Desktop Wi-Fi/VPN switch | up to 40s, mDNS may never rebuild | ≤5s (addr diff) + dial |
| Mobile Wi-Fi↔cellular, foreground | 30–40s, UI shows Synced | ~1s (RPC hook) + dial |
| Mobile Wi-Fi→Wi-Fi (same type) | 30–40s | ~1s with `networkId` in the RPC; ≤5s via addr diff otherwise |
| Silent midstream conn death | 30–40s | ≤20s (yamux); 30s QUIC until M1 |
| Offline dial burn (N spaces) | N dials/20s forever | N dials/2min, instant snap-back |
