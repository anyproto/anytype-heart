# LAN IPv6 link-local on interfaces without IPv4

Date: 2026-10-09. Ticket: GO-7581. Status: draft.
Builds on GO-7576 (native discovery bridge), GO-7577 (iOS listener lifecycle) and GO-7580 (own-address selection), all merged in #3317 and #3319.

## Problem

A phone and a Mac joined only by a cable (Wi-Fi off) cannot sync over LAN. The link between them carries IPv6 link-local addresses and nothing else, and heart is IPv4-only end to end.

Observed on 2026-10-09 (iPhone 17 Pro over USB-C/Thunderbolt to a Mac, Wi-Fi and Bluetooth off on the phone):

| Check (on the Mac) | Result |
|---|---|
| `ifconfig en16` (the cable) | `inet6 fe80::a886:c6ff:febd:f923%en16`, **no IPv4** |
| `dns-sd -B _anytype._tcp` | the iPhone's service appears on interface 31 = `en16` |
| `dns-sd -L` / `-G v4v6` for it | `iPhoneRQ.local.:62654`; on `en16` only `FE80::A886:C6FF:FEBD:F9DC`, the A query answers `0.0.0.0 No Such Record` |
| `ping6 fe80::…f9dc%en16` | 2/2, ~4 ms |
| TCP connect to `[fe80::…f9dc%en16]:62654` | **succeeds**: heart's listener (`:port`) already accepts IPv6 |
| the Mac's own anytype service | advertised on `en0`, `bridge100`, `bridge101`, **never on `en16`** |

So the iPhone side already works: iOS advertises the service with its link-local AAAA on the cable, and the port is reachable. What fails is in heart and in the zeroconf fork:

1. `net/addrs.(*NetInterfaceWithAddrCache).GetAddr` filters out IPv6, so an interface with only IPv6 looks empty, and `filterMulticastInterfaces` drops it (it requires `len(GetAddr()) > 0`). Desktop zeroconf therefore never advertises or browses on `en16`.
2. Desktop zeroconf runs IPv4-only: `zeroconf.ServerSelectIPTraffic(zeroconf.IPv4)` and `zeroconf.SelectIPTraffic(zeroconf.IPv4)` in `localdiscovery.go`, and `readAnswers` reads `entry.AddrIPv4` only.
3. The fork loses the receiving interface. `ServiceEntry.AddrIPv6` is a bare `[]net.IP`, and `client.recv` discards the `ipv6.ControlMessage` (`n, _, src, err = pConn.ReadFrom(b)`). A link-local address without its zone cannot be dialed.
4. The fork's proxy mode sends one fixed address set on every interface. `RegisterProxy(…, ips, …, ifaces)` stores the IPs in the service entry, and `appendAddrs` only falls back to per-interface addresses (`addrsForInterface(ifIndex)`) when that list is empty. A link-local address announced on the wrong interface is useless to the receiver.
5. `spacecore.recordLocalPeer` parses the inbound connection's address with `url.Parse`, which rejects a zone (`parse "yamux://[fe80::1%en16]:5": invalid URL escape "%en"`; verified). The address the peer connected from, the one address that is guaranteed to work, is silently dropped for a link-local peer.

What already handles link-local correctly, verified in code: the bridge builds addresses with `net.JoinHostPort` (`clientlibrary/service/discovery.go`), so `fe80::1%en2` becomes `[fe80::1%en2]:port`; any-sync's `peerService.dialAddr` strips the scheme as a string and the yamux transport hands the rest to `net.Dial`, which resolves the zone; `peer.CtxPeerAddr` for an inbound yamux connection is `"yamux://" + RemoteAddr().String()`, which includes the zone.

## Goals

