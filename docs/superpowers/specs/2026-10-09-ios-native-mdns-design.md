# iOS native mDNS: LAN discovery through the system Bonjour (DNS-SD)

Date: 2026-10-09. Ticket: GO-7576 (heart) + IOS-XXXX (anytype-swift). Status: heart part (A) implemented.
Repos: anytype-heart (Go bridge, small) and anytype-swift (Bonjour engine, most of the work).
Related: `2026-10-09-ios-lan-socket-lifecycle-design.md` (TCP listener rebind; still needed), `2026-10-09-ios-socket-research.md` (Apple/XNU research).

## Why

Today heart runs its own mDNS responder and browser on iOS (`space/spacecore/localdiscovery/localdiscovery.go`, our `anyproto/zeroconf` fork) over raw multicast sockets. On a real iPhone (2026-10-09, `cmd/lanprobe`) these problems showed up:

- **Dead after one lock.** iOS defuncts the multicast sockets when it suspends the app. The server and browser recv loops then sit in error backoff forever. The device stops answering mDNS and stops hearing peers until the app restarts.
- **Peers reported only once.** zeroconf reports each peer once per browser. After unlock the device never re-runs SpaceExchange with peers it already knew.
- **Unreachable addresses advertised.** We list every IP on every interface, including cellular and tunnel addresses (the iPhone advertised `10.179.72.218`), so peers waste a 5 s dial timeout on each attempt.
- **Apple discourages this setup.** Apple strongly recommends against running your own responder (DTS, forum thread 685181). Ours needs the `com.apple.developer.networking.multicast` entitlement and competes with mDNSResponder for UDP 5353.

The system DNS-SD (mDNSResponder) fixes all of these by construction:

- **Automatic goodbye.** A registration lives in the system daemon. When iOS suspends the app, the daemon withdraws the record and sends the goodbye itself. This works on a crash too.
- **Change events.** Browse reports add, remove and address changes, so there is no dedup problem.
- **Correct addresses.** Each interface advertises its own addresses: Wi-Fi, Ethernet, Thunderbolt Bridge, USB. There is no mDNS on cellular, and the system picks up interface changes without polling.
- **Explicit permission errors.** A Local Network denial comes back as `kDNSServiceErr_PolicyDenied`.

Android already works this way: heart calls a platform bridge (`clientlibrary/service/discovery_android.go`) that uses `NsdManager` in anytype-kotlin. This spec gives iOS the same shape.

## Goals

1. On iOS, heart's LAN discovery uses the system DNS-SD through a Swift bridge. The zeroconf responder and browser don't run on iOS when the bridge is installed.
2. **Full interop** with heart peers that still use zeroconf (desktop on macOS, Windows and Linux; older iOS builds), in both directions. Android NSD peers interoperate too.
3. **After a lock/unlock**, the iPhone is advertised again and rediscovers peers within ~3 s of foreground, and SpaceExchange runs again.
4. **While backgrounded or suspended**, the advertisement is withdrawn (goodbye), so peers stop seeing the device.
5. **Peer removals** ("lost") reach heart. Phase 1 logs them; dial suppression is a follow-up.
6. **No regression for old app builds.** If no bridge is installed, heart falls back to zeroconf.

## Non-goals

- The TCP listener itself. Go keeps owning `:62654`, and its rebind-after-suspension fix is in the listener spec. Native mDNS without the listener fix advertises a port that refuses after a suspension, so **both must ship for end-to-end LAN sync after a lock**.
- macOS and desktop. They stay on zeroconf. A cgo `dns_sd` port for macOS is a possible later step.
- Acting on "lost" in heart, for example not dialing lost peers. That is a follow-up.
- Peer-to-peer Wi-Fi (AWDL, `kDNSServiceFlagsIncludeP2P`). Go's listener can't accept over `awdl0` without extra socket options.
- Changing Android behaviour. Android gets only the interface rename and the async observer from A2.

## Architecture

