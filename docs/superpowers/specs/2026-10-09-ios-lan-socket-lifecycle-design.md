# iOS LAN listener lifecycle: close on background, rebind on the same port

Date: 2026-10-09. Ticket: GO-7577. Status: implemented; spec revised after 3 Codex lenses, implementation after a 4-lens review round; on-device acceptance pending.
Companion: `2026-10-09-ios-native-mdns-design.md` (GO-7576). Discovery moves to the system DNS-SD in the app, so this spec only covers the TCP listener. **Both must ship in the same app release**: native mDNS advertises the port this spec keeps alive.

## Problem

iOS defuncts an app's sockets when it suspends the app. On recent iOS this happens immediately (Apple DTS, forum threads 840808 and 795697). Heart binds its LAN listener once, in `clientServer.Run`, and never re-creates it. So after one phone lock the iPhone cannot accept LAN connections until the app restarts.

What we observed on 2026-10-09 (iPhone, heart v0.50.21, `cmd/lanprobe` on a Mac on the same Wi-Fi):

| Socket | What happens after lock/unlock | Evidence |
|---|---|---|
| TCP listener `:62654` (yamux) | The port answers RST forever, even with the app in the foreground. The Go goroutine stays parked in `TCPListener.Accept` (`IO wait`) and logs nothing. | lanprobe `refused`; post-unlock stack dump: goroutine 604, `[IO wait, 22 minutes]` |
| UDP `:62654` (QUIC listener) | quic-go closes its Transport after `recvmsg: socket is not connected`, and nothing re-listens. | iOS log `net.transport.quic listener closed with error`, after every resume |
| pprof listener `:6060` (control) | Opened shortly before the lock; refused after a single ~1 min lock. | `nc` refused on both ports while the phone answered ping |
| HTTP gateway | Recovers. It already stops on Background and starts on Foreground. | a new `gateway.startServer` goroutine in the post-unlock dump |
| mDNS server/browser | Dead after suspension. | Handled by the native-mDNS spec (GO-7576), not here. |

**Why the hang is silent** (XNU source, research notes §1):
- A defunct TCP listener has its PCB closed, so incoming SYNs get RST.
- A non-blocking `accept` on an empty queue returns EWOULDBLOCK before it ever checks `so_error`.
- kqueue fires for a listener only when its accept queue is non-empty, so Go's Accept parks forever.
- A listener with a connection already queued at defunct time does get an error (EBADF). That is likely why the pprof server exited while the LAN listener hung.

Apple's guidance (TN2277; Quinn): close listeners before the app can be suspended and re-open them on foreground. status-go (PR #7663) fixed the same Go-on-iOS hang with same-port rebinding plus a self-dial probe, validated on a device.

## Goals

1. After a foreground transition, an iPhone accepts LAN connections within ~1 s, **on the same port as before**.
2. While the app is backgrounded, LAN peers get an immediate refusal for new connections instead of a TLS-handshake hang.
3. A listener that dies without a lifecycle event is detected and rebound while the app is in the foreground. This covers a missed Background and a suspension inside a background task. Goal 3 covers only states in which heart's goroutines run, that is, while not suspended. A **missed Foreground** after a delivered Background is not recoverable here: the recorded state stays background, and a timer can't tell continued background execution from a lost event. The listener then waits for the next Foreground, the same as the HTTP gateway.

## Non-goals

- Discovery and mDNS (GO-7576).
- Outgoing connections. The GO-3958 recovery pipeline flushes the pool on foreground after a long background.
- **Established connections.** Closing the listener does not close accepted sessions. Those die by yamux keepalive (≤20 s) or are reused if they survived. Goal 2 is about new connections only.
- Restoring the QUIC listener (D3).
- Android and desktop. Neither showed the bug: lanprobe confirmed Android's listener survives every lock. The whole mechanism is iOS-only.

## Design

All of it lives in `space/spacecore/clientserver`. No any-sync change is needed: yamux only uses `net.Listener` methods, and `Addr()` only for logging (verified by review).

### 1. `lanListener`: one long-lived `net.Listener` with a replaceable socket

`clientServer` registers a `lanListener` with yamux exactly once, in place of the raw TCP listener. It wraps an inner listener, `inner net.Listener` (an interface so tests can fake it), and a generation counter, both under a mutex and a `sync.Cond`.

