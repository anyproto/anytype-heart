# GO-7581 Variant D: per-link deliveries in the fork, per-source address store in heart

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the zeroconf fork's cross-link address cache (`merge.go`) with per-link deliveries, and do the union of addresses in heart, so IPv4 browsing is develop's code again and one heart mechanism can later serve the native (phase 2) discovery too.

**Architecture:** The fork browser keeps develop's per-instance, once-per-TTL dedup for every packet from the IPv4 socket. A packet from the IPv6 socket on a selected (cable) interface is deduped separately per instance and interface, re-delivered at most every 120 s, and marked with that interface (`ServiceEntry.LinkIndex`). Heart keeps per peer a *base* list (what develop installs: the latest IPv4/native discovery or inbound exchange, last writer wins) plus *proven link-local routes* (`inbound`, `mdns:<index>`) with expiry, and installs `routes first, then base`. With no routes the installed list is exactly the base list, as on develop.

> **Superseded during review (2026-10-11).** The code differs from the original steps in these review-driven points; Tasks 4 and 5 below are updated to match.
> - No exchange skip: every discovery, base or cable, runs the outbound exchange (no `exchanged`, `markExchanged`, `exchangedWith`).
> - No connection tracking: route renewal asks the pool's `Pick` only.
> - An inbound route's zone is resolved to its decimal index when the route is admitted; a zone name is never looked up again.
> - A route is renewed only by a live connection over one of its own addresses (IP and index).
> - Forgetting a peer drops its routes and keeps its base list.
> - Without a cable, discoveries are dispatched at once, as on develop; the per-source queue is used only with one.
> - Close clears the store.
> - Fork: a lookup delivers a received packet whole (only the caller's cancellation cuts a send short).
> - Fork: a cable delivery needs a link-local address (an A-only answer takes no slot).
> - Also: base discoveries carry IPv4 only; the inbound link-local address is a route, not part of the base list; `linkLocalUsable` became `linkLocalZone`.

**Tech Stack:** Go, github.com/miekg/dns, golang.org/x/net/ipv4/ipv6, any-sync peer pool, testify.

**Spec:** `docs/superpowers/specs/2026-10-09-lan-ipv6-link-local-design.md` (phase-1 goal and testing) and `docs/superpowers/specs/2026-10-09-lan-ipv6-design-decision-handoff.md` (variants A/B/C; the Fable decision adds D, recorded there in Task 6).

## Global Constraints

- Worktrees: heart `/Users/roman/anytype/anytype-heart_lanipv6` (branch `go-7581-lan-ipv6-link-local`), fork `/Users/roman/anytype/zeroconf_lanipv6` (branch `go-7581-ipv6-link-local`). Heart's go.mod keeps `replace github.com/libp2p/zeroconf/v2 => ../zeroconf_lanipv6`.
- Do not push, do not open PRs.
- Every heart and fork commit message starts with `GO-7581 `.
- Goal 3: on a machine with no link-local-only interface nothing changes — no IPv6 socket, develop's dedup, develop's installed list, develop's exchange cadence.
- Zones are decimal interface indexes (Go's zone-name cache can be stale for 60 s).
- Link-local is dialed first, IPv4 after: a wrong IPv4 can burn the 2 s local dial budget (`localPeerDialTimeout`), an unplugged cable's zone fails in 0 ms.
- Errors wrapped with `fmt.Errorf("op: %w", err)` (CLAUDE.md).
- Verification suite (heart): `go test -race ./space/spacecore/... ./net/addrs/... ./clientlibrary/service/...`; iOS vet: `CC="$(xcrun --sdk iphoneos -f clang)" CGO_ENABLED=1 GOOS=ios GOARCH=arm64 go vet -tags "nographviz nowatchdog nosigar nomutexdeadlockdetector noheic" ./space/spacecore/... ./net/addrs/...`. Fork: `go test -race -run 'Test(Mainloop|Recv|Join|Append|Interface|Lookup|Enumeration|IPv4Only|Link)' ./...` (TestBasic hangs on this Mac on the base commit too).

## Review Focus

1. A cable peer that also sits on Wi-Fi: the Wi-Fi delivery (base) must not wipe the cable route, and the cable delivery must not wipe the Wi-Fi base.
2. Cable-only peer whose first exchange fails (peer not listening yet): later cable deliveries must retry the exchange, not be suppressed as "unchanged".
3. Cable unplugged / interface index gone: the route must leave the installed list (on expiry at the latest, and at compose time when `linkLocalUsable` says no).
4. A live link-local connection must keep its route past the TTL (no answer needed while syncing).
5. Two deliveries for one peer from different sources arriving while an exchange runs: neither may be dropped by the per-peer queue.

Each has a test in the owning task (Tasks 2, 4, 5).

---

### Task 1: Fork — per-link deliveries instead of the merge cache

**Files:**
- Delete: `zeroconf_lanipv6/merge.go`
- Modify: `zeroconf_lanipv6/client.go` (mainloop, receivedMsg, recv)
- Modify: `zeroconf_lanipv6/service.go` (ServiceEntry.LinkIndex)
- Modify: `zeroconf_lanipv6/server.go` (fold ServerSelectIPv6Ifaces into ServerLinkLocalIPv6)
- Test: `zeroconf_lanipv6/linklocal_test.go` (drop merge tests, add per-link tests)

**Interfaces:**
- Produces: `ServiceEntry.LinkIndex int` — non-zero when the entry was built from a packet received over IPv6 on a `SelectIPv6Ifaces` interface; such an entry describes that link only. `ServerLinkLocalIPv6(ifIndexes ...int)` now also limits server IPv6 traffic to those interfaces. `ServerSelectIPv6Ifaces` is removed. `SelectIPv6Ifaces` (client) unchanged.

- [ ] **Step 1: Write the failing tests** in `linklocal_test.go`. Remove `TestMainloopMergesAnswersFromSelectedLinks` and any test that exercises `byIface`/`answerAddrs`/`merge.go`. Add a `send6` helper and the tests:

```go
func (f *mainloopFixture) send6(msg *dns.Msg, ifIndex int) {
	f.msgCh <- receivedMsg{msg: msg, ifIndex: ifIndex, v6: true}
}

func TestMainloopDeliversEachCableSeparately(t *testing.T) {
	t.Run("a Wi-Fi answer then a cable answer are two deliveries", func(t *testing.T) {
		// given
		f := newMainloopFixture(t, &client{ipv6Ifaces: indexSet([]int{ifCable})}, true, 4)
		f.send(f.answer(3600, "192.168.1.62"), ifWifi)
		first := f.next()

		// when
		f.send6(f.answer(3600, "fe80::1"), ifCable)

		// then
		second := f.next()
		assert.Equal(t, 0, first.LinkIndex)
		assert.Equal(t, ifCable, second.LinkIndex)
		assert.Equal(t, []netip.Addr{netip.MustParseAddr("fe80::1%if" + strconv.Itoa(ifCable))}, second.AddrIPv6Zoned)
	})
	t.Run("IPv4 socket answers keep develop's once-per-TTL dedup with a cable selected", func(t *testing.T) {
		f := newMainloopFixture(t, &client{ipv6Ifaces: indexSet([]int{ifCable})}, true, 4)
		f.send(f.answer(3600, "192.168.1.62"), ifWifi)
		f.next()
		f.send(f.answer(3600, "192.168.1.63"), ifWifi)
		f.send(f.answer(3600, "192.168.1.62"), ifCable)
		f.none()
	})
	t.Run("a repeated cable answer is delivered once until the 120 s cap", func(t *testing.T) {
		f := newMainloopFixture(t, &client{ipv6Ifaces: indexSet([]int{ifCable})}, true, 4)
		f.send6(f.answer(3600, "fe80::1"), ifCable)
		e := f.next()
		assert.WithinDuration(t, time.Now().Add(linkRedelivery), e.Expiry, 2*time.Second)
		f.send6(f.answer(3600, "fe80::1"), ifCable)
		f.none()
	})
	t.Run("the cable answer is delivered again after the cap expires", func(t *testing.T) {
		origCleanup, origCap := cleanupFreq, linkRedelivery
		cleanupFreq, linkRedelivery = 50*time.Millisecond, 100*time.Millisecond
		t.Cleanup(func() { cleanupFreq, linkRedelivery = origCleanup, origCap })
		f := newMainloopFixture(t, &client{ipv6Ifaces: indexSet([]int{ifCable})}, true, 4)
		f.send6(f.answer(3600, "fe80::1"), ifCable)
		f.next()
		time.Sleep(300 * time.Millisecond)
		f.send6(f.answer(3600, "fe80::1"), ifCable)
		assert.Equal(t, ifCable, f.next().LinkIndex)
	})
	t.Run("two cables are independent", func(t *testing.T) {
		f := newMainloopFixture(t, &client{ipv6Ifaces: indexSet([]int{ifCable, ifCable2})}, true, 4)
		f.send6(f.answer(3600, "fe80::1"), ifCable)
		f.send6(f.answer(3600, "fe80::2"), ifCable2)
		assert.ElementsMatch(t, []int{ifCable, ifCable2}, []int{f.next().LinkIndex, f.next().LinkIndex})
	})
	t.Run("without a selection IPv6 packets use the instance key", func(t *testing.T) {
		f := newMainloopFixture(t, &client{}, true, 4)
		f.send6(f.answer(3600, "2a0d::5"), ifWifi)
		assert.Equal(t, 0, f.next().LinkIndex)
		f.send(f.answer(3600, "192.168.1.62"), ifWifi)
		f.none()
	})
}
```

Define the constants next to `ifWifi` if missing: `ifCable = 31`, `ifCable2 = 32`. Keep `TestIPv4OnlyBrowsingKeepsDeliveriesAcrossCleanup`, `TestLookupReturnsAfterItsFirstResult`, `TestRecvLoopDropsUnselectedInterfaces`, `TestRecvReportsTheReceivingInterface`, `TestJoinUdp6MulticastOnlySelected`, `TestEnumerationIgnoresIPv6Selection`, `TestInterfaceZoneIsTheIndex`, the `TestAppendAddrs*` tests and `TestMainloopZonesLinkLocalByReceivingInterface`; adapt them to the removed option (server tests build `serverOpts` via `ServerLinkLocalIPv6` only). `TestRecvReportsTheReceivingInterface` additionally asserts `v6` is true for the IPv6 socket and false for IPv4.

- [ ] **Step 2: Run, expect compile failure** (`v6`, `LinkIndex`, `linkRedelivery` undefined).

Run: `cd /Users/roman/anytype/zeroconf_lanipv6 && go test -race -run 'TestMainloop' ./...`

- [ ] **Step 3: Implement.**
  - `git rm merge.go`.
  - `receivedMsg` gets `v6 bool`; `recv` sets it from the conn type (pass it into `recvLoop` as a parameter `v6 bool`).
  - `ServiceEntry` gets:
    ```go
    // LinkIndex is the interface an IPv6 answer arrived on when that
    // interface was selected with SelectIPv6Ifaces; 0 otherwise. Such an
    // entry describes that link only: answers on other links are
    // delivered on their own.
    LinkIndex int `json:"-"`
    ```
  - `mainloop`: restore develop's body (from `git show 528971b:client.go`), keeping: the `msgCh` parameter, `zonedIPv6` filling `AddrIPv6Zoned`, and the ctx-aware `send`. Then, per packet:
    ```go
    // linkIndex: a packet on the IPv6 socket over a selected interface is
    // its own delivery stream, so an answer over Wi-Fi does not hide the
    // one over the cable (and back)
    linkIndex := 0
    if received.v6 && !enumeration {
        if _, ok := c.ipv6Ifaces[received.ifIndex]; ok {
            linkIndex = received.ifIndex
        }
    }
    ```
    where `enumeration := params.ServiceRecord.ServiceTypeName() == params.ServiceRecord.ServiceName()` is computed once. In the delivery loop the dedup key is `key := k` or, with `linkIndex != 0`, `k + "|v6:" + strconv.Itoa(linkIndex)`; `sentEntries` uses `key` everywhere `k` was used. Before sending a link entry: `e.LinkIndex = linkIndex` and
    ```go
    // re-delivered at most every linkRedelivery, so heart's route for
    // this link outlives it only while answers keep coming
    if limit := now.Add(linkRedelivery); e.Expiry.After(limit) {
        e.Expiry = limit
    }
    ```
    with `var linkRedelivery = 120 * time.Second` (a var for tests; the RFC 6762 §10 address TTL).
  - `server.go`: delete `ServerSelectIPv6Ifaces`; `ServerLinkLocalIPv6` sets both `o.linkLocalIfaces` and `o.ipv6Ifaces` to the same set; doc: "…and limits IPv6 traffic to them: IPv4 keeps using every interface".

- [ ] **Step 4: Run tests, expect PASS.** Run the fork command from Global Constraints and `go vet ./...`. Check the non-test diff size: `git diff --stat 528971b -- ':!*_test.go'` should be roughly 250 lines.

- [ ] **Step 5: Commit** (fork): `git commit -am "GO-7581 Deliver each selected IPv6 link separately instead of merging answers"`

### Task 2: Heart — discovery reports the link a delivery came from

**Files:**
- Modify: `space/spacecore/localdiscovery/common.go` (DiscoveredPeer.Link)
- Modify: `space/spacecore/localdiscovery/localdiscovery.go` (readAnswers, peerAddrs, startServer)
- Modify: `space/spacecore/localdiscovery/peerqueue.go` (latest per peer and link)
- Test: `localdiscovery_test.go`, `peeraddrs_test.go` in that package if peerAddrs tests live there, `peerqueue_test.go`

**Interfaces:**
- Consumes: `ServiceEntry.LinkIndex` (Task 1), `ServerLinkLocalIPv6` only.
- Produces: `DiscoveredPeer.Link string` — `""` for an IPv4 or native discovery (installs the base list), else the decimal interface index the delivery is about; such a delivery's `Addrs` holds only zoned link-local `ip%zone:port` on that link.

- [ ] **Step 1: Failing tests.**
  - `peerAddrs` (existing table test): add cases — a link entry (`LinkIndex: 31`, AddrIPv4 `192.168.1.5`, zoned `fe80::1%31`) returns only `[fe80::1%31]:4006`; a link entry whose zone is not one of ours returns nil.
  - `peerqueue_test.go`:
    ```go
    t.Run("a parked discovery from another link is not replaced", func(t *testing.T) {
        // given: one discovery running, then a base and a link discovery parked
        // when: the running one returns
        // then: both parked ones run, base first (arrival order), and a second
        // base discovery replaces the first parked base one
    })
    ```
    (Build it on the existing blocking-notifier fixture in that file: a notifier that blocks the first call on a channel and records every `DiscoveredPeer` it gets.)
  - `readAnswers`: an entry with `LinkIndex` set yields `DiscoveredPeer{Link: "31", Addrs: ["[fe80::1%31]:4006"]}`.

- [ ] **Step 2: Run, expect FAIL.** `go test -race ./space/spacecore/localdiscovery/...`

- [ ] **Step 3: Implement.**
  - `DiscoveredPeer`:
    ```go
    // Link is empty for a discovery that replaces the peer's address list
    // (IPv4 mDNS, the app's native discovery). Otherwise it is the decimal
    // index of the cable interface the peer answered on, and Addrs holds
    // only its link-local addresses there (GO-7581).
    Link string
    ```
  - `peerAddrs(entry, zones)`: when `entry.LinkIndex != 0`, return only the zoned link-local addresses on our zones (skip IPv4). Otherwise unchanged.
  - `readAnswers`: set `Link: strconv.Itoa(entry.LinkIndex)` when non-zero.
  - `startServer`: drop `zeroconf.ServerSelectIPv6Ifaces(...)`.
  - `peerqueue.go`: `pendingDiscovery.next` becomes `[]queuedDiscovery`; `notify` replaces a parked entry with the same `peer.Link`, else appends; `run` pops the head. Update the type comment: discoveries from different sources (base, each link) are all kept, only a newer one from the same source replaces a parked one.

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit:** `GO-7581 Report the cable link a discovery came from`

### Task 3: Heart — 169.254 does not hide a cable

**Files:**
- Modify: `space/spacecore/localdiscovery/linklocal.go:45`
- Modify: `space/spacecore/peer.go` (`linkLocalUsable` shares the rule)
- Test: `space/spacecore/localdiscovery/ownaddr_test.go` or `localdiscovery_test.go` (where linkLocalOnly is tested)

**Interfaces:**
- Produces: `addrs.HasRoutableIPv4(iface *addrs.NetInterfaceWithAddrCache) bool` in `net/addrs/common.go`: true when the interface holds an IPv4 address outside 169.254/16. Used by both `linkLocalOnly` and `linkLocalUsable`.

- [ ] **Step 1: Failing test:** `linkLocalOnly("darwin", iface)` for `en5` holding `169.254.10.2/16` and `fe80::1/64` returns `[fe80::1]`; holding `192.168.2.2/24` and `fe80::1/64` returns nil. Plus a `HasRoutableIPv4` table test in `net/addrs/common_test.go`.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** `HasRoutableIPv4` with the loop now inline in `linkLocalUsable`; `linkLocalOnly` uses `HasRoutableIPv4(iface)` instead of `len(iface.GetAddr()) > 0`; `linkLocalUsable` calls it. Check `ownAddrs` (ownaddr.go): an interface whose only IPv4 is 169.254 must yield link-local entries and still its 169.254 IPv4 entry as today (assert in the test).
- [ ] **Step 4: Run, expect PASS** (`./space/spacecore/... ./net/addrs/...`).
- [ ] **Step 5: Commit:** `GO-7581 Treat a self-assigned IPv4 like no IPv4 when choosing cable interfaces`

### Task 4: Heart — per-source address store

**Files:**
- Create: `space/spacecore/localpeeraddrs.go` (store, compose, expiry, zone check, live connections)
- Modify: `space/spacecore/peer.go` (remove `withInboundLinkLocal`, `rememberDiscovered`, `expireInboundLinkLocal`, `liveLinkLocalConn`, `linkLocalUsable`, the `inboundLinkLocal` type and helpers; keep `setLocalPeerAddrs`, `isLinkLocalAddr`, `hostIP`)
- Modify: `space/spacecore/rpchandler.go` (`recordLocalPeer`)
- Modify: `space/spacecore/service.go` (fields, `Close`)
- Test: `space/spacecore/localpeeraddrs_test.go` (replaces `TestWithInboundLinkLocal`, `TestInboundLinkLocalExpires*`, `TestLinkLocalUsable`)

**Interfaces:**
- Consumes: `DiscoveredPeer.Link` (Task 2), `addrs.HasRoutableIPv4` (Task 3), `setLocalPeerAddrs(peerId string, hostPorts []string)`.
- Produces (called under `s.localAddrsMu` unless noted):
  - `func (s *service) setBaseAddrs(peerId string, hostPorts []string)`, `setRoute(peerId, source string, hostPorts []string)`, `dropRoute(peerId, source string)`: change the store only.
  - `func (s *service) installLocalAddrs(peerId string) []string`: composes and installs, once per event.
  - `func (s *service) forgetLocalPeerAddrs(peerId string)`: drops the routes and installs the base list again, which stays; a peer without routes is left as is.
  - `func (s *service) closeLocalPeerAddrs()`: on `Close`, stops every route; nothing is installed after it.
  - `expireRoute` (timer callback, takes the lock itself).
  - `var linkLocalZone func(zone string) (index string, ok bool)` (name or index to the decimal index of an up interface without routable IPv4) over `usableLinkZone`; `var liveLinkLocalAddrs func(s *service, peerId string) []netip.Addr` over `pooledLinkLocalAddrs` (unary and stream `Pick`, no dial; link-local only, zones resolved to indexes). Both called outside the lock.
  - sources: `routeInbound = "inbound"`, `routeMDNS + index`.

Store, in `localpeeraddrs.go`:

```go
// localPeerAddrs is what we know about how to dial one LAN peer.
type localPeerAddrs struct {
	base   []string
	routes map[string]*linkRoute
}

// linkRoute is one source's addresses, each "[ip%index]:port" with the
// decimal index of the interface it was proven on.
type linkRoute struct {
	hostPorts []string
	expiry    *time.Timer
}

const linkRouteTTL = 4 * time.Minute
```

Compose (pure, unit-tested on its own):

```go
// composeAddrs is the dial list: the inbound route, then the cable routes by
// source, then the base list, each address once. A route address counts
// only when its zone is the decimal index of a usable interface; a zone name
// is never looked up here. Without routes: the base list as it is, and no
// interface lookup.
func composeAddrs(routes map[string]*linkRoute, base []string, usable func(index string) bool) []string
```

Expiry: the timer callback calls `liveLinkLocalAddrs` **before** taking `localAddrsMu`, then under the lock checks the route is still the same pointer. A live address that is one of the route's (IP and index) re-arms it; otherwise the route is deleted. The list is composed and installed again either way. `Pick` prefers the incoming connection, so a live outgoing cable connection behind an incoming one does not renew; the next cable answer puts the route back.

- [ ] **Step 1: Failing tests** in `localpeeraddrs_test.go` (fixture: the service with a recording `peerService`, `afterFunc` capturing timers to fire by hand, stubbed `linkLocalZone` and `liveLinkLocalAddrs`, a default succeeding exchange; every case goes through `PeerDiscovered`/`recordLocalPeer`):
  - `composeAddrs`: no routes → base unchanged, no lookup; inbound + `mdns:31` + `mdns:32` + base with duplicates → inbound, cable routes, base, each once; unusable index dropped; a name-zoned route dropped without a lookup.
  - Review Focus 1: a Wi-Fi discovery keeps the cable route, and a cable discovery keeps the Wi-Fi list.
  - Review Focus 3: an unplugged cable leaves the list at the next install; an expired route leaves it.
  - Review Focus 4: a live connection over the route renews it; one over another cable, or to another address on the same cable, does not; a renewed route over an interface no longer usable leaves the list. The pool is asked without the lock.
  - An old timer after renewal does nothing; forgetting a peer drops its routes and keeps its base list; a forgotten peer without routes keeps its installed list; Close stops every route and nothing is installed after it.
  - `recordLocalPeer`: an IPv4-only discovery keeps the inbound route in front; an IPv4 connection drops it; the same cable from mDNS (index) and inbound (name) is one address; the route keeps its cable when the name moves to another interface; a zone naming no usable interface admits no route.
  - `pooledLinkLocalAddrs` with gomock pools (`Pick` only): a link-local connection zoned by index, IPv4 ignored, one address once, no connection, an unusable zone.
- [ ] **Step 2: Run, expect FAIL.** `go test -race ./space/spacecore/ -run 'Compose|LocalPeer|RecordLocalPeer|InboundRoute|PooledLinkLocal|UsableLinkZone'`
- [ ] **Step 3: Implement** the store; `service.go`: replace `inboundLinkLocal`, `localAddrs` and `discoveredAddrs` with `localPeers map[string]*localPeerAddrs` and `localAddrsClosed`, both guarded by `localAddrsMu` (taken before `ownMu`). `recordLocalPeer`: an IPv4 connection address goes first in the base list; a zoned link-local one, resolved to its decimal index with `linkLocalZone` outside the lock, becomes the inbound route (no route when it names no usable interface), otherwise `dropRoute(peerId, routeInbound)`. `forgetPeerIPs` calls `forgetLocalPeerAddrs`; `Close` calls `closeLocalPeerAddrs`.
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit:** `GO-7581 Keep LAN peer addresses per source in heart`

### Task 5: Heart — wire discovery into the store

**Files:**
- Modify: `space/spacecore/peer.go` (`PeerDiscovered`)
- Modify: `space/spacecore/localdiscovery/localdiscovery.go`, `peerqueue.go` (dispatch)
- Test: `space/spacecore/peeraddrs_test.go` (`TestRecordLocalPeer` stays green; new `TestPeerDiscoveredSources`), `localdiscovery_test.go` (`TestReadAnswersDispatch`)

**Interfaces:**
- Consumes: Task 4's store API, `DiscoveredPeer.Link`.

`PeerDiscovered`:

```go
s.localAddrsMu.Lock()
if s.localAddrsClosed {
	s.localAddrsMu.Unlock()
	return
}
if discovered.Link == "" {
	s.setBaseAddrs(discovered.PeerId, discovered.Addrs)
} else {
	s.setRoute(discovered.PeerId, routeMDNS+discovered.Link, discovered.Addrs)
}
s.installLocalAddrs(discovered.PeerId)
s.localAddrsMu.Unlock()
s.rememberOwn(own)
// every discovery exchanges, as on develop. A cable answers again every
// two to three minutes, so a cable peer is exchanged with at that pace,
// which also teaches a restarted peer, or one whose route to us expired,
// our address on the cable again
shared, proof, err := s.exchangeFn(ctx, discovered.PeerId, &own)
if err != nil {
	return
}
s.publishLocalPeer(discovered.PeerId, shared, proof, directionOutbound)
```

Dispatch (`readAnswers` → `peerDiscoveries.notify(..., direct)`, `direct` when no link-local interface is selected): under the queue's mutex, a peer with work queued parks the discovery (replacing a parked one of the same source); otherwise `direct` runs it at once, as on develop, and a cable selected queues it.

- [ ] **Step 1: Failing tests** (`TestPeerDiscoveredSources`, with `exchangeFn` stubbed, counting calls and recording the installed list each call starts with):
  - "every cable discovery exchanges, after a failure too": three identical cable discoveries, the first exchange failing → three exchanges, each over the installed cable route. (Review Focus 2)
  - "a base discovery always exchanges, as on develop".
  - "a cable discovery keeps the Wi-Fi list, and its expiry leaves it"; "a Wi-Fi discovery keeps the cable route". (Review Focus 1)
  - "a forgotten cable peer keeps its Wi-Fi list for the next cable answer".
  - "an inbound IPv4 exchange keeps the cable route from mDNS but drops the inbound one".
  - "without a cable the list and the exchanges are develop's" (installed list after each step, the list each exchange started with, no interface lookup, no pool call, no timer), "an empty discovery empties the list and still exchanges", "duplicate base addresses go in as develop installs them". (Goal 3)
  - `TestReadAnswersDispatch`: without a cable, three deliveries reach the notifier while the first still runs; with a cable, they run one at a time; a discovery parked behind a cable one stays parked when the cable goes away.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** as above.
- [ ] **Step 4: Run the full heart verification suite** (Global Constraints). Expect PASS.
- [ ] **Step 5: Commit:** `GO-7581 Install LAN peer addresses from the per-source store`

### Task 6: Spec, handoff and memory

**Files:**
- Modify: `docs/superpowers/specs/2026-10-09-lan-ipv6-link-local-design.md` (section 5 "As built": replace the merge-cache description with D; add "Windows: no cable sync in phase 1" with the reason — no receiving interface from x/net, no allowlist entry — and the suggestion to test IPv4 APIPA first; drop the accepted limit "reverse-order cold split" if it no longer applies, keep "stale route ≤ linkRouteTTL"; add the device checks: locked phone on USB with both on Wi-Fi, two cables)
- Add: `docs/superpowers/specs/2026-10-09-lan-ipv6-design-decision-handoff.md` (append a "Decision" section: D, with the measured dial table from the Fable report)
- Add: this plan

- [ ] **Step 1:** Edit both docs.
- [ ] **Step 2: Commit:** `GO-7581 Document the per-source design and the Windows limit`
- [ ] **Step 3:** Update memory `lan-ipv6-link-local.md` (D landed; heads; fork diff size).

### Task 7: Review rounds

- [ ] Run the 4-lens Codex review (as in rounds 1-12) on both branches; approve/reject findings; implement; commit per round (`GO-7581 Review round N: …`); stop when a round has no important findings.

---

## Notes for the implementer

- Fable's Variant D also proposed dropping a route when `refreshInterfaces` sees its interface vanish. Not planned: a vanished index fails the dial in 0 ms (measured) and `composeAddrs` drops unusable zones on every install, so the timer suffices.
- The `interfaceIds` refresh cost (~6 sysctls / 5 s) is unchanged by this plan; it stays an accepted cost.
- `TestBasic` in the fork hangs on this Mac on the base commit; do not chase it.
