# Local peers: yamux (TCP) instead of QUIC

**Decision (2026-08-03):** drop the UDP liveness probe approach (GO-7421) in
favour of dialing yamux (TCP) for loopback/private addresses.

**Revision (2026-08-04):** local peers are now dialed over **yamux only**,
rather than "yamux first, QUIC fallback". The original plan needed a
per-address scheme-ordering change inside any-sync (P3 below) because the dial
loop orders schemes globally; advertising a single scheme makes that change
unnecessary — the existing loop finds no `quic://` candidates for local peers
and goes straight to TCP. The compatibility cost is softened by keeping the
QUIC listener bound (see "Version skew").

**Status: implemented** — GO-7424, heart branch `go-7424-prefer-yamux-local`.
No any-sync change required. GO-7421 canceled; its probe branch
`go-7421-udp-probe-local-peers` stays parked, salvageable pieces noted under
GO-7410.

Rationale for the transport choice is in
[LocalTransportYamuxVsQuic.md](./LocalTransportYamuxVsQuic.md).

---

## Why not just sorting logic

**There was no TCP listener on the client.** Preference ordering alone changes
nothing, because nothing is listening on the other end.

| Fact | Where |
|---|---|
| Client listened on **UDP only** | `clientserver.go` (`s.quic.ListenAddrs`) |
| yamux `ListenAddrs` is empty → `Run` binds nothing | `config.go:632`, any-sync `yamux.go` |
| A TCP listener was created only to *pick a free port*, then closed | `clientserver.prepareListener` |
| mDNS advertises **no scheme** — instance=peerId, IPs, one port, `nil` TXT | `localdiscovery.go:300` |
| Discovery rebuilds bare `ip:port` | `localdiscovery.go:341` |
| Scheme is prefixed **unilaterally by the dialer's heart**, never negotiated | `peer.go` `addSchema` |
| Scheme order is **global**, not per-address | any-sync `peerservice.go` |
| `PreferQuic(true)` unless `PeferYamuxTransport` | `config.go:322` |

Because mDNS carries no transport information, the wire format does not
change.

## What was implemented

### P1 — bind a yamux listener on the same port (`clientserver.go`)

`clientserver.prepareListener` already created exactly the TCP listener we
need — to pick a free port — and threw it away. It is now kept bound and
handed to any-sync via `yamux.AddListener`; QUIC binds UDP on the same port as
before.

This also removed the latent bug where a free **TCP** port was used to choose
a **UDP** bind: if UDP on that port was occupied, `startServer` failed and the
app silently came up with no local P2P server at all. Now a UDP bind failure
drops the TCP reservation and retries with fresh ephemeral ports
(`listenAttempts`).

**Ordering constraint (important).** `AddListener` both appends to
`y.listeners` *and* starts `acceptLoop` immediately. `yamux.Run` then starts
an `acceptLoop` for **every** entry in `y.listeners`. So calling `AddListener`
before `yamux.Run` yields **two accept loops racing on one listener**.
Registration order saves us — yamux is registered before clientserver in
`bootstrap.go`, so `clientserver.Run` happens after `yamux.Run`. Do not
reorder those registrations, and do not move the `AddListener` call into
`Init`.

### P2 — advertise yamux only for local peers (`peer.go`)

`addSchema` now emits `yamux://<ip>:<port>` (was `quic://`). Both discovery
directions go through it: `PeerDiscovered` and the `SpaceExchange` handler.

### P3 — per-address scheme ordering in any-sync: dropped for local peers, **revived for config addrs**

The original plan advertised both schemes and needed any-sync's
`peerService.Dial` to order schemes per address (private → yamux first),
because the loop iterates schemes on the outside from one global `preferQuic`
bool — with both schemes advertised it would try *every* QUIC address before
*any* yamux address. Advertising one scheme sidesteps the whole problem for
local peers, along with the any-sync release dependency.

But the per-address ordering still pays off for **network-config peers with
private addresses** — self-hosted nodes on LAN/docker-compose, whose addrs
come from `nodeConf.PeerAddresses` (both schemes listed, global QUIC-first
order, so an unreachable node still burns the QUIC handshake timeout before
the yamux fallback). Implemented standalone on any-sync branch
`dial/yamux-local-addrs` (off main), in simplified "sorter" form rather than
loop restructuring:

- `preferredSchemes()` is parameterized; the per-address order is just
  `preferredSchemes(preferQuic && !isLocal(addr))`, and `Dial` walks a flat,
  stably-sorted candidate list. With `preferQuic` unset (all servers) the
  order is yamux-first either way and `isLocal` is short-circuited away — no
  behaviour change and no resolution on nodes.
- **Hostnames** (docker-compose service names, `.lan`/hosts-file names in
  self-hosted configs) are classified by resolving with a 200ms deadline plus
  a 1-minute verdict cache: locally answered names (hosts file — cached
  in-memory by Go's resolver, docker's 127.0.0.11 embedded DNS, mDNS cache)
  reply within the budget; anything needing upstream recursion is treated as
  non-local. A wrong verdict only affects ordering — every address is still
  dialed. Both transports already resolve hostnames at dial time (yamux via
  `net.Dialer`, quic via `net.ResolveUDPAddr`), so this adds no new
  requirement.