```
anytype-swift                                        anytype-heart (Go, gomobile Lib)
─────────────                                        ───────────────────────────────
GlobalServicesConfiguration.configure()
  └─ Lib.ServiceSetDiscoveryProxy(bridge) ───────►  localdiscovery (iOS): provider set → native path
                                                     on account start:  proxy.SetObserver(observer)
BonjourDiscoveryBridge  ◄──── SetObserver ─────────  on account stop:   proxy.RemoveObserver()
  (ServiceDiscoveryProxyProtocol)
  BonjourEngine (dns_sd, serial queue)
    register  _anytype._tcp  name=peerId port=observer.port()
    browse    _anytype._tcp
    resolve + getaddrinfo per peer
  ── observer.observeChange(result) ───────────────►  PeerDiscovered → SpaceExchange (async in Go)
  ── observer.observeLost(peerId) ─────────────────►  log (phase 1)
  ── observer.observeError(code, msg) ─────────────►  PolicyDenied → DiscoveryLocalNetworkRestricted
  UIScene foreground/background → start/stop engine (Swift-owned lifecycle)
```

**Ownership:**
- **Heart** decides when discovery is on: at account start and stop, and through the existing `AccountEnableLocalNetworkSync` / `DontStartLocalNetworkSyncAutomatically`.
- **Swift** decides when it can run (scene foreground) and owns everything DNS-SD.
- **Discovery runs** only when heart has set an observer **and** the scene is in the foreground. This is the same rule Android uses: anytype-kotlin's `MainActivity` starts NSD in `onResume` and stops it in `onPause`.

## Part A: heart (Go)

### A1. One bridge for both platforms (`clientlibrary/service`)

The Android bridge already has the right shape, so make it universal instead of adding an iOS copy:

- **Rename** `discovery_android.go` to `discovery_mobile.go` and `discovery_android_test.go` to `discovery_mobile_test.go`, both with `//go:build android || ios`.
- **Rename** `AndroidDiscoveryProxy` to `DiscoveryProxy`. Everything else keeps its name and semantics.
- **Add two methods** to the observer: `ObserveLost` and `ObserveError`.

```go
// DiscoveryProxy is implemented by the platform: anytype-kotlin (NsdManager) and anytype-swift (DNS-SD).
type DiscoveryProxy interface {
	SetObserver(observer DiscoveryObserver) // start advertising+browsing for this observer (replaces any previous one)
	RemoveObserver()                        // stop everything, deregister
}

func SetDiscoveryProxy(proxy DiscoveryProxy) // unchanged, called once at app launch

// ObservationResult is implemented by the platform.
type ObservationResult interface {
	Port() int
	Ip() string     // all addresses, comma-separated (gomobile has no slices)
	PeerId() string // the DNS-SD instance name
}

// DiscoveryObserver is implemented by Go and called by the platform.
type DiscoveryObserver interface {
	Port() int           // our listener port to advertise
	PeerId() string      // our instance name; also used to skip self
	ServiceType() string // "_anytype._tcp"
	ObserveChange(result ObservationResult)
	ObserveLost(peerId string)             // NEW: the peer's service was removed on all interfaces
	ObserveError(code int, message string) // NEW: platform error code (iOS: kDNSServiceErr_*, Android: NsdManager FAILURE_*);
	                                       // code 0 = a healthy (re)registration, clears an earlier error state
}
```

**Kotlin impact.** One rename when anytype-kotlin bumps the middleware: `MDNSDelegate` implements `DiscoveryProxy` instead of `AndroidDiscoveryProxy`. The new observer methods are additive. Go implements the observer and Kotlin only calls it, so Kotlin compiles without using them. Wiring Kotlin's `onServiceLost` to `observeLost` is the Android follow-up in the listener spec.

**What Swift sees** (gomobile naming, see `Lib.framework/Headers/Service.objc.h`): the protocols `ServiceDiscoveryProxyProtocol`, `ServiceDiscoveryObserverProtocol` and `ServiceObservationResultProtocol`, plus `Lib.ServiceSetDiscoveryProxy(_:)`. `int` maps to `Int`.

### A2. Non-blocking observer calls

Today `discoveryObserver.ObserveChange` calls `notifier.PeerDiscovered` **synchronously**. That runs the outbound SpaceExchange, which dials and can take seconds, on the platform's calling thread.

**Change:** `ObserveChange`, `ObserveLost` and `ObserveError` copy their arguments and return immediately. The work runs on a goroutine. To serialize work for the same peer and drop superseded updates, keep one in-flight exchange per peerId: if a newer `ObserveChange` arrives meanwhile, re-run once after the current one finishes.

