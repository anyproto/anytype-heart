# GO-7581 handoff: keep the address cache, or the simpler design?

Date: 2026-10-09. For a decision, not for implementation.

## Context

Goal of GO-7581 phase 1: an iPhone and a Mac joined only by a cable (Wi-Fi off on both) discover each other and sync over IPv6 link-local; nothing changes on networks with IPv4. Spec: `docs/superpowers/specs/2026-10-09-lan-ipv6-link-local-design.md` (its section 5 "As built" describes the current implementation).

Code:
- heart worktree `/Users/roman/anytype/anytype-heart_lanipv6`, branch `go-7581-lan-ipv6-link-local`, `git log origin/develop..` (15 commits).
- zeroconf fork worktree `/Users/roman/anytype/zeroconf_lanipv6`, branch `go-7581-ipv6-link-local`, `git log 528971b..` (12 commits). Heart's go.mod replaces the fork with this local path.

Background facts (verified in code):
- On develop the fork's browser delivers each service instance once per TTL: the first answer wins (`sentEntries` keyed by instance). That was harmless for IPv4 because heart's responder (`RegisterProxy`) sends the same fixed address list on every interface, so any single answer is complete.
- `SpaceExchange` requests carry `LocalServer{Ips, Port}`; the receiver records the sender at those addresses (`recordLocalPeer`), putting the address the connection came from first. Responses carry no `LocalServer`. GO-7580 narrows `LocalServer.Ips` to the receiver's subnet (`OwnAddresses.For`) to avoid useless dials.
- A link-local address is reachable only on its own link, and the zone (`%en16`) names the receiver's interface. A wrong-link link-local dial is not rejected at once: it waits for neighbor discovery and can use the whole 2 s local dial budget (`localPeerDialTimeout`).
- any-sync dials the address list in heart's order (all LAN addresses are yamux), sequentially, within one budget.
- iPhone side in phase 1: Swift is unchanged, so native discovery (mDNSResponder via the bridge) resolves IPv4 only. iOS's own responder advertises each interface's own AAAA only on that interface (on the cable: its cable link-local, no A record), with TTL 4500 s.
- The Mac's heart learns the iPhone over the cable from mDNS; the iPhone learns the Mac's cable address only from the Mac's inbound connection (`recordLocalPeer`).

## Variant A: the current implementation (address cache in the fork)

Advertise: per interface, an interface's own link-local AAAA only on that interface (`ServerLinkLocalIPv6`), plus the fixed IPv4 list everywhere. IPv6 mDNS joins and sends only on link-local-only interfaces (`ServerSelectIPv6Ifaces` / `SelectIPv6Ifaces`).

Browse: an AAAA is zoned with the interface it arrived on (decimal index). Because answers are now per-link partial, the browser merges answers across links in `merge.go`: per-link, per-record address cache with record TTLs (AAAA capped at 120 s), RFC 6762 cache-flush with the 1 s grace, per-record goodbyes, split announcements matched to cached targets, per-link withdrawals, and redelivery when the usable address set changes. Redelivery only for instances that have had a usable selected link-local; every other instance takes exactly the develop path on the packet's own entry.

Heart: link-local-only interfaces by name allowlist; per-peer serialized PeerDiscovered; link-local first in dial order; inbound link-local remembered across IPv4-only discoveries (2 min, renewed by a live link-local pool connection, timer removal, restore the discovered route); refresh signature with interface identity, flags and address ownership.

History: 12 rounds of 4 Codex review lenses until clean. Nearly all fork findings from round 2 on were in `merge.go` (TTL vs service TTL, partial answers with port 0, goodbyes, cache-flush, split packets, legacy-peer special cases, an introduced 10 s redelivery regression). Fork diff is large; the cache is an mDNS cache re-implemented inside the browser.

Accepted limits (spec): stale link-local on a still-up IPv6-only switch port <= 120 s; cold split with AAAA before SRV waits for the next answer; options not test-inspectable.

## Variant B: complete address lists everywhere, receiver zones (proposed, not built)

