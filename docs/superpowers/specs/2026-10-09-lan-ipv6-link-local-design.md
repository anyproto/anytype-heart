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
2. Address lists stay short: IPv6 appears only for an interface that has **no IPv4 at all**, and only its link-local address.
3. Nothing changes for peers on a network with IPv4: same addresses, same order, same dial cost.

## Non-goals

- Global and ULA IPv6 (`2a0d:…`, `fdf0:…`): they rotate and there are several per interface. An IPv6-only LAN needs none of them: its interfaces have no IPv4, so the link-local rule applies to them, and mDNS (`ff02::fb`) never leaves the link, so every peer it finds is reachable over link-local. They would matter only for peers on another subnet, which mDNS cannot find either.
- IPv6 in `LocalServer.Ips`. A zone names the *receiver's* interface, which the sender cannot know; the receiver gets the working address from the inbound connection instead (see Design §4).
- IPv6 on interfaces that also have IPv4.
- Phase 2 (below) in this ticket's first PR.

## Design

### 1. Own addresses: keep IPv6-only interfaces (heart)

- `net/addrs`: keep `GetAddr` IPv4-only (every existing caller relies on it). Add a sibling that returns the interface's IPv6 link-local addresses (`ip.IsLinkLocalUnicast() && ip.To4() == nil`), cached the same way. On Android, `interface_android.go` already receives every address from the app's `InterfaceGetter`; keep IPv6 there for the sibling instead of nothing.
- `localdiscovery.filterMulticastInterfaces`: keep an interface that is up, multicast, not loopback and has either an IPv4 address or an IPv6 link-local address.
- `localdiscovery/ownaddr.go`: extend `ownAddr` with the link-local of every interface that has no IPv4. It is never `advertised` (stays out of `OwnAddresses.Addrs`) and never `matchable` (stays out of `OwnAddresses.Nets`, so it can't reach `LocalServer`). It is used only for the mDNS records in §2. `logOwnAddresses` prints it as `link-local`.

### 2. Desktop zeroconf: advertise and browse on the link (zeroconf fork + heart)

Fork (`github.com/anyproto/zeroconf/v2`, replaced in `go.mod`):

- **Advertise per interface.** Add a server option, e.g. `ServerLinkLocalIPv6()`: when answering on `ifIndex`, `appendAddrs` adds that interface's own IPv6 link-local addresses (from `addrsForInterface`), and only those, on top of the fixed proxy list. The fixed list keeps carrying IPv4 as today. Never announce one interface's link-local on another interface.
- **Browse with the receiving interface.** In `client.recv`, keep the control message: `n, cm, src, err = pConn.ReadFrom(b)` for the IPv6 conn (and enable `ipv6.FlagInterface` with `SetControlMessage` where the platform allows it; on Windows control messages are unavailable, see Risks). Carry `cm.IfIndex` with the parsed message to the entry and record each AAAA as a zoned address. Proposed shape: a new `ServiceEntry.AddrIPv6Zoned []netip.Addr` (`netip.Addr.WithZone`), keeping `AddrIPv6 []net.IP` untouched for compatibility. The zone is the interface **name** from `net.InterfaceByIndex`; when that fails, the decimal index (Go's `net.Dial` accepts both).
- The fork already joins the IPv6 multicast group (`joinUdp6Multicast`) and sends AAAA records; nothing else is needed there.

Heart (`space/spacecore/localdiscovery/localdiscovery.go`):

- Switch both server and browse to `zeroconf.IPv4AndIPv6`, and pass the new server option.
- `readAnswers`: after the IPv4 addresses, append the entry's zoned link-local addresses (`net.JoinHostPort(addr.String(), port)` keeps the zone). Only addresses with a zone; drop an unzoned `fe80::`.
- The interface list passed to `RegisterProxy` and `Browse` now includes IPv6-only interfaces (from §1).

### 3. Dial order

Link-local addresses go **after** every IPv4 address in the list given to `SetPeerAddrs`. `peerService.orderAddrs` sorts stably by transport scheme only, and every LAN address is `yamux://`, so heart's order is the dial order. A peer reachable over Wi-Fi therefore never pays for the link-local attempt.

Budget: `localPeerDialTimeout` (2 s) covers the whole address walk. On a cable-only link the IPv4 addresses (if any are cached) fail at once with "no route to host", because their interface is down, so the link-local attempt keeps most of the budget. Verify this in the acceptance test; if a stale IPv4 address hangs instead, put an entry's link-local addresses first when the answer arrived on an interface with no IPv4.

### 4. Inbound side: record the zoned source (heart)

`recordLocalPeer` (`space/spacecore/rpchandler.go`) puts the address the peer connected from first. Replace its `url.Parse` with the same string handling as `hostIP` in `space/spacecore/peer.go` (strip `scheme://`, `net.SplitHostPort`, `netip.ParseAddr`, which keeps the zone), then `net.JoinHostPort(ip.String(), port)`.

This is what makes one discovery direction enough. In phase 1 the Mac discovers the iPhone over the cable, dials `[fe80::…f9dc%en16]:62654` and runs the exchange. The iPhone records the Mac at `[fe80::…%en2]:<Mac port>` from that connection (its own zone, correct by construction), then dials back and syncs. Without this fix the iPhone keeps only the Mac's IPv4 `LocalServer` addresses, which are unreachable over the cable.

Also check `peerIPsOf`/`hostIP`: a zoned address parses fine and simply never matches an IPv4 subnet in `OwnAddresses.For` (a zoned `netip.Addr` never matches a prefix), so the Mac's `LocalServer` to the iPhone falls back to its advertised IPv4 list. That's harmless: those fail fast while Wi-Fi is off, and the iPhone dials the recorded link-local address first anyway.

### 5. As built (after review round 1)

- IPv6 mDNS runs only on the link-local-only interfaces (`ServerSelectIPv6Ifaces`, `SelectIPv6Ifaces` in the fork); every other interface stays IPv4-only, byte for byte.
- The fork's browser merges addresses from answers on different interfaces and re-delivers the entry when the union grows, instead of keeping the first answer for the whole TTL. An entry with nothing usable is not passed on.
- Dial order: IPv4 on one of our subnets, then link-local, then other IPv4. Without a link-local address the IPv4 order is unchanged.
- With a link-local-only interface present, own IPv4 comes from the interfaces only; the flat-list fallback (which holds loopback and tunnels) is skipped.
- iOS checks a link-local-only `en*` with a zoned self-connect, like it checks IPv4.
- Accepted limit: any-sync's `PreferQuic` ranks a yamux address it classifies as local (link-local) ahead of a yamux address it doesn't (a LAN on public IPv4). It only matters with a public-IPv4 LAN and a cable at the same time; fixing it belongs in any-sync.

## Phase 2 (separate PRs)

- **Swift** (`anytype-swift`): the address lookup also asks for `kDNSServiceProtocol_IPv6`, but reports a link-local address only for an interface where no IPv4 showed up for that peer. Format it as `fe80::…%<if_indextoname(interfaceIndex)>`. The bridge needs no change. This lets the iPhone discover the Mac too, so either side can start the exchange.
- **Android**: the same rule in the Kotlin NSD resolver (per network, report the link-local with the interface name). Needs the union of addresses per peer, which `getHost()` does not give (see the GO-7579 review notes).
- `cmd/lanprobe`: browse IPv6 and print zoned addresses, to test without a second app.

## Testing

Unit (heart):
- `net/addrs`: the link-local sibling returns zoned-capable `fe80::` only, never global or ULA; `GetAddr` stays IPv4-only.
- `filterMulticastInterfaces`: an IPv6-only interface is kept; an interface with neither is dropped.
- `ownaddr`: an IPv6-only interface yields a `link-local` entry that is neither in `Addrs` nor in `Nets`; an interface with both families yields no IPv6.
- `readAnswers` (or a helper it calls): zoned link-local addresses come after IPv4; unzoned ones are dropped.
- `recordLocalPeer`: an inbound `yamux://[fe80::1%en16]:51234` with `LocalServer.Port` 4006 records `yamux://[fe80::1%en16]:4006` first. Table-test `hostIP` with zones.

Unit (fork):
- `appendAddrs` with the option: on interface A it adds only A's link-local; the fixed IPv4 list is unchanged.
- receive path: a packet carrying an AAAA, delivered with `IfIndex` = N, yields an entry whose zoned address has interface N's name as its zone.

On device (the setup from 2026-10-09):
1. iPhone and Mac both running heart from this branch (the iPhone needs the `recordLocalPeer` zone fix to keep the Mac's link-local address), joined by a cable, Wi-Fi off on both, Bluetooth off on the phone.
2. Mac log: `own lan addresses` lists `en16 fe80::… link-local`. `dns-sd -B _anytype._tcp` lists the Mac's service on `en16`.
3. Mac log: the iPhone discovered with `[fe80::…%en16]:62654`, and an outbound exchange succeeds.
4. iPhone log: `updated local peer` for the Mac lists a `[fe80::…%en…]` address first.
5. Edit an object in a shared space on each side: it syncs both ways with no internet.
6. Turn Wi-Fi back on: addresses return to IPv4-first, and `own lan addresses` no longer lists `en16` link-local if `en16` gains IPv4 (it won't over this cable, so the line stays; check nothing else changes).
7. Regression on a normal Wi-Fi LAN: the address lists and the lanprobe output match today's.

## Risks

- **Windows**: `golang.org/x/net/ipv6` control messages are not implemented on Windows, so the receiving interface is unknown and no zone can be attached. Skip link-local on Windows (log once) rather than guessing a zone.
- **Virtual interfaces** with IPv6 only would start being advertised on and browsed. A Mac has many: `anpi*`, `ap1`, `llw0`, `awdl0`, and several `utun*` (VPN and system tunnels, all `MULTICAST` with a link-local address). Linux has `veth*`, `docker*` and VPN tunnels. Implemented as an allowlist by interface name for link-local-only use: darwin/iOS `en*`, `bridge*`; Linux `en*`, `eth*`, `wl*`, `usb*`; Android `wlan*`, `eth*`, `rndis*`, `usb*`; nothing on Windows.
- **More mDNS traffic**: IPv6 multicast is added on every kept interface. Measure query and answer counts on a busy LAN; the existing `refreshInterfaces` rebuild path is unchanged.
- **Zone names change** when interfaces are renumbered (cable re-plugged). The next discovery or inbound connection replaces the address list (`SetPeerAddrs` replaces), and GO-7579's rediscovery path will clear stale entries.

## References

- Device evidence: the 2026-10-09 18:14 cable test in the Problem table. Note the iPhone's cable interface is not always IPv6-only: with Wi-Fi on, its log showed `en2 169.254.104.250/16` in `own lan addresses`, while the Mac's `en16` had no IPv4 in either test. The rule is evaluated per device and per interface.
- Code: `net/addrs/common.go` (`GetAddr`), `space/spacecore/localdiscovery/{common,localdiscovery,ownaddr}.go`, `space/spacecore/{rpchandler,peer}.go`, zeroconf fork `client.go` (`recv`, AAAA parsing) and `server.go` (`appendAddrs`, `addrsForInterface`).
- `docs/superpowers/specs/2026-10-09-ios-native-mdns-design.md` (bridge contract, Swift address lookup).