This is **not a dependency** of the heart branch: local (mDNS) peers are
yamux-only regardless. It lands with a regular any-sync release and needs no
heart-side wiring.

## Why this fixes the dial hangs

A dead local peer answers a TCP SYN with **RST → `ECONNREFUSED` in one RTT**,
generated by the peer's TCP stack. No ICMP dependency, no Windows
`SIO_UDP_CONNRESET` problem, identical on every platform. The existing peer
manager path then works unchanged: `pool.Get` fails fast, `RemoveLocalPeer`
evicts the stale entry (`peermanager/manager.go`).

Compare QUIC before: an **unconnected** UDP socket per dial (any-sync
`quic.go`) gives the kernel nothing to attach the ICMP port-unreachable to,
quic-go v0.60.0 has zero ECONNREFUSED handling, so it retransmits Initials
until `HandshakeIdleTimeout` (10s). Only the responsible-peers path caps that
locally (`localPeerDialTimeout`, 5s); the `PeerDiscovered` dial runs on the
component context with no deadline, so it was and is bounded only by the
transport's own timeout.

What it does **not** fix: a peer whose host is off, asleep, or on a network we
can no longer route to. No RST comes back either. ARP failure usually surfaces
`EHOSTUNREACH` before the full timeout, which is better than silence, but this
case stays bounded by the dial timeout. See GO-7410.

## Version skew

Accepted trade-off (decided 2026-08-04): a strict yamux-only client cannot
dial a not-yet-updated (UDP-only) client. In practice the break is narrow:

- **Old → new**: old clients still prefix `quic://` on their side, and the new
  client **keeps its QUIC listener bound on the same port** — the old side
  connects as before.
- **New → old**: the yamux dial fails with an instant RST (cost ≈ one RTT).
  But mDNS discovery is symmetric — both sides discover and dial each other —
  so the pair still ends up connected through the old side's QUIC dial, and
  that connection is bidirectional.
- The only truly broken case is an old peer that accepts but never dials; that
  path does not exist (`PeerDiscovered` always dials).

Follow-up for a later release, once the fleet has largely updated: drop the
client's QUIC listener (and this transition note).

## Ruled out — do not re-derive these

**`net.DialUDP` in any-sync's `quicTransport.Dial`.** Earlier framed as "the
root fix"; it does not work. quic-go always writes with an explicit
destination (`WriteTo` in `sys_conn.go`, `WriteMsgUDP` in `oobConn`), and Go
rejects both on a connected socket with `ErrWriteToConnected`. The first
packet fails. Making it work requires a change *inside quic-go*, not a
one-liner in any-sync.

**Why the socket is unconnected at all** — it is forced, not chosen: quic-go's
API requires `net.PacketConn`; QUIC demultiplexes by connection ID rather than
4-tuple so it must accept packets from changing addresses (migration, NAT
rebinding); and the `OOBCapablePacketConn` fast path (DF bit for path MTU
discovery, ECN, `recvmmsg`, GSO on Linux) rides on the sendmsg-with-address
path.

**Global `PeferYamuxTransport`** — affects node connections too; it stays the
"client has problems with QUIC" escape hatch.

## What happened to GO-7421

Branch `go-7421-udp-probe-local-peers`, commit `c396cb040` — the `net/udpprobe`
package plus peer-manager wiring. **Superseded**: a real TCP dial replaces the
heuristic (no TOCTOU window, no "inconclusive" bucket, no Windows `WSAIoctl`,
and it is a connection attempt rather than a guess). Two pieces are worth
salvaging under GO-7410, independent of the probe:

1. `pool.Pick` before dialing, so an existing live connection skips the dial
   entirely.
2. `peerstore.SetLocalPeerAddrs` / `LocalPeerAddrs` — a heart-side copy of each
   local peer's raw `ip:port`, since any-sync's `peerService` keeps addrs
   behind a write-only interface.

## Risks / open questions

- **Firewall prompt.** Keeping a TCP listener bound may trigger a fresh
  Windows/macOS firewall dialog. Previously the listener existed only
  momentarily. On Windows, an install approved via the standard firewall
  prompt gets program-scoped rules covering both protocols; installs where
  inbound TCP is blocked fall back to the other side dialing out (and to node
  sync).
- **Symmetric inbound-TCP block loses local sync entirely** (review finding,
  accepted). yamux-only means no UDP fallback between two updated clients: if
  both sides drop inbound SYNs (e.g. UDP-scoped program rules created back
  when the app had no TCP listener), neither direction connects, and a
  LocalOnly/self-hosted setup with no reachable node stops syncing. The old
  QUIC path would have worked there. Re-adding `quic://` as a second local
  candidate must wait for the any-sync per-address ordering to ship (with the
  pinned v0.12.16, a second scheme would put QUIC first again — the exact hang
  being fixed).
- **iOS Local Network permission** — not expected to be a blocker;
  `selfconnect.go`'s existing restriction probe is already TCP.
- **Mobile cost** — one extra listener plus accept loop per client.
- **Unmeasured.** Perf claims in the companion doc are third-party plus two
  locally verified constants (`lo0` MTU 16384; quic-go's 1452 datagram cap at
  `internal/protocol/protocol.go:114`). A yamux-vs-quic benchmark over
  loopback with a realistic sync payload has **not** been run.