Restore the develop property "any single answer is complete":
1. mDNS advertise: put our link-local-only interfaces' link-local addresses into the fixed advertised list, sent on every interface like IPv4. No per-interface advertise option.
2. mDNS receive: zone each `fe80::` with OUR OWN link-local-only interface: exactly the arrival interface when the answer came over a link-local-only interface; otherwise each of our link-local-only interfaces (normally one cable). A receiver with no link-local-only interface drops it (no zone), so IPv4-only peers never dial it.
3. Exchange: add our link-local-only interfaces' link-local addresses (unzoned) to `LocalServer.Ips` whenever we have such an interface; the receiver zones them the same way. This covers the mixed case (phone on Wi-Fi + cable dials the Mac over Wi-Fi, Mac learns the phone's cable address; later the phone turns Wi-Fi off and the Mac dials the cable) without any Swift change.
4. Dial order by how the address was learned: proven on the link (answer arrived over the cable, or inbound connection came over it) first; learned indirectly (Wi-Fi answer, LocalServer) after IPv4.
5. Fork browser dedup stays exactly as on develop. `merge.go` and redelivery go away. Keep: receive-interface zone plumbing, IPv6 interface restriction, IPv6 receive filter, decimal-index zones. Heart keeps the allowlist, dial order, refresh detection and the inbound-memory logic (possibly simplified).

Known costs of B:
- Wrong-link candidate when the receiver has a cable to a DIFFERENT device (3 devices, 2 cables): NDP wait; mitigated by putting indirect link-local after IPv4.
- Extra AAAA records in answers on IPv4 interfaces while a cable is plugged in (bytes change; dials don't for IPv4-only receivers).
- Older heart receivers get `fe80::x` in `LocalServer.Ips` and format it with `"%s:%d"` into an unparseable address: one wasted, fast-failing entry after IPv4 (verify how fast).
- iPhone's native responder still advertises per interface, so the Mac learns the phone's cable address from mDNS only over the cable; the exchange path (3) covers the mixed case when the phone dials.
- Mixed transition (phone turns Wi-Fi off): the Mac's first dial may hit the dead Wi-Fi IPv4 first (indirect link-local is after IPv4) until an exact address arrives.

## Variant C: two browsers (considered, superseded by B)

Keep the develop IPv4 browser untouched; add a second IPv6-only browser bound to the cable interfaces; each keeps first-answer dedup; heart unions per peer. Avoids the cache but needs two browser lifecycles; does not solve the iPhone-side or mixed case better than B.

## What is needed from the reviewer

Critically weigh A vs B (and C if relevant): correctness for the phase-1 goal, the mixed Wi-Fi+cable cases, goal 3 (nothing changes on IPv4 networks), dial-budget risk, complexity and maintenance (fork diff size, future upstreaming), compatibility with older heart peers and with phase 2 (Swift/Kotlin reporting link-local). Find weak points in both, including ones not listed. Make a decision: keep A, switch to B (possibly modified), or something else, with the concrete reasons and the minimal change list if switching.

## Decision (2026-10-11)

Variant D, neither A nor B: the fork delivers each cable on its own (develop's browser loop, plus a per-instance-and-interface stream for IPv6 answers on the selected interfaces, `ServiceEntry.LinkIndex`), and heart takes the union per peer in a per-source store (a base list as develop installs it, plus link-local routes from mDNS per cable and from the inbound connection, each expiring). As built: spec §5. Plan: `docs/superpowers/plans/2026-10-11-lan-ipv6-variant-d.md`.

Why not B:
- "Every answer is complete" does not hold for iOS: its responder announces each interface's own AAAA only on that interface, as RFC 6762 §6.2 requires (all addresses valid on the sending interface, none that are not). B's step 1 would break that rule on our side as well.
- The mixed Wi-Fi + cable case can deadlock, each side waiting for an address only the other can teach it.
- A wrong-link candidate costs the whole dial budget, measured:

| Dial target | Result |
|---|---|
| `fe80::x` zoned to a live link where no neighbor answers | waits out the full deadline (5 s in the probe) |
| zone index that names no interface | 0 ms, `EINVAL` |
| unzoned `fe80::x` | 1 ms failure |
| `fe80::x:4006` (no brackets) | 0 ms, "too many colons" |

- Older heart peers would get `fe80::x` in `LocalServer.Ips`, about +200 ms per dial (a DNS lookup).

Why not A:
- It re-implements an mDNS cache inside the fork's browser (record TTLs, cache-flush, goodbyes, split announcements); nearly every review finding from round 2 on was in it.
- Goal 3 (nothing changes without a cable) held by argument, not by diff: D's browser is develop's loop, and heart installs exactly develop's list when no route exists.
- It cannot take phase 2's native discovery as a source; D's store can.