This also fixes Android, where Kotlin serializes resolves behind a semaphore that `observeChange` holds.

### A3. Selecting the native path on iOS (`space/spacecore/localdiscovery`)

- The provider-backed implementation that Android already uses, `localdiscovery_android.go`, becomes `localdiscovery_provider.go` (`//go:build android || ios`) and is reused as is. The zeroconf implementation `localdiscovery.go` (`//go:build !android`) is renamed internally, for example `zeroconfDiscovery`.
- **Selection rule:**
  - On Android, `New()` returns the provider implementation, as today.
  - On iOS, `New()` returns a thin selector. At `Start()` it picks the provider implementation if `getNotifierProvider() != nil`, and zeroconf otherwise. It logs the choice at INFO.
  - The choice is fixed for the component's lifetime. If Swift installs the proxy late, that takes effect at the next account start.
  - On desktop, zeroconf, as today.
- **Discovery-possibility state (UI):**
  - The provider path keeps its `refreshInterfaces` and `getDiscoveryPossibility`. On iOS that is the existing self-connect check on `en*` interfaces.
  - **New:** `ObserveError` with `kDNSServiceErr_PolicyDenied` (-65570) sets `DiscoveryLocalNetworkRestricted`.
  - **New:** it is cleared again through `getDiscoveryPossibility` by the next successful `ObserveChange`, or by `ObserveError(0, …)`, which Swift sends after each successful registration. An interface change alone does not clear a denial.
- **Lost (phase 1):** log at INFO with the peerId, and add a debugstat counter. There is no behaviour change.
- **Port change.** If the listener spec ever rebinds to a different port, the provider calls `Provide` again (Remove + SetObserver) with the new `Port()`. Swift handles a fresh `setObserver` by re-registering.

### A4. Heart tests

- **Selector:** provider set selects native and never starts zeroconf; no provider selects zeroconf.
- **Async observer:** `ObserveChange` returns while the notifier is blocked (synctest). Per-peer coalescing runs exactly one follow-up.
- **Errors:** `ObserveError(-65570)` sets the restricted state, and a later success clears it.
- **Unchanged:** the existing bridge tests (`discovery_mobile_test.go` after the rename) keep passing.

## Part B: anytype-swift (iOS)

### B1. Where the code goes (existing conventions)

- **Gomobile adapter:** `Modules/ProtobufMessages/Sources/DiscoveryProxyAdapter.swift`. This is the only module that imports `Lib`. It contains a `fileprivate final class` conforming to `ServiceDiscoveryProxyProtocol` that forwards to a public Swift protocol, the same pattern as `ServiceMessageHandlerAdapter.swift` (`Lib.ServiceSetEventHandlerMobile`). The adapter must hold a **strong reference** to the bridge object it passes to Go.
- **Engine:** `Anytype/Sources/ServiceLayer/LocalDiscovery/` holds `BonjourDiscoveryService` (protocol plus `final class`), registered as a singleton in `Anytype/Sources/ServiceLayer/ServicesDI.swift`.
- **Installation:** `GlobalServicesConfiguration.configure()` calls `Lib.ServiceSetDiscoveryProxy(adapter)` once, **before any account start**. This is the same timing as Android, which calls it in `Application.onCreate`.
- **Info.plist:** add `_anytype._tcp` to `NSBonjourServices`. Today it lists only `_pulse._tcp`, and without the entry browse and register fail with PolicyDenied. `NSLocalNetworkUsageDescription` already exists. It currently reads "Network access. Non-localized value", so give it a real, localized string, because users see it in the permission prompt.
- **Entitlement:** keep `com.apple.developer.networking.multicast` for one release, because heart's zeroconf fallback still exists. Remove it once the fallback is gone.

### B2. Engine behaviour

Use the C DNS-SD API (`import dnssd`) for register, browse, resolve and address lookup. Bind every `DNSServiceRef` to **one serial `DispatchQueue`** with `DNSServiceSetDispatchQueue`. `NWListener` is not usable: Go owns the socket, and `DNSServiceRegister` can advertise any port. `NWBrowser` is acceptable for browse only, but resolution still needs DNS-SD, so one API is simpler.

