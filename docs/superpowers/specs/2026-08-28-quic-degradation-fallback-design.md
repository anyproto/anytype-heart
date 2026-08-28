# QUIC Degradation Fallback (GO-7467)

## Problem

Some DPI middleboxes allow the first packets of a QUIC connection (the
handshake completes) and then silently drop the rest of the flow. The
connection dies ~30–60 s later when keepalives stop being answered
(`quic.IdleTimeoutError`). Every redial opens a fresh UDP socket
(`net.ListenUDP` port 0 in `quic.Dial`), so the new 4-tuple slips past the
DPI again, handshakes fine, and dies again — an endless
connect → stall → timeout → redial loop. Sync never makes progress over QUIC
while a working TCP/yamux path exists one scheme lower in the dial order.

A second, related wedge: networks that drop UDP entirely make every QUIC dial
burn the full `HandshakeIdleTimeout` before yamux is tried (the GO-7410
selfhosted case).

## Detection signals

No qlog tracer or packet stats are needed. Three signals identify the
degradation, two of which already exist:

1. **Close cause.** quic-go cancels the connection context with the close
   error (`context.WithCancelCause`), so when the conn dies,
   `context.Cause(conn.Context())` yields the reason. `*quic.IdleTimeoutError`
   on a connection with keepalives enabled means the path went black — a
   healthy path always has ACK traffic within the 30 s idle window because
   keepalive PINGs are sent every 25 s.
2. **Connection lifetime.** Degraded conns die within ~1 min of establishment.
   An idle-timeout death after hours is a lid close or network change, not DPI.
3. **Bytes moved.** `quicMultiConn` already tracks `BytesRead`/`BytesWritten`
   (diagnostic value in logs; not used by the trigger).

The loop itself is the confirmation: **N consecutive short-lived idle-timeout
deaths** has no other cause — total network loss kills yamux too and is
reported by heart's netmonitor, whose events reset the counters.

Additionally, a QUIC dial failing with quic-go's own
`*quic.HandshakeTimeoutError` (not a caller context cancellation — caller
deadlines are ambiguous) counts as the same degraded event, covering the
full-UDP-block case.

Only **outbound** (dialed) connections feed the mechanism; an inbound conn
dying says nothing about our dial path.

## Architecture

Three parts: event source in the any-sync QUIC transport, penalty state
machine in any-sync peerservice, persistence + reset wiring in anytype-heart.

### 1. any-sync `net/transport/quic` — conn death classification

- `quicMultiConn` records `startTime`.
- `Dial` (outbound only) spawns a per-conn watcher: on `CloseChan()`, read
  `context.Cause`, classify, and report to an observer registered via
  `SetConnObserver` (same pattern as `SetAccepter`). Goroutines are bounded by
  live outbound conn count and exit when the conn closes.
- Classification (`transport.ConnCloseEvent`):
  - `Degraded`: cause is `*quic.IdleTimeoutError` **and**
    lifetime < `degradedMaxLifetime` (5 min).
  - `Healthy`: lifetime ≥ 5 min (whatever the close cause — DPI kills in
    seconds-to-a-minute, so surviving 5 min proves the path).
  - `Neutral`: everything else (local close, graceful remote close,
    short-lived non-idle errors). No signal.
- Event carries `PeerId` (from the handshake ctx), `Kind`, `Lifetime`,
  `BytesRead`, `BytesWritten`, `Cause` (for logging).

### 2. any-sync `net/peerservice` — penalty state machine

Per-peer state under the existing mutex:
`{consecutiveDegraded int, demotedUntil time.Time, backoffLevel int}`.

- **Degraded event** (conn death or handshake-timeout dial failure, which
  peerservice sees directly in `dialScheme`): `consecutiveDegraded++`. At
  **2**, demote the peer: `demotedUntil = now + 30min × 2^backoffLevel`
  (capped at 4 h), `backoffLevel++`.
- **Healthy event**: fully reset the peer's state.
- **Dial ordering**: `preferredSchemes()` becomes per-peer. A demoted peer
  (or global escalation, below) gets yamux before QUIC. QUIC is never removed
  from the list — some networks block TCP instead.
- **TTL expiry**: the next dial tries QUIC first again (one shot).
  `consecutiveDegraded` is left at threshold−1, so a single further degraded
  death re-demotes immediately at the next backoff level. Only a Healthy
  outcome clears the memory.
- **Global escalation** (derived, not stored): when ≥2 distinct currently
  demoted peers are **nodeconf peers** (known network nodes), all dials go
  yamux-first. LAN p2p peers are excluded from the count — a sleeping phone
  also produces short-lived idle timeouts — but still get per-peer demotion.
