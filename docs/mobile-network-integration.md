# Network & device state integration guide (iOS / Android / Desktop)

anytype-heart recovers sync connectivity fastest when the client tells it
about network and lifecycle changes. This guide describes exactly which RPCs
to call, when, and what heart does with them. No new API is required — the
existing `DeviceNetworkStateSet` and `AppSetDeviceState` RPCs now drive a full
recovery pipeline (connection-pool flush → responsible-peer rebuild →
immediate head-sync of all spaces).

Heart also runs its own net monitor (interface-address diffing every 5s +
sleep detection via clock jump), so missing signals degrade gracefully — but
OS callbacks are faster (~instant vs up to 5s) and, on Android, interface
enumeration inside Go is impossible without the injected getter (see below),
so the RPC signals matter most there.

## What heart does with each signal

| RPC | When heart receives it | Effect |
|---|---|---|
| `Device.NetworkState.Set(type)` | type differs from the last report | notifies Wi-Fi-gate/mDNS hooks; **flushes the connection pool, rebuilds responsible peers, head-syncs all spaces** (skipped for the very first report and when switching *to* NOT_CONNECTED — then only flush + fast "offline" status) |
| `App.SetDeviceState(FOREGROUND)` | after >15s in background | same recovery pipeline + refreshes opened objects |
| `App.SetDeviceState(BACKGROUND)` | always | flushes WAL so a frozen/killed process loses nothing |

While heart believes the device is offline (client reported `NOT_CONNECTED`,
or no usable interface address exists), periodic node re-dials stretch from
20s to 2min per space and sync-status flips to offline immediately — so
report `NOT_CONNECTED` promptly to save battery, and report the reconnect
promptly to snap back (the reconnect triggers an immediate rebuild, you never
wait for the 2min probe).

Signals are debounced heart-side (5s suppression window with one coalesced
trailing run), so call the RPC on **every** path-change callback and do not
debounce client-side.

## iOS — NWPathMonitor

Use `NWPathMonitor` (Network.framework). It fires on Wi-Fi↔cellular
transitions, connect/disconnect, and also when the Wi-Fi *network* changes
while the type stays the same (heart's interface monitor catches that case
too, since Go interface enumeration works on iOS).

```swift
import Network

final class HeartNetworkReporter {
    private let monitor = NWPathMonitor()
    private let queue = DispatchQueue(label: "io.anytype.netmonitor")

    func start() {
        monitor.pathUpdateHandler = { path in
            let networkType: Anytype_Model_DeviceNetworkType
            if path.status != .satisfied {
                networkType = .notConnected
            } else if path.usesInterfaceType(.cellular) {
                networkType = .cellular
            } else {
                // wifi / wiredEthernet / other satisfied paths
                networkType = .wifi
            }
            // fire and forget; heart dedupes repeated values
            var req = Anytype_Rpc.Device.NetworkState.Set.Request()
            req.deviceNetworkType = networkType
            _ = Lib.ServiceDeviceNetworkStateSet(try? req.serializedData())
        }
        monitor.start(queue: queue)
    }
}
```

Lifecycle (SwiftUI example; UIKit: `didEnterBackground` /
`willEnterForeground` notifications):

```swift
.onChange(of: scenePhase) { phase in
    var req = Anytype_Rpc.App.SetDeviceState.Request()
    switch phase {
    case .background: req.deviceState = .background
    case .active:     req.deviceState = .foreground
    default:          return
    }
    _ = Lib.ServiceAppSetDeviceState(try? req.serializedData())
}
```

Notes:
- Call `NetworkState.Set` from the `pathUpdateHandler` directly — including
  the initial callback after `start()` (heart treats the first report as
  baseline, not as a switch).
- Do not gate the foreground `SetDeviceState` on network availability; heart
  decides what to do.
- `path.status == .requiresConnection` counts as not satisfied → report
  `notConnected`.

## Android — ConnectivityManager.NetworkCallback

Register a default-network callback. It fires on Wi-Fi↔cellular handoffs,
loss of connectivity, and VPN changes.