**State:**
```
observer: ServiceDiscoveryObserverProtocol?      (set/cleared by Go)
isForeground: Bool                               (UIScene notifications)
running = observer != nil && isForeground
```
- **Start/stop:** recompute `running` on every `setObserver`, `removeObserver`, foreground and background, and start or stop the engine on the transition.
- **Threading:** Go calls `setObserver` and `removeObserver` from arbitrary threads, so hop onto the serial queue first.

**Start:**
1. **Register:**

   ```
   DNSServiceRegister(flags: kDNSServiceFlagsNoAutoRename, interfaceIndex: 0 /*any*/,
                      name: observer.peerId(), regtype: observer.serviceType() /*"_anytype._tcp"*/,
                      domain: "local.", host: nil /*system hostname*/,
                      port: UInt16(observer.port()).bigEndian, txt: empty)
   ```

   - **`NoAutoRename` is mandatory.** Heart treats the instance name as the peerId, and an auto-renamed `"<peerId> (2)"` would make every peer dial with the wrong identity.
   - **On success:** the register callback with `kDNSServiceErr_NoError` calls `observer.observeError(0, "")`. That tells heart the Local Network permission now works, and clears a "restricted" state left by an earlier denial.
   - **On `kDNSServiceErr_NameConflict`:** report it with `observeError`, then retry the registration after 5 s, three times. A conflict with our own stale record right after a quick restart is the likely cause.
2. **Browse:** `DNSServiceBrowse(flags: 0, interfaceIndex: 0, regtype: "_anytype._tcp", domain: "local.")`. Each callback carries `(flags Add/remove, interfaceIndex, serviceName)`.
   - **Skip our own instance:** `serviceName == observer.peerId()`.
   - **Track instances:** keep `[peerId: [interfaceIndex: Instance]]`.
3. **Per (peerId, interface) instance:**
   - `DNSServiceResolve(name, type, domain, interfaceIndex)` returns `hosttarget` and `port`, in network byte order.
   - Then a long-lived `DNSServiceGetAddrInfo(kDNSServiceProtocol_IPv4, interfaceIndex, hosttarget)` collects IPv4 addresses with Add/remove events.
   - **Lifetime:** keep the resolve and address-lookup refs alive while the instance exists, so address changes keep arriving. Deallocate them when the browse reports it removed.
4. **Report to Go**, debounced per peer by 250 ms, so `kDNSServiceFlagsMoreComing` bursts collapse:
   - **The address set:** the union of IPv4 addresses across that peer's interfaces. Keep link-local `169.254.x.x`, because Thunderbolt Bridge often uses it. Drop `0.0.0.0` and loopback. Order: the interface the peer was first seen on first.
   - **The port** comes from the resolve.
   - **When it changes,** call `observer.observeChange(Result(peerId:, ip: addrs.joined(","), port:))`.
   - **When the last interface instance of a peer is removed,** call `observer.observeLost(peerId)`.
   - **Never call Go while holding a lock.** Go returns immediately (A2).

**Stop** (background, `removeObserver`, or a fatal error): deallocate **the registration first**, so mDNSResponder sends the goodbye, then the browse, resolve and address-lookup refs. Clear the state. Don't call `observeLost` for peers cleared by our own stop: they didn't leave, we stopped looking.

**Errors:**

| Error | Action |
|---|---|
| `kDNSServiceErr_PolicyDenied` (-65570) | `observeError(code)`, stop, don't retry until the next foreground. The user denied, or hasn't yet answered, the Local Network prompt. The first browse after install triggers the prompt; don't start from a background launch. |
| `kDNSServiceErr_ServiceNotRunning` (-65563), `kDNSServiceErr_DefunctConnection` (-65569) | `observeError`. Restart the whole engine if still `running`, with backoff 1, 2, 4 … 30 s. These happen after a suspension and when mDNSResponder restarts. |
| `kDNSServiceErr_NameConflict` (-65548) | as described under Start step 1 |
| any other error on browse or register | `observeError`, restart with the same backoff |
| resolve or address-lookup error for one instance | drop that instance; the next browse Add retries it |

