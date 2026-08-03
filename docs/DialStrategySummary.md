# Peer dial rework — summary

*A shareable digest of the research in `DialStrategyRework.md` (full design) and
`DialStrategyReview.md` (three adversarial review rounds). Read those for detail.*

## TL;DR

Our peer-dial path fails badly on exactly the networks users can't control:
firewalls that silently drop UDP, cellular/slow links, and networks that kill a
connection *after* it's established. The root cause is structural — we walk a
peer's addresses **one at a time**, a dead UDP address burns a full ~10-20s
handshake timeout, and any timeout we add on top just truncates the walk (that's
what caused the GO-7409 sync-loss regression). We also **forget every failure
instantly** and **never learn that a "successful" dial died 15s later**.

The fix is a shift in strategy, borrowed from go-libp2p's production design:
**dial concurrently instead of sequentially** (first success wins, a black hole
costs ~250ms instead of 10s), **keep fail-fast evidence outside quic-go** (probe
dead ports, track UDP black holes statistically), and **react to network changes
we already get signalled about**. Node sync and local (P2P/LAN) sync are kept
strictly separate so one never blocks the other.

A bonus from the review: a likely **root cause for the original self-hosted
incident** that no firewall repro reproduced — a server misconfig
(`dialTimeoutSec` unset) makes the server complete the handshake and then hang
silently, looking exactly like a black hole. Worth chasing independently.

---

## The problems we have today

**1. A dead UDP port costs a full handshake timeout.**
any-sync dials QUIC on an *unconnected* UDP socket, so the OS never delivers the
ICMP "port unreachable" back to us, and quic-go just retransmits handshake
packets until it times out (~10-20s). A firewall silently dropping UDP looks
identical. Every doomed QUIC address = one full timeout.

**2. We dial addresses one at a time.**
`peerService.Dial` walks every scheme × every address sequentially; `GetOneOf`
walks the responsible nodes sequentially on top of that. One black-holing
address delays everything behind it. Prod nodes advertise UDP :5430 first, so on
a UDP-blocked network the client burns the QUIC timeout before it ever tries TCP.

**3. Any timeout we add truncates the walk.**
This is the GO-7409 regression: a single budget over a sequential walk gets fully
consumed by the first stale address, and every address behind it then fails
instantly with "deadline exceeded" — including the healthy TCP fallback. That
cost self-hosted users their sync entirely. Budgets are the wrong tool for a
sequential walk.

**4. We forget every failure instantly.**
There is no dial backoff or failure memory anywhere in the net stack (one narrow
20-minute exception for version mismatches). The next cycle re-dials the same
dead address from scratch, every 20 seconds, forever.

**5. A "successful" dial that dies young is invisible.**
Corporate DPI / hotel firewalls let the handshake through, then kill the flow. We
score that as success, so nothing above the transport learns anything — the
connection dies at the ~30s idle timeout, we redial the same way, and sync
flaps ~15s-alive / ~30s-dead permanently.

**6. Offline detection is slow.**
Node status only flips to "error" when the whole walk *returns*, which on a full
sequential walk is tens of seconds. That lags the sync-status UI and the
fast-retry cadence.

**7. Offline app-start can freeze every RPC.**
On the space-load path, waiting for responsible peers is unbounded, and
`AccountSelect` holds a global lock for its whole duration — so an offline user
opening the app can freeze the entire middleware, not just sync.

**8. Stale LAN peers pollute the node path after a network switch.**
Local (mDNS) peers discovered on Wi-Fi stay in the peer store after a switch to
cellular, and nothing invalidates them on network change — even though mobile
already tells us the network changed. They cost dials every cycle, and the "P2P
devices" UI counter counts stale store entries, not live connections.

**9. (Review finding) A server misconfig mimics a black hole.**
A self-hosted node with QUIC listening but `dialTimeoutSec` unset completes the
QUIC/TLS handshake and then never answers the stream — client hangs forever,
server leaks the connection. Indistinguishable from a firewall from the client
side, and no firewall repro reproduces it. This may be the real cause of the
original incident.

---

## The direction

Four principles, then the concrete moves:

- **Concurrency over budgets** — make the walk concurrent and self-bounding, so
  bounds become "windows," not truncation hazards.
- **Fail-fast evidence lives outside quic-go** — probe dead ports before dialing;
  track UDP black holes statistically above the transport. quic-go can't be
  trusted to react to ICMP.
- **Nodes and local peers never share fate** — separate dial loops, separate
  failure state, keyed by *where a peer came from* (node config vs mDNS), not by
  IP class. Local peers speed sync up; they must never delay node connectivity.
- **Network changes are events, not discoveries** — mobile already signals them;
  tag peers with the network they were seen on, invalidate instantly on change.

Concretely:

| Move | What it buys |
|---|---|
| **Ranked, staggered, concurrent dial** (go-libp2p model) | A black hole costs ~250ms, not 10s; first success wins; offline verdict in ~12s not tens of seconds |
| **UDP black-hole detector** | After a few silent-drop failures, skip QUIC and go straight to TCP until UDP proves itself again — catches what a port probe can't |
| **Dead-port probe** (already built for LAN in GO-7421, generalized) | Instant fail on provably-dead ports instead of a full timeout |
| **Per-attempt timeouts inside the transports** | The correctness floor; also fixes the server misconfig hang (problem 9) |
| **Connection-liveness feedback** | Early death feeds back into dial policy, closing the post-handshake-blocking flap (problem 5) |
| **Network-epoch tags + invalidation** | Stale LAN peers vanish on network change instead of costing dials for up to an hour |
| **Bounded space-load wait** | Offline app-start no longer freezes every RPC (problem 7) |

## Status & next steps

- Design is complete and has survived **three adversarial review rounds** (five
  network-condition lenses, plus two follow-up passes). No architectural
  reversal; the reviews hardened the details (security on hostile LANs,
  mobile battery cost, observability, rollout safety).
- **Sequencing**: the fixes land in any-sync on the 0.12 line (so heart can
  consume them as v0.12.20 **without** the separate 0.13 protocol migration),
  plus heart-side items (network tags, space-load bound) that ship independently
  now. Full PR breakdown in §5 of the design doc.
- **Two things to do before trusting any numbers**: (1) chase the server-misconfig
  root-cause lead — get an affected self-hoster's node config and repro with
  `dialTimeoutSec` deleted; (2) rebuild the e2e dial test harness with
  interleaved A/B arms (the original incident's repro didn't survive scrutiny).

*Reference: go-libp2p smart-dialing (dial ranker, black-hole detector, dial
backoff) is the closest production-proven design for this exact problem shape.*
