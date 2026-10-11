# GO-7581 Variant D: per-link deliveries in the fork, per-source address store in heart

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the zeroconf fork's cross-link address cache (`merge.go`) with per-link deliveries, and do the union of addresses in heart, so IPv4 browsing is develop's code again and one heart mechanism can later serve the native (phase 2) discovery too.

**Architecture:** The fork browser keeps develop's per-instance, once-per-TTL dedup for every packet from the IPv4 socket. A packet from the IPv6 socket on a selected (cable) interface is deduped separately per instance and interface, re-delivered at most every 120 s, and marked with that interface (`ServiceEntry.LinkIndex`). Heart keeps per peer a *base* list (what develop installs: the latest IPv4/native discovery or inbound exchange, last writer wins) plus *proven link-local routes* (`inbound`, `mdns:<zone>`) with expiry, and installs `routes first, then base`. With no routes the installed list is exactly the base list, as on develop.

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
- Create: `space/spacecore/localpeeraddrs.go` (store, compose, expiry)
- Modify: `space/spacecore/peer.go` (PeerDiscovered, remove `withInboundLinkLocal`, `rememberDiscovered`, `expireInboundLinkLocal`, `inboundLinkLocal` type and helpers; keep `setLocalPeerAddrs`, `linkLocalUsable`, `liveLinkLocalConn`, `isLinkLocalAddr`, `hostIP`)
- Modify: `space/spacecore/rpchandler.go` (recordLocalPeer)
- Modify: `space/spacecore/service.go` (fields)
- Test: `space/spacecore/localpeeraddrs_test.go` (replaces `TestWithInboundLinkLocal`, `TestInboundLinkLocalExpires*` in `peeraddrs_test.go`)

**Interfaces:**
- Consumes: `DiscoveredPeer.Link` (Task 2), `linkLocalUsable(zone string) bool`, `liveLinkLocalConn(s *service, peerId string) bool`, `setLocalPeerAddrs(peerId string, hostPorts []string)`, `timeNow`, `afterFunc`.
- Produces (all called under `s.localAddrsMu`):
  - `func (s *service) setBaseAddrs(peerId string, hostPorts []string) (installed []string)`
  - `func (s *service) setRoute(peerId, source string, hostPorts []string) (installed []string, changed bool)`
  - `func (s *service) dropRoute(peerId, source string)`
  - `func (s *service) markExchanged(peerId string, installed []string)` / `func (s *service) exchangedWith(peerId string, installed []string) bool`
  - sources: `routeInbound = "inbound"`, `"mdns:" + zone`.

Store, in `localpeeraddrs.go`:

```go
// localPeerAddrs is what we know about how to dial one LAN peer (GO-7581).
// base is the list develop installed: the latest IPv4 or native discovery,
// or the latest inbound exchange, whichever came last. routes are link-local
// addresses proven on a cable (the peer answered or connected over it), one
// per source, each expiring unless renewed. The installed list is the
// routes, then base: with no route it is exactly base.
type localPeerAddrs struct {
	base      []string
	routes    map[string]*linkRoute
	exchanged []string // the installed list at the last successful outbound exchange
}

type linkRoute struct {
	hostPorts []string
	zone      string
	at        time.Time
	expiry    *time.Timer
}

// linkRouteTTL: a cable delivery repeats at least every 120 s while the
// peer answers (the fork's linkRedelivery, queries ≤60 s apart), and an
// inbound route is renewed by each exchange; two missed rounds end it.
const linkRouteTTL = 4 * time.Minute
```

Compose (pure, unit-tested on its own):

```go
// composeAddrs orders the dial list: inbound route, then cable routes by
// source name, then base; each address once. Routes whose zone is not
// usable (interface gone, down, or now with routable IPv4) are left out.
func composeAddrs(routes map[string]*linkRoute, base []string, usable func(zone string) bool) []string
```