- New API:
  - `TransportPenalties() PenaltySnapshot` — per-peer map for persistence.
  - `SeedTransportPenalties(PenaltySnapshot)` — applied at startup.
  - `ResetTransportPenalties()` — clears all state.
  - `SetPenaltyObserver(func())` — fires on demote/restore/reset transitions
    so heart can persist (heart debounces).

### 3. anytype-heart — persistence and reset wiring

**Persistence** (mobile eviction is frequent; re-learning on every app open
costs ~60–90 s of dead QUIC per network):

- File: `<accountRepo>/transport_penalties.json` — runtime state, deliberately
  not in the user-editable `config.json`:

  ```json
  {
    "networkKey": "<netmonitor connectivity snapshot key>",
    "updatedAt": "2026-08-28T10:00:00Z",
    "peers": {
      "<peerId>": {
        "consecutiveDegraded": 2,
        "demotedUntil": "2026-08-28T11:00:00Z",
        "backoffLevel": 1
      }
    }
  }
  ```

- A small heart component (`net/transportpenalty`, registered in bootstrap
  after peerservice):
  - `Init`: load the file; if any `demotedUntil` is in the future, seed
    peerservice (Init runs before any dials).
  - Subscribes to the penalty observer; debounced (~1 s) rewrite of the file
    together with the current netmonitor network key.
  - When netmonitor's first snapshot arrives: if its key differs from the
    stored `networkKey`, call `ResetTransportPenalties()` and clear the file.
    An empty/unknown stored key keeps the seed (fail open — worst case is one
    yamux-first session on a good network, which still works).
- `demotedUntil` is wall-clock; clock skew is benign (worst case QUIC is
  retried early).

**Reset triggers**: netmonitor's existing connectivity-change events
(interface addresses lost/regained, wake from sleep) call
`ResetTransportPenalties()` via the existing recovery pipeline. Wake-based
reset also prevents sleep-killed conns from counting toward demotion (they
are usually Healthy-by-lifetime anyway; the 2-strike threshold covers the
rest).

**Config compat**:

- `PeferYamuxTransport=true` (existing knob) still forces yamux-first
  globally; auto-demotion is then moot but harmless.
- Auto-demotion is **on by default**. Escape hatch for debugging:
  `ANYTYPE_QUIC_AUTO_DEMOTION=0` env var skips observer registration and
  seeding.

**Observability**: Info-level log on demote / escalate / restore / reset with
peerId, cause, lifetime, bytes; counters exposed via debugstat.

## Constants

| Constant | Value | Rationale |
|---|---|---|
| `degradedMaxLifetime` / healthy threshold | 5 min | degraded conns die in 30–60 s (25 s keepalive, 30 s idle timeout); sleep/network-change deaths are older |
| demote threshold | 2 consecutive | one idle timeout can be bad luck; two in a row on fresh 4-tuples is the DPI signature |
| base demotion TTL | 30 min | cheap to be wrong (one QUIC probe), avoids flapping |
| TTL backoff | ×2, cap 4 h | persistent DPI converges to rare probes |
| global escalation | ≥2 demoted nodeconf peers | DPI is a network property; nodes share infrastructure |

## Edge cases

- **Caller-cancelled dials** (short caller deadlines, GO-7410 dial budget):
  `context.DeadlineExceeded`/`Canceled` from the caller ctx does **not**
  count — only quic-go's own `HandshakeTimeoutError`.
- **Server side**: any-sync nodes also run peerservice. The observer only
  watches dialed conns; node-to-node demotion is harmless (yamux works) and
  nodes don't register the heart persistence component.
- **Local peers** (GO-7424): local discovery publishes yamux-only addrs, so
  the mechanism doesn't touch the LAN path.
- **webtransport**: stays last in the scheme order, unaffected.

## Testing

- **Transport**: table tests for classification (idle timeout young/old,
  app close, local close, `net.ErrClosed`); watcher fires the observer with
  correct fields (existing `mock_quic` + `conn_idle_test.go` patterns).
- **peerservice**: state machine — 2-strike demote, Healthy reset, TTL expiry
  one-shot + immediate re-demote with doubled TTL, cap, global escalation
  only counting nodeconf peers, seed/snapshot round-trip, reset; per-peer
  `Dial` scheme ordering including handshake-timeout dial failures.
- **heart**: persistence component — load/seed on Init, debounced save on
  observer fire, reset+clear on network-key mismatch; config compat
  (`PeferYamuxTransport` still forces yamux); env escape hatch.

## Out of scope / follow-ups

- **QUICv2 pinning**: measured TSPU parses only QUICv1 Initials; pinning v2
  would dodge SNI-based drops entirely (not flow-duration policing, so this
  mechanism stays needed regardless).
- Smarter stats (qlog tracer loss/RTT telemetry) — only if field data shows
  the trigger needs tuning.
- Coordinated per-network hints across devices — not worth the complexity.