```kotlin
class HeartNetworkReporter(context: Context) {
    private val cm = context.getSystemService(ConnectivityManager::class.java)

    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onCapabilitiesChanged(net: Network, caps: NetworkCapabilities) {
            report(caps)
        }
        override fun onLost(net: Network) {
            // onLost fires before a replacement network is up; if another
            // network is already active this is followed by
            // onCapabilitiesChanged for it, which heart coalesces.
            reportType(DeviceNetworkType.NOT_CONNECTED)
        }
    }

    fun start() = cm.registerDefaultNetworkCallback(callback)

    private fun report(caps: NetworkCapabilities) {
        val type = when {
            !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) ->
                DeviceNetworkType.NOT_CONNECTED
            caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) ->
                DeviceNetworkType.CELLULAR
            else -> DeviceNetworkType.WIFI // wifi / ethernet / vpn-over-wifi
        }
        reportType(type)
    }

    private fun reportType(type: DeviceNetworkType) {
        // middleware wrapper generated from protos
        Service.deviceNetworkStateSet(
            Rpc.Device.NetworkState.Set.Request(deviceNetworkType = type)
        )
    }
}
```

Lifecycle via `ProcessLifecycleOwner`:

```kotlin
ProcessLifecycleOwner.get().lifecycle.addObserver(LifecycleEventObserver { _, event ->
    when (event) {
        Lifecycle.Event.ON_STOP ->
            Service.appSetDeviceState(Rpc.App.SetDeviceState.Request(deviceState = BACKGROUND))
        Lifecycle.Event.ON_START ->
            Service.appSetDeviceState(Rpc.App.SetDeviceState.Request(deviceState = FOREGROUND))
        else -> {}
    }
})
```

**Required on Android — interface getter injection.** Go cannot enumerate
network interfaces on Android 11+ (netlink is restricted). The bindings
already expose an injection point used by local discovery *and* by the new
heart-side interface monitor; make sure it is wired at startup and re-invoked
is not needed (heart re-reads through the getter every 5s):

```kotlin
// clientlibrary/service exposes SetInterfaceGetter (see lib_android.go)
Service.setInterfaceGetter(object : InterfaceGetter {
    override fun interfaces(): List<Interface> = enumerateViaConnectivityManager()
})
```

Without it, heart's monitor logs `net monitor: interface enumeration
unavailable` once and relies solely on your RPC callbacks and the sleep
detector — same-type Wi-Fi→Wi-Fi switches then go undetected until the
transport keepalive (≤20s), so wiring the getter is strongly recommended.

Battery notes:
- The 2min offline probe cadence only applies while `NOT_CONNECTED` is the
  last reported state — report it reliably (e.g. also from `onUnavailable`).
- Heart's yamux/QUIC keepalives run at 10s while connections exist; when the
  app is backgrounded the process is frozen and nothing ticks.

## Desktop (Electron) — recommended but optional

Heart self-detects sleep/wake (clock jump) and interface changes (5s poll) on
desktop, so no client work is strictly required. For the fastest recovery,
forward powerMonitor events; heart coalesces them with its own detection:

```js
const { powerMonitor } = require('electron')
powerMonitor.on('suspend', () => rpc.AppSetDeviceState({ deviceState: 0 /* BACKGROUND */ }))
powerMonitor.on('resume',  () => rpc.AppSetDeviceState({ deviceState: 1 /* FOREGROUND */ }))
```

## Verifying the integration

1. Watch heart logs for `connectivity recovery` entries with the reason
   (`network type changed to …`, `foreground resume`, `wake from sleep`,
   `interface addresses changed`). Every OS-level transition should produce
   exactly one (bursts coalesce; `connectivity recovery coalesced` lines are
   fine).
2. Toggle airplane mode: sync status must flip to offline within ~1s and
   recover within ~2s of connectivity returning (plus dial time).
3. Switch Wi-Fi↔cellular with the app foregrounded: edits made on another
   device must arrive within a few seconds (previously up to ~40s).