1. An iPhone and a Mac joined only by a cable discover each other, exchange spaces and sync, with no Swift or Kotlin change (phase 1).
2. Address lists stay short: IPv6 appears only for an interface that has **no routable IPv4** (a self-assigned 169.254/16 address doesn't count, §5), and only its link-local address.
3. Nothing changes for peers on a network with IPv4: same addresses, same order, same dial cost.

## Non-goals

- Global and ULA IPv6 (`2a0d:…`, `fdf0:…`): they rotate and there are several per interface. An IPv6-only LAN needs none of them: its interfaces have no IPv4, so the link-local rule applies to them, and mDNS (`ff02::fb`) never leaves the link, so every peer it finds is reachable over link-local. They would matter only for peers on another subnet, which mDNS cannot find either.
- IPv6 in `LocalServer.Ips`. A zone names the *receiver's* interface, which the sender cannot know; the receiver gets the working address from the inbound connection instead (see Design §4).
- IPv6 on interfaces that also have routable IPv4.
- Phase 2 (below) in this ticket's first PR.

## Design

### 1. Own addresses: keep IPv6-only interfaces (heart)

- `net/addrs`: keep `GetAddr` IPv4-only (every existing caller relies on it). Add a sibling that returns the interface's IPv6 link-local addresses (`ip.IsLinkLocalUnicast() && ip.To4() == nil`), cached the same way. On Android, `interface_android.go` already receives every address from the app's `InterfaceGetter`; keep IPv6 there for the sibling instead of nothing.
- `localdiscovery.filterMulticastInterfaces`: keep an interface that is up, multicast, not loopback and has either an IPv4 address or an IPv6 link-local address.
- `localdiscovery/ownaddr.go`: extend `ownAddr` with the link-local of every interface that has no routable IPv4. It is never `advertised` (stays out of `OwnAddresses.Addrs`) and never `matchable` (stays out of `OwnAddresses.Nets`, so it can't reach `LocalServer`). It is used only for the mDNS records in §2. `logOwnAddresses` prints it as `link-local`.

### 2. Desktop zeroconf: advertise and browse on the link (zeroconf fork + heart)

Fork (`github.com/anyproto/zeroconf/v2`, replaced in `go.mod`):

- **Advertise per interface.** Add a server option, e.g. `ServerLinkLocalIPv6()`: when answering on `ifIndex`, `appendAddrs` adds that interface's own IPv6 link-local addresses (from `addrsForInterface`), and only those, on top of the fixed proxy list. The fixed list keeps carrying IPv4 as today. Never announce one interface's link-local on another interface.
- **Browse with the receiving interface.** In `client.recv`, keep the control message: `n, cm, src, err = pConn.ReadFrom(b)` for the IPv6 conn (and enable `ipv6.FlagInterface` with `SetControlMessage` where the platform allows it; on Windows control messages are unavailable, see Risks). Carry `cm.IfIndex` with the parsed message to the entry and record each AAAA as a zoned address. Proposed shape: a new `ServiceEntry.AddrIPv6Zoned []netip.Addr` (`netip.Addr.WithZone`), keeping `AddrIPv6 []net.IP` untouched for compatibility. The zone is the receiving interface's decimal index (see §5).
- The fork already joins the IPv6 multicast group (`joinUdp6Multicast`) and sends AAAA records; nothing else is needed there.

Heart (`space/spacecore/localdiscovery/localdiscovery.go`):

- Run server and browse on `zeroconf.IPv4AndIPv6` only while a link-local-only interface exists, and pass the new server option.
- `readAnswers`: report the zoned link-local addresses of an answer received over a cable as that cable's route (§5); any other answer goes on as on develop.
- The interface list passed to `RegisterProxy` and `Browse` now includes IPv6-only interfaces (from §1).

### 3. Dial order

`peerService.orderAddrs` sorts stably by transport scheme only, and every LAN address is `yamux://`, so heart's order is the dial order. `localPeerDialTimeout` (2 s) covers the whole address walk.

Link-local addresses proven on a cable go **first**: the cable leads straight to the peer, while its IPv4 may be stale (its Wi-Fi went off) and hang until the budget is gone. A cable that is gone fails at once, because its zone names no interface. A peer without such a route keeps develop's list, so a Wi-Fi-only LAN is unaffected (§5).

### 4. Inbound side: record the zoned source (heart)

`recordLocalPeer` (`space/spacecore/rpchandler.go`) records the address the peer connected from. Its `url.Parse` is replaced with the same string handling as `hostIP` in `space/spacecore/peer.go` (strip `scheme://`, `net.SplitHostPort`, `netip.ParseAddr`, which keeps the zone). An IPv4 address goes first in the peer's list, as before; a zoned link-local one becomes the peer's inbound route (§5).

This is what makes one discovery direction enough. In phase 1 the Mac discovers the iPhone over the cable, dials `[fe80::…f9dc%31]:62654` and runs the exchange. The iPhone records the Mac at `[fe80::…%<index of en2>]:<Mac port>` from that connection (its own interface, correct by construction), then dials back and syncs. Without this fix the iPhone keeps only the Mac's IPv4 `LocalServer` addresses, which are unreachable over the cable.

Also check `peerIPsOf`/`hostIP`: a zoned address parses fine and simply never matches an IPv4 subnet in `OwnAddresses.For` (a zoned `netip.Addr` never matches a prefix), so the Mac's `LocalServer` to the iPhone falls back to its advertised IPv4 list. That's harmless: those fail fast while Wi-Fi is off, and the iPhone dials the recorded link-local address first anyway.

### 5. As built

Fork (`../zeroconf_lanipv6`, about 300 non-test lines over the pinned commit):
- IPv6 mDNS runs only on the link-local-only interfaces. `ServerLinkLocalIPv6(indexes)` makes an answer sent there carry that interface's own link-local addresses and limits the server's IPv6 (join, receive, send) to those interfaces; `SelectIPv6Ifaces` does the same for the browser. Every other interface stays IPv4-only. IPv6 packets from other interfaces are dropped on receive (the wildcard socket also takes unicast traffic).
- The browser loop is develop's. A packet from the IPv6 socket on a selected interface is deduplicated per instance and interface rather than per instance, delivered with `ServiceEntry.LinkIndex` set to that interface, and held back at most 120 s (the RFC 6762 §10 address TTL): while the peer answers, a cable is delivered again every two to three minutes. Packets from the IPv4 socket, from other interfaces and service type enumeration take develop's path.
- A zone is the receiving interface's decimal index, not its name: Go resolves a name again at dial time through a cache that can map it to the old index for a minute after a cable is plugged back in. A lookup's delivery gives up only when the caller cancels, so a packet is delivered whole, as on develop.

Heart:
- `DiscoveredPeer.Link` is the decimal index of the cable a discovery came from, and its `Addrs` are only the zoned link-local addresses there. IPv4 mDNS and the app's native discovery leave it empty and carry develop's list.
- Without a link-local-only interface each discovery is dispatched at once, as on develop. With one, a peer's discoveries run one at a time; a parked discovery is replaced only by a newer one from the same source (the base list or one cable), and a peer with work queued keeps queueing after the cable goes.
- A self-assigned 169.254/16 address is not routable IPv4 (`addrs.HasRoutableIPv4`), both for choosing cable interfaces and for checking a route's interface: in the asymmetric case (the iPhone's cable interface has one, the Mac's has no IPv4) link-local is the only path.
- `space/spacecore/localpeeraddrs.go` keeps per peer a base list, which is what develop installs (the latest base discovery or inbound exchange, last writer wins), and routes: `inbound` (the link-local address the peer last connected from) and `mdns:<index>` per cable. An inbound zone is resolved to its decimal index when the route is admitted; a zone naming no usable interface admits none. A name is never looked up again, so a route cannot move to another cable that takes the name.
- The installed list is the inbound route, the cable routes, then the base list, each address once. A route whose interface is gone, down or has routable IPv4 is left out. Without routes the installed list is exactly the base list and no interface is looked up.
- A route expires 4 minutes after it was set, unless a connection the pool picks (`Pick`, no dial) runs over one of its addresses, IP and index; either way the list is composed again then.
- Every discovery, base or cable, runs the outbound exchange, as on develop. A cable answers every two to three minutes, so a cable peer is exchanged with at that pace, which also teaches a restarted peer, or one whose route to us expired, our cable address again.
- When the peer store forgets a peer, its routes go and its base list is installed again and kept for the next discovery; a peer without routes keeps its list, as on develop. Close stops every route; nothing is installed after it.
- With a link-local-only interface present, own IPv4 comes from the interfaces only; the flat-list fallback (which holds loopback and tunnels) is skipped. iOS checks a link-local-only `en*` with a zoned self-connect, like it checks IPv4. Interface changes are detected by identity (name and index), and for the interfaces that may be used over link-local, by which addresses each holds.

Accepted limits:
- A stale route on a still-up interface (an IPv6-only switch port the peer left) is dialed first until it expires (at most 4 minutes) and can use the 2 s budget. Bounding each address attempt would need per-address deadlines in any-sync.
- `Pick` returns the incoming connection first, so a live outgoing cable connection hidden behind an incoming one does not renew its route. The peer stays reachable over the live incoming (or Wi-Fi) connection, both sides dial the cable first anyway, and the next cable answer puts the route back.
- A discovery dispatched at once (no cable) is not tracked: a cable appearing while it runs may race it, until the next cable answer.
- The interface refresh reads the addresses of each cable-candidate interface every 5 s.
- Windows: no cable sync in phase 1. `golang.org/x/net` reports no receiving interface there, so no zone can be attached, and the interface allowlist has no Windows entry. To try a cable on Windows, test IPv4 APIPA (169.254/16) over it first.
- any-sync's `PreferQuic` ranks a yamux address it classifies as local (link-local) ahead of one it doesn't (a LAN on public IPv4). With link-local first this changes nothing.

Follow-ups:
- Phase 2: the app's native discovery (Swift, Kotlin) reporting zoned link-local addresses feeds the same store as one more source, next to its IPv4 base list.
- When a cable answer arrives and `Pick` returns a connection that is not over the cable, close it, so the next dial takes the cable.
- Windows: read the receiving interface (`IP_PKTINFO`/`IPV6_PKTINFO` through `WSARecvMsg`, which `x/net` does not expose) and add allowlist entries for Ethernet adapters.

Device checks still to run before merge: the on-device steps under Testing, plus a locked phone on USB with both devices on Wi-Fi (sync goes on, the cable route stays first), and two cables (a route each; unplugging one leaves the other).

## Phase 2 (separate PRs)

- **Swift** (`anytype-swift`): the address lookup also asks for `kDNSServiceProtocol_IPv6`, but reports a link-local address only for an interface where no IPv4 showed up for that peer, zoned by its interface index, as a cable source (`DiscoveredPeer.Link`) so heart keeps it as a route next to the IPv4 list; the bridge has to carry the interface for that. This lets the iPhone discover the Mac too, so either side can start the exchange.
- **Android**: the same rule in the Kotlin NSD resolver (per network, report the link-local with the interface name). Needs the union of addresses per peer, which `getHost()` does not give (see the GO-7579 review notes).
- `cmd/lanprobe`: browse IPv6 and print zoned addresses, to test without a second app.

## Testing

Unit (heart):
- `net/addrs`: the link-local sibling returns zoned-capable `fe80::` only, never global or ULA; `GetAddr` stays IPv4-only.
- `filterMulticastInterfaces`: an IPv6-only interface is kept; an interface with neither is dropped.
- `ownaddr`: an IPv6-only interface yields a `link-local` entry that is neither in `Addrs` nor in `Nets`; an interface with both families yields no IPv6.
- `readAnswers` (or a helper it calls): a cable answer yields its zoned link-local addresses on our cable only; any other answer its IPv4 addresses, in interface order; unzoned link-local is dropped.
- `recordLocalPeer`: an inbound `yamux://[fe80::1%en16]:51234` with `LocalServer.Port` 4006 installs `yamux://[fe80::1%31]:4006` first when `en16` is index 31. Table-test `hostIP` with zones.
- The per-peer store: a Wi-Fi list and a cable route don't wipe each other in either order; an unusable or expired route leaves the list; a live connection over a route renews it; without a cable every list and exchange is develop's.

Unit (fork):
- `appendAddrs` with the option: on interface A it adds only A's link-local; the fixed IPv4 list is unchanged.
- receive path: a packet carrying an AAAA, delivered with `IfIndex` = N, yields an entry whose zoned address has N as its zone.
- browser: an IPv6 answer on a selected interface is delivered on its own (`LinkIndex`) and again after the 120 s cap; every other packet keeps develop's once-per-TTL dedup.

On device (the setup from 2026-10-09):
1. iPhone and Mac both running heart from this branch (the iPhone needs the `recordLocalPeer` zone fix to keep the Mac's link-local address), joined by a cable, Wi-Fi off on both, Bluetooth off on the phone.
2. Mac log: `own lan addresses` lists `en16 fe80::… link-local`. `dns-sd -B _anytype._tcp` lists the Mac's service on `en16`.
3. Mac log: the iPhone discovered with `[fe80::…%31]:62654` (`en16`'s index), and an outbound exchange succeeds.
4. iPhone log: `updated local peer` for the Mac lists a `[fe80::…%<index>]` address first.
5. Edit an object in a shared space on each side: it syncs both ways with no internet.
6. Turn Wi-Fi back on: the cable route stays first while the cable answers, with the Wi-Fi addresses after it; `own lan addresses` no longer lists `en16` link-local if `en16` gains IPv4 (it won't over this cable, so the line stays; check nothing else changes).
7. Regression on a normal Wi-Fi LAN: the address lists and the lanprobe output match today's.

## Risks

- **Windows**: `golang.org/x/net/ipv6` control messages are not implemented on Windows, so the receiving interface is unknown and no zone can be attached. No cable sync there in phase 1 (§5).
- **Virtual interfaces** with IPv6 only would start being advertised on and browsed. A Mac has many: `anpi*`, `ap1`, `llw0`, `awdl0`, and several `utun*` (VPN and system tunnels, all `MULTICAST` with a link-local address). Linux has `veth*`, `docker*` and VPN tunnels. Implemented as an allowlist by interface name for link-local-only use: darwin/iOS `en*`, `bridge*`; Linux `en*`, `em*`, `eth*`, `wl*`, `usb*`, `thunderbolt*`, `br*`, `bond*` and biosdevname `p1p1`-style names; Android `wlan*`, `eth*`, `rndis*`, `usb*`; nothing on Windows.
- **More mDNS traffic**: IPv6 multicast is added on every kept interface. Measure query and answer counts on a busy LAN; the existing `refreshInterfaces` rebuild path is unchanged.
- **Interfaces are renumbered** when a cable is plugged back in. Routes are zoned by index, so the old ones are unusable at once and leave the list; the next answer over the cable, or the next inbound connection, brings the new index.

## References

- Device evidence: the 2026-10-09 18:14 cable test in the Problem table. Note the iPhone's cable interface is not always IPv6-only: with Wi-Fi on, its log showed `en2 169.254.104.250/16` in `own lan addresses`, while the Mac's `en16` had no IPv4 in either test. The rule is evaluated per device and per interface.
- Code: `net/addrs/common.go` (`GetAddr`, `HasRoutableIPv4`), `space/spacecore/localdiscovery/{common,localdiscovery,ownaddr,linklocal,peerqueue}.go`, `space/spacecore/{localpeeraddrs,rpchandler,peer}.go`, zeroconf fork `client.go` (`recv`, `mainloop`) and `server.go` (`appendAddrs`, `ServerLinkLocalIPv6`).
- `docs/superpowers/specs/2026-10-09-lan-ipv6-design-decision-handoff.md` (why per-source routes over a fork address cache or complete address lists).
- `docs/superpowers/specs/2026-10-09-ios-native-mdns-design.md` (bridge contract, Swift address lookup).