**Lifecycle details:**
- **Notifications:** subscribe to `UIScene.willEnterForegroundNotification` and `UIScene.didEnterBackgroundNotification`, the same signals `SceneStateNotifier` and `SceneLifecycleStateService` use for `AppSetDeviceState`.
- **Background task:** wrap the stop in `beginBackgroundTask` / `endBackgroundTask`. It takes milliseconds, but the assertion guarantees the goodbye goes out before suspension.
- **Cold launch into the background** (push, background fetch): don't start. Start on the first foreground.
- **Restart on foreground:** always create new refs. Don't reuse refs from before the suspension, which are defuncted.

### B3. Interop requirements (must hold; verified in acceptance)

- **Same naming as heart's zeroconf:** the instance name equals the peerId exactly, the type is `_anytype._tcp`, the domain is `local.`, and the TXT record is empty. heart's zeroconf passes `nil` text.
- **Peers see the iPhone:** zeroconf browsers on desktop and older iOS see the iPhone with an SRV target of the system hostname (for example `Romans-iPhone.local.`) and its A records. The iPhone's port must equal heart's listener port.
- **The iPhone sees peers:** the iPhone's browse sees zeroconf peers, whose SRV host is `<peerId>.local.` with A records, and Android NSD peers.

## Part C: rollout

1. **Heart PR (A1–A4).** It is safe alone: without a proxy, iOS keeps zeroconf. It ships in a heart nightly.
2. **anytype-swift:**
   - bump `MIDDLE_VERSION` in `Libraryfile`, or use `make setup-middle-local` against a local heart checkout during development;
   - implement B1–B2;
   - add the Info.plist entry.
3. **Listener rebind** (the other spec) must be in the same app release, for LAN sync to work after a lock.
4. **One release later:**
   - remove the iOS zeroconf fallback from heart, so the zeroconf build tags become desktop only;
   - drop the multicast entitlement.

## Acceptance (on device, app launched from the springboard and not under the Xcode debugger, `cmd/lanprobe` on a Mac on the same Wi-Fi)

1. **lanprobe sees the iPhone:** an mDNS `NEW` with the iPhone's **Wi-Fi address only**, with no cellular or tunnel IP, and the correct port. The probe's exchange to the iPhone succeeds.
2. **The iPhone sees peers:** heart logs the discovery of the Mac desktop app (zeroconf) and of an Android device (NSD), and the exchanges with them succeed.
3. **Lock 60 s:** lanprobe reports the iPhone `MISSING` from the next round on. The goodbye goes out even without a background task, because mDNSResponder withdraws the record.
4. **Unlock:** within ~3 s the iPhone is `seen` again, lanprobe logs a fresh inbound `v2 from` the iPhone (rediscovery plus re-exchange), and a `-fresh` dial succeeds. That last check needs the listener fix.
5. **Ten lock/unlock cycles:**
   - no name-conflict errors;
   - the instance name never gains a suffix;
   - no growth in refs, which you can check with the Instruments allocations or a debug counter.
6. **Local Network denied** (in Settings → Privacy → Local Network): `observeError(-65570)` reaches heart, and the LAN sync UI shows the restricted state.
7. **Thunderbolt or USB:** iPhone cabled to the Mac with Wi-Fi off. Discovery and an exchange work over the cable interface.
8. **Account switch / logout:** `removeObserver` deregisters, and lanprobe sees the goodbye. A new account registers with its own peerId.

## Risks

- **Interop with zeroconf parsing.** zeroconf has to pick up the A records the system attaches to answers whose SRV target is a hostname. This is plausible but unverified. Acceptance step 1 checks it with exactly the desktop code path, because lanprobe uses zeroconf.
- **iOS conflict handling on a quick re-register** after a crash, while a stale record is still cached. It is mitigated by `NoAutoRename` plus retry, and needs to be observed in testing.
- **Hostname collisions** are resolved by the system and invisible to us: the SRV target changes. That is fine.

## Open questions

1. Should Swift report IPv6 too, for when heart enables IPv6 for local peers? This spec says IPv4 only, matching heart (`localdiscovery.startServer` advertises IPv4 only).
2. Should the iOS engine also run while the app holds a background task (BGProcessing sync)? This spec says foreground only, the same as Android.
