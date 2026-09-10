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
  `SetConnObserver` (same pattern as `SetAccepter`; peerservice registers
  itself when `EnableQuicDemotion` is called). Goroutines are bounded by
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
- **Opt-in**: the whole mechanism is off until `EnableQuicDemotion()` is
  called (heart calls it at config init, where `PreferQuic(true)` already
  lives). Server nodes never call it, so node dial behavior is unchanged by
  the any-sync release.
- New API:
  - `EnableQuicDemotion()` — turns tracking on and registers the conn
    observer on the QUIC transport.
  - `TransportPenalties() PenaltySnapshot` — per-peer map for persistence.
  - `SeedTransportPenalties(PenaltySnapshot)` — applied at startup.
  - `ResetTransportPenalties()` — clears all state.
  - `SetPenaltyObserver(func())` — fires on every state mutation so heart
    can persist (heart debounces).

### 3. anytype-heart — persistence and reset wiring

**Persistence** (mobile eviction is frequent; re-learning on every app open
costs ~60–90 s of dead QUIC per network):

- File: `<accountRepo>/transport_penalties.json` — runtime state, deliberately
  not in the user-editable `config.json`:

  ```json
  {
    "networkKey": "<device.NetworkState network identity>",
    "updatedAt": "2026-08-28T10:00:00Z",
    "penalties": {
      "peers": {
        "<peerId>": {
          "consecutiveDegraded": 1,
          "demotedUntil": "2026-08-28T11:00:00Z",
          "backoffLevel": 1
        }
      }
    }
  }
  ```

- **Network identity**: `device.NetworkState` gains `NetworkIdentity()` — a
  composite of the client-reported network type + path id (mobile) and the
  net monitor's interface snapshot (desktop). Equal keys mean "still the same
  network".
- A small heart component (`net/transportpenalty`, registered in bootstrap
  after peerservice and device):
  - `Init`: load the file; if it has any peers, seed peerservice (strike
    memory is worth keeping even when the demotion TTL has expired). Init
    runs before any dials.
  - Subscribes to the penalty observer; debounced (1 s) rewrite of the file
    together with the current network identity. An emptied state removes the
    file.
  - **Identity check**: on the connectivity-recovery hook and once ~3 s after
    start (a stable network never fires a recovery). First observation is
    compared against the stored `networkKey`: mismatch → reset + clear file.
    An empty/unknown stored key keeps the seed (fail open — worst case is one
    yamux-first session on a good network, which still works). A later
    identity change (device moved networks mid-session) → reset.
- `demotedUntil` is wall-clock; clock skew is benign (worst case QUIC is
  retried early).

**Reset triggers**: the identity check above rides `RegisterConnectivityHook`
(fired on network switch, interface change, wake, foreground resume — after
the pool flush). Resets happen only when the identity actually changed, so a
foreground resume or wake on the same network keeps the learned verdict —
that is what makes persistence useful on mobile. Sleep-killed conns rarely
count toward demotion anyway (usually Healthy-by-lifetime; the 2-strike
threshold covers the rest).

**Config compat**:

- `PeferYamuxTransport=true` (existing knob) still forces yamux-first
  globally; `EnableQuicDemotion` is then not called (moot).
- Auto-demotion is **on by default** for clients: config calls
  `EnableQuicDemotion()` next to `PreferQuic(true)`. Escape hatch for
  debugging: `ANYTYPE_QUIC_AUTO_DEMOTION=0` skips both the enable call and
  the persistence component's seeding/saving.

**Observability**: Info-level log on degraded deaths (peerId, cause,
lifetime, bytes, whether it demoted), demotion by handshake timeout, resets,
and seed/persist events.

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