**`Accept()`** loops:
1. Under the mutex, wait until there is an inner listener (`inner != nil`) or the outer listener is closed. Take `inner` and its `gen`.
2. Call `conn, err := inner.Accept()`.
3. **Success:** re-check under the mutex. If the outer listener is closed, or `gen` changed (a pause or rebind happened meanwhile), close `conn` and loop back to step 1, or return `net.ErrClosed` if closed. Otherwise return `conn`. A connection accepted by an obsolete socket is never handed to yamux.
4. **Error:**
   - **Outer listener closed:** return the bare `net.ErrClosed`. yamux's loop then exits and logs "listener closed" at INFO.
   - **`gen` changed** (intentional pause or rebind): loop back to step 1.
   - **Temporary error** (`net.Error.Temporary()`): return it. yamux sleeps 1 s and retries, as today.
   - **Anything else, on the current `gen`:** the socket died, for example EBADF from a defunct listener with a queued connection. Mark it dead and close it (`inner = nil`, `gen++`), send a non-blocking rebind request to the worker, and loop back to step 1.

**`Addr()`** returns a fixed `:port` address; it is used only for logging.

**`pause()`** closes `inner` and sets it to nil, then `gen++`. Go's `Close` is synchronous and wakes a goroutine parked in `Accept` (`pd.evict`), so this also releases a goroutine stuck on a defunct fd.

**Rebinding** is the worker's `rebindOnce`. It runs **entirely under the lifecycle mutex `lc.mu`** (the one Background takes):
1. If the desired state is background, do nothing.
2. Close any old `inner` first. While the old fd is open, its defunct PCB stays referenced, and the new bind can fail with EADDRINUSE.
3. Bind the saved port with `net.Listen("tcp", ":"+savedPort)` through an injectable `listen` func.
4. Install the socket with `install`. If the outer listener was closed meanwhile, `install` refuses and the new socket is closed.

A Background therefore either runs before the bind and is seen in step 1, or waits until the new socket is installed and then closes it. A live socket that is not installed never exists while Background returns. The bind is a local syscall, so Background waits microseconds at most. (Review round 1 found the earlier bind-outside-the-lock version could leave a live, untracked socket across a suspension.)

**`probe()`** reports whether the current socket is healthy. It calls `inner.(syscall.Conn).SyscallConn().Control` with `getsockopt(SOL_SOCKET, SO_ERROR)`. A non-zero error, or no inner listener, means dead. SO_ERROR is read-and-clear, so the caller rebinds on the first failure.

Whether a defunct listener reports EBADF here comes from XNU source and **is verified first, in an isolated diagnostic build** (Rollout step 1). If it doesn't hold, `probe()` switches to a self-dial of `127.0.0.1:port` with a 500 ms timeout, where refused means dead. That fallback costs one INFO-level handshake-error log per probe in yamux, so it runs only on foreground and on the watchdog tick.

**`Close()`**, at shutdown: mark the outer listener closed, close `inner`, and `Broadcast`.

### 2. `clientServer` lifecycle (iOS only)

The platform gate is a field (`lifecycle bool`, set from `runtime.GOOS == "ios"` in `New`), so tests can enable it on a Mac. With the gate off, `clientServer` behaves exactly as today, except for the startup fix in §3.

**`StateChange(state int)`** makes `clientServer` an `app.ComponentStatable`. `App.SetDeviceState` calls every component synchronously under `app.mu.RLock`, so the handler must not block on the network:

- **Background:** call `lanListener.pause()` **synchronously** and set `desired = background`. Closing a TCP listener does no network I/O and takes microseconds. So when the `AppSetDeviceState(BACKGROUND)` RPC returns to the app, the port is already closed, and teardown cannot be lost to suspension. Ideally the iOS app still wraps the RPC in `beginBackgroundTask` (Q1), but correctness doesn't depend on it.
- **Foreground:** set `desired = foreground` and kick the worker through a coalescing channel with buffer 1. The worker rebinds if the listener is paused or dead, or if `probe()` fails.
- **ClosingInitiated:** ignored; `Close` handles shutdown.

**Worker goroutine** (started in `Run` when `lifecycle` is on):
- It acts on the **latest** desired state, using a generation number so a stale kick can't undo a newer Background. Before installing a rebound socket it re-checks `desired == foreground`. If the app went to background meanwhile, it closes the new socket instead.
- **Rebind retries:** 50, 100, 200, 400 ms, then 1, 2, 5 s, capped at 5 s, for as long as the desired state is foreground and the port isn't bound. The port never changes at runtime (D2).
  - **Interrupting the wait:** a new lifecycle signal (Foreground, Background, or a dead socket) interrupts the backoff wait. The worker acts on it at once and starts the backoff over from 50 ms.
  - **Logging failures:** at WARN, on the first attempt and then about once a minute while the failure persists.