Expiry: the timer callback calls `liveLinkLocalConn` **before** taking `localAddrsMu` (Fable: don't hold the lock across `pool.Pick`), then under the lock checks the route is still the same pointer; live → renew (`at`, new timer); not live → delete the route and re-install `composeAddrs` if it differs from `localAddrs[peerId]`.

- [ ] **Step 1: Failing tests** in `localpeeraddrs_test.go` (fixture: `service` with stubbed `setLocalPeerAddrs` capture via the existing `peerService` mock pattern used in `peeraddrs_test.go`; stub `timeNow`, `afterFunc` returning a controllable fake timer, `linkLocalUsable`, `liveLinkLocalConn`):
  - `composeAddrs`: no routes → base unchanged (same order, same elements); inbound + `mdns:31` + base with a duplicate → `[inbound, mdns, base-without-dup]`; unusable zone dropped.
  - "a Wi-Fi discovery keeps the cable route": setRoute(`mdns:31`, fe80) then setBaseAddrs([192.168.1.5]) → installed `[fe80…%31, 192.168.1.5]`. (Review Focus 1)
  - "a cable discovery keeps the Wi-Fi base": reverse order, same result. (Review Focus 1)
  - "an unplugged cable leaves at compose time": `linkLocalUsable` false → installed is base only. (Review Focus 3)
  - "expiry without a live connection removes the route and reinstalls base"; "a live link-local connection renews it" (fire the fake timer, assert a new timer and unchanged list). (Review Focus 3, 4)
  - "an old timer after renewal does nothing".
  - "forgetPeerIPs clears routes and stops timers".
- [ ] **Step 2: Run, expect FAIL.** `go test -race ./space/spacecore/ -run 'Compose|Route|LocalPeer'`
- [ ] **Step 3: Implement** the store; `service.go`: replace `inboundLinkLocal`, `localAddrs`? — keep `localAddrs` (installed list, used by expiry), replace `inboundLinkLocal` and `discoveredAddrs` with `localPeers map[string]*localPeerAddrs` (guarded by ownMu). `forgetPeerIPs` deletes `localPeers[peerId]` after stopping its timers.
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit:** `GO-7581 Keep LAN peer addresses per source in heart`

### Task 5: Heart — wire discovery and inbound into the store

**Files:**
- Modify: `space/spacecore/peer.go` (`PeerDiscovered`)
- Modify: `space/spacecore/rpchandler.go` (`recordLocalPeer`)
- Test: `space/spacecore/peeraddrs_test.go` (`TestRecordLocalPeer` stays green; new `TestPeerDiscoveredSources`)

**Interfaces:**
- Consumes: Task 4's store API, `DiscoveredPeer.Link`.

`PeerDiscovered`:

```go
s.localAddrsMu.Lock()
var installed []string
if discovered.Link == "" {
	installed = s.setBaseAddrs(discovered.PeerId, discovered.Addrs)
} else {
	installed, _ = s.setRoute(discovered.PeerId, "mdns:"+discovered.Link, discovered.Addrs)
}
skip := discovered.Link != "" && s.exchangedWith(discovered.PeerId, installed)
s.localAddrsMu.Unlock()
s.rememberOwn(own)
if skip {
	// a cable answer repeats every 120 s; the exchange already ran
	// over this exact list
	return
}
shared, proof, err := s.exchangeOutbound(ctx, discovered.PeerId, &own)
if err != nil {
	return
}
s.localAddrsMu.Lock()
s.markExchanged(discovered.PeerId, installed)
s.localAddrsMu.Unlock()
```

A base discovery always exchanges, as on develop (goal 3). `recordLocalPeer`: `portAddrs` (connection address first, then LocalServer ips) goes to `setBaseAddrs`; when the connection address is a zoned link-local, also `setRoute(peerId, routeInbound, [thatHostPort])`, else `dropRoute(peerId, routeInbound)`.

- [ ] **Step 1: Failing tests** (`TestPeerDiscoveredSources`, with `exchangeFn` stubbed and counting calls):
  - "a repeated cable discovery does not exchange again": two `PeerDiscovered{Link: "31"}` with the same addrs → one exchange.
  - "a failed exchange is retried on the next cable discovery": stub returns an error the first time → two exchanges. (Review Focus 2)
  - "a base discovery always exchanges": two identical `Link: ""` discoveries → two exchanges (develop behavior).
  - "an inbound IPv4 exchange keeps the cable route from mDNS but drops the inbound route".
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** as above; delete the now-unused helpers from `peer.go` and their old tests in `peeraddrs_test.go` (their behaviors are re-covered in Task 4's tests).
- [ ] **Step 4: Run the full heart verification suite and the iOS vet** (Global Constraints). Expect PASS.
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