- **Watchdog:** every 20 s while desired ≠ background, it calls `probe()` and rebinds on failure. The worker also takes a rebind request from `Accept` (§1, step 4).
- **Initial state:** nothing has been reported yet, and Background is enum zero, so the worker starts as "foreground" with the watchdog active.

`clientServer.Close` stops the worker first, then closes the `lanListener`. No rebind can install past it, and yamux's later Close of the same wrapper is a no-op.

### 3. Startup fix: a bound listener must count as started

Today `startServer` returns an error when **persisting** the port fails (`storage.Set`) after the listeners are already bound. `Run` then leaves `serverStarted=false`. A healthy listener serves connections, but `localdiscovery.Start` sees `ServerStarted()==false` and never starts discovery.

**Fix:** a persistence failure is logged at WARN and doesn't fail `startServer`. The port is still in use; it may just change on the next launch.

### Decisions

- **D1. Close on background, not only rebind on foreground.** This follows Apple's guidance, deterministically releases the parked goroutine, and gives peers an immediate RST instead of the 5 s TLS hang lanprobe measured. The cost: an app backgrounded but not yet suspended (holding a background task or playing audio) can't accept new LAN connections. That's accepted, since serving can't be relied on once suspension is possible.
- **D2. Same port only; no runtime fallback to a random port.** Every consumer has the port cached and none of them watch it:
  - the native-mDNS registration (`observer.port()`, set once in `Provide`);
  - Android NSD;
  - `spacecore.currentOwn()`, which feeds `LocalServer` in re-exchanges;
  - peers' recorded addresses.

  Keeping the port fixed avoids propagating changes through all of them. The app owns the port while foregrounded and closed it itself on background, so EADDRINUSE should be rare and transient; the worker retries. A port that stays taken is logged, and the next app launch falls back to a random port as today.
- **D3. Don't restore the QUIC listener.** Since GO-7424, local peers dial yamux only. Restoring QUIC would need an any-sync API to close a single listener and Transport.

  **Compatibility limit (stated, not a regression):** clients that predate GO-7424 (v0.50.19 and earlier; the version mapping is unverified) dial local peers over QUIC only and listen on UDP only. Once the iPhone's QUIC listener dies, which already happens today after the first suspension, the two can't reach each other in either direction until the iPhone app restarts. If that window matters, Q2 adds the any-sync closer API and rebinds QUIC in the same worker.
- **D4. Lifecycle events are primary; the probe and watchdog are safety nets.**
- **D5. iOS only.**

## Testing

**Unit tests, with seams:** an injectable `listen` func, an inner `net.Listener` interface (fakes return chosen errors), and the `lifecycle` gate settable in tests.

- **`lanListener` (with fakes):**
  - Accept survives `pause` and `rebind` without an error.
  - **A successful accept on a superseded generation is closed and not returned.** The fake returns a conn after `gen` advanced.
  - Accept returns `net.ErrClosed` after `Close`, including while waiting for an inner listener.
  - A non-temporary error on the current generation (fake EBADF) triggers the rebind request and doesn't surface.
  - Temporary errors are returned unchanged.
  - `rebind` after `Close` closes the new socket.
- **`lanListener` (real sockets, outside synctest):**
  - After `pause` then `rebind`, a TCP client connects to **the same port**.
  - `pause` refuses new connections.
- **`clientServer` worker** (synctest, fake `listen`):
  - Background pauses synchronously: the inner listener is closed before `StateChange` returns.
  - Foreground rebinds the same port.
  - Background → Foreground → Background in quick succession ends paused, and a slow rebind finishing late is closed, not installed.
  - A duplicate Foreground with a healthy listener does nothing.
  - Rebind retries back off and never pick another port.
  - A probe failure (injected) rebinds.
  - `StateChange` returns while the worker is blocked in `listen`.
  - With the gate off: no worker, no `StateChange` effect.
- **Startup:** a failing persistence still sets `ServerStarted()==true`.

**Mutations** must target observable failures:
- returning a superseded conn;
- pausing asynchronously (port still open when `StateChange` returns);
- falling back to a random port;
- dropping the desired-state re-check before install.

**On-device acceptance.** Use an iPhone running an app with the native-mDNS bridge and this fix, launched from the springboard and not under the Xcode debugger, with `cmd/lanprobe` on a Mac:

1. **SO_ERROR probe:** verified separately in the diagnostic build (Rollout step 1). Proactive pausing closes the socket before it can be defuncted, so the production build can't show it.
2. **Foreground:** lock 60 s, unlock. `nc 192.168.x.x 62654` succeeds within ~1 s of unlock, and lanprobe's `-fresh` dial succeeds. iOS logs `lan listener rebound` with the same port.
3. **Background:** while locked, connects get **refused** immediately, not a handshake timeout.
4. **Real sync, not just lanprobe:** Mac desktop app and iPhone share a space. Lock and unlock the iPhone, then edit an object on the Mac and open an attached file on the iPhone; both sync over LAN with the node blocked or offline. lanprobe's fake exchanges prove reachability, not sync.
5. **Stress:** 10 lock/unlock cycles. The port is never stuck refused in the foreground, and it never changes.
6. **Missed Background:** a debug toggle drops **both** lifecycle RPCs around one lock, so the recorded state stays foreground. After unlock, the watchdog finds the defunct socket (WARN `lan listener socket found dead`) and rebinds within 20 s. That line, with its errno, is also the on-device proof that the SO_ERROR probe works. A dropped Foreground alone is not recoverable by design (see Goal 3).

## Observability

- **INFO, visible in user builds** (`client.space.clientserver=INFO` was added to `logging.DefaultLogLevels`):
  - `lan listener paused for background`;
  - `lan listener rebinding`;
  - `lan listener rebound`, with the attempt count and how long it took.

  That's about two lines per lock cycle, enough to tell a missing transition from a successful rebind.
- **WARN:**
  - `lan listener socket found dead` (the probe or Accept found a dead socket without a pause of ours);
  - `lan listener socket died` (from Accept);
  - `lan listener rebind failed` (rate-limited).

## Rollout

1. **Diagnostic build** (iOS, no pause): log `probe()` on every Foreground for a few lock cycles. Confirm SO_ERROR reports EBADF for a defunct listener. Otherwise switch to the self-dial fallback.
2. **Heart PR:** `lanListener`, the iOS lifecycle worker, the startup fix and tests.
3. **Ship in the same app release as GO-7576 Part B** (the anytype-swift native mDNS). If this heart change ships first, iOS keeps its zeroconf fallback. That responder is still dead after a suspension, so this fix alone helps only peers that already know the address. It is still strictly better than today's permanently dead listener, never worse.
4. **On-device acceptance** as above.

## Open questions

- **Q1.** Does the iOS app hold `beginBackgroundTask` around `AppSetDeviceState(BACKGROUND)`? It doesn't today: anytype-swift's `SceneLifecycleStateService` sends it on `didEnterBackground` without an assertion. Correctness doesn't depend on it, because the pause is synchronous inside the RPC. The assertion would only add margin.
- **Q2.** Do we need iOS to accept QUIC from pre-GO-7424 clients (D3)? If yes, add the any-sync closer API and rebind QUIC in the same worker.
- **Q3.** Should the debug pprof server (`RunDebugServer`) get the same treatment, so on-device profiling survives a lock?

## Follow-ups (separate tickets)

- **Re-exchange retries.** A re-exchange after foreground can fail, for example when the recovery pool flush closes the connection it used. Nothing retries it until the platform reports the peer again. The discovery bridge (GO-7576) should retry a failed exchange, bounded, after the pool's connectivity hook.
- **Clear file-fetch bans on rediscovery.** rpcstore's local-peer strikes (GO-3192: 10 s doubling up to 5 min) are cleared only by expiry or a successful fetch. A peer rediscovered after foreground should be dialable again immediately.
- **Act on `ObserveLost`** (GO-7576): treat a lost peer as "don't dial" until it's rediscovered. Android already unregisters NSD on Activity pause, so this also stops peers wasting 5 s connect timeouts on a backgrounded Android, whose firewall drops connections instead of refusing them.
- **any-sync:** use `errors.Is(err, net.ErrClosed)` in the yamux and QUIC accept loops to quiet shutdown logs.
- **Kotlin bridge cleanups** (anytype-kotlin `middleware/.../discovery/`):
  - `stop()` skips `stopServiceDiscovery` and `lock.release()` if `unregisterService` throws;
  - a throwing `observeChange` never releases the resolve semaphore;
  - only a single `hostAddress` is reported;
  - NSD auto-rename on a name conflict would advertise the wrong peerId.

## References

- Research: `docs/superpowers/specs/2026-10-09-ios-socket-research.md` (TN2277, TN3179, Apple forum threads, XNU sources, status-go #7663, tailscale #21353/#21396).
- Evidence: lanprobe logs (`lan1.log`, Android run), iOS report `anytype-report-20261009-102107`, post-unlock stack dump `stack.20261009.102932.02.log`.
- Review: 3 Codex lenses (gpt-6-astra, max effort), 2026-10-09. Findings on the dropped mDNS sections are moot; the listener findings are incorporated above.
