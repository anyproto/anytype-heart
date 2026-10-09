# iOS socket defuncting ("socket resource reclaim") and Go: research notes

Date: 2026-10-09. Scope: anytype-heart on iOS through gomobile. Covers the TCP listener on :62654, the QUIC UDP socket, mDNS multicast sockets, and outgoing connections.

Legend: **[DOC]** comes from Apple documentation or a DTS engineer's answer. **[SRC]** I read it in XNU or Go source. **[3P]** a third-party report. **[INF]** my own inference, not verified on a device.

---

## 1. What iOS does to sockets on suspension

### Apple's statements
- **[DOC] TN2277.** If an app is suspended, "the socket's resources might get reclaimed by the kernel, after which all networking operations on the socket will fail." The exact conditions are "purposely not documented". The error is also "purposely not specified", but "in many cases the error will be `EBADF`". On listeners: "If the system suspends your app and then, later on, reclaims the resources from underneath your listening socket, your app will no longer be listening for connections, even after it has been resumed. The app may or may not be notified of this." https://developer.apple.com/library/archive/technotes/tn2277/_index.html
- **[DOC] Quinn (DTS), 2025.** The modern term is "defuncted": "Modern systems are a lot more aggressive about defuncting than old systems… The system will defunct stuff immediately on suspending your app." In Network.framework the defunct shows up as `.failed(…)`. An `NWListener` registered with Bonjour fails on suspend, because its IPC connection to mDNSResponder is defuncted. An `NWListener` without Bonjour is currently *not* defuncted, but that is "the current implementation, not part of the API contract". His advice is to stop listeners whenever the app *becomes eligible for suspension*. https://developer.apple.com/forums/thread/840808
- **[DOC] Quinn, Aug 2025.** "Recent versions of iOS defunct on app suspend, so you don't have to explicitly lock the device." https://developer.apple.com/forums/thread/795697
- **[DOC] Quinn, 2017.** A BSD listening socket started failing `accept()` with ECONNABORTED after time in the background. Quinn: "very likely that your listening socket's resources have been reclaimed… close your listening socket before the app becomes eligible for suspension and then re-open it". https://developer.apple.com/forums/thread/85038
- **[DOC]** A defuncted socket raises SIGPIPE on write, so set SO_NOSIGPIPE. https://developer.apple.com/forums/thread/52744
- **[DOC]** A Network.framework QUIC flow logged `Receive failed with error "Socket is not connected"` after resume. Quinn identified this as socket resource reclaim and recommended retrying on error. https://developer.apple.com/forums/thread/724950

### Timing
- **[DOC]** Quinn says sockets are defuncted "immediately on suspend". The policy itself lives in userspace (runningboard/assertiond) and is not documented.
- **[3P]** A libtailscale user reports the socket is reclaimed about 30 s after the device locks. https://github.com/tailscale/tailscale/issues/21353
- Our observation #5 fits this. After the app is suspended there is a window in which the kernel still completes TCP handshakes on the listener but nobody accepts, so clients hang on TLS. The defunct then follows and clients get RST.

### What the kernel actually does (XNU `bsd/kern/uipc_socket.c`, `uipc_syscalls.c`, `netinet/*`) [SRC]
Source: https://github.com/apple-oss-distributions/xnu (main branch)

`sosetdefunct()` sets `SOF_DEFUNCT` and `SB_DROP` on both buffers. `sodefunct()` then does the following:
1. It calls `pru_defunct`. For UDP, `udp_defunct()` **drops every multicast membership** (`inp_moptions = NULL`).
2. It calls `soshutdownlock_final(SHUT_RD)` and `(SHUT_WR)`. For a TCP *listener*, `pru_shutdown` reaches `tcp_usrclosed`, which for `TCPS_LISTEN` calls `tcp_close()`. The listening PCB is gone, so **incoming SYNs get RST** (our symptom #1).
3. It calls `sodisconnectlocked()`, then `soisdisconnected()` for connectionless sockets.
4. It sets `so_error = EBADF` if it was 0, and marks the socket `SS_DEFUNCT`.

The file descriptor stays valid in the process. Per-syscall results afterwards:

| Operation on a defunct socket | Result |
|---|---|
| `recv`/`recvmsg`/`recvfrom` (`soreceive`) | **`ENOTCONN`** immediately, every call. This is our QUIC and mDNS error. |
| `send*` (`sosend`) | `EPIPE` (+ SIGPIPE unless SO_NOSIGPIPE) |
| `connect` | `EOPNOTSUPP` |
| `bind` | `EINVAL` |
| `listen` | `EINVAL` (the same fd can't be re-listened) |
| non-blocking `accept`, empty queue | **`EWOULDBLOCK`**. The `SS_NBIO` check runs *before* `so_error` / `SS_CANTRCVMORE` is examined. |
| non-blocking `accept`, queue non-empty | returns `so_error`, which is **`EBADF`** |
| kqueue `EVFILT_READ` on a listener | readiness is `!TAILQ_EMPTY(&so->so_comp)` and *nothing else*. EOF and errors are ignored. |
| kqueue `EVFILT_READ` on a UDP/TCP data socket | fires with `EV_EOF` (`SS_CANTRCVMORE`), so the socket is permanently "readable" |
| `getsockopt(SO_ERROR)` | returns `EBADF` once (it reads and clears `so_error`). Healthy listeners return 0. |
| `getsockopt(SO_ISDEFUNCT=0x1101)` | 1 when defunct. This is a **private** constant from `socket_private.h` and is not in the SDK. |

### Our observations explained [SRC + INF]
1. **Listener Accept parks forever.** Go's `poll.FD.Accept` calls `accept` and gets `EAGAIN` because the queue is empty, so it parks in netpoll. Kqueue only reports a listener readable when its completed-connection queue is non-empty. The defunct listener's PCB is closed, so nothing ever enqueues and the goroutine never wakes. This is a structural silent hang, not a lost event. Tailscale's patch states the same thing: "a defunct listening socket never becomes readable, so Accept blocks on it forever". The status-go team saw it on device ("the Go accept loop never return[s] an error"). https://github.com/status-im/status-go/pull/7663
2. **The pprof listener did error out [INF].** It most likely had a connection sitting in its accept queue at defunct time, for example a handshake the kernel completed while the app was suspended. On resume kqueue fires, `accept` skips the wait loop, and it returns `head->so_error = EBADF`. Go does not retry EBADF (only EINTR, EAGAIN and ECONNABORTED), so `http.Server.Serve` exits. In short, **the listener errors if something was queued and hangs if nothing was.**
3. **QUIC `recvmsg: socket is not connected`.** `soreceive` returns ENOTCONN for defunct sockets. In Go `syscall.Errno.Temporary()` is false for ENOTCONN. quic-go v0.63 `Transport.listen()` calls `t.close(err)` on any non-temporary read error, so the Transport and every connection on it are dead for good.
4. **mDNS busy-spin.** Kqueue reports the defunct UDP socket as readable forever (`EV_EOF`), and `recvmsg` returns ENOTCONN forever. Retrying on the same fd can never recover, and backoff only slows the spin. The multicast group memberships are gone as well.

Note on `ECONNABORTED`: Go's `internal/poll/fd_unix.go` Accept loop silently `continue`s on ECONNABORTED (see golang/go#3395, #6163). The old pattern from forum thread 85038 would therefore be swallowed in Go. That doesn't matter with today's kernel path, which is EAGAIN and then a park.

---

## 2. Apple's recommended practice

- **[DOC] Listeners.** "Close the listening socket when it goes into the background and reopen it when it comes back into the foreground… closing a listening socket is always fast", and "stop any Bonjour registrations for that socket" (TN2277). Quinn repeats this in 2025. Closing gives clients an immediate "connection refused" instead of a hang. The trigger is becoming *eligible for suspension*, not merely entering the background: while a `beginBackgroundTask` assertion is held, or audio is playing, the app is not suspended.
- **[DOC] Data sockets and connections.** Either close them proactively or "be prepared" for the defunct and retry. Quinn leans towards retrying.
- **[DOC] Notification API.** There is none for BSD sockets. Network.framework reports a defunct as a `.failed` state on `NWConnection`, and on `NWListener` when it has Bonjour registered. For BSD sockets you only see the errors in the table above.
- **[DOC] Bonjour, mDNS and the multicast entitlement** (TN3179, https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy):
  - On iOS, sending or receiving UDP multicast or broadcast requires `com.apple.developer.networking.multicast`. That covers our own zeroconf responder and browser.
  - Bonjour through the system (DNS-SD, `NWListener.service`, `NWBrowser`) needs only Local Network permission and `NSBonjourServices` for declared types. It does not need the multicast entitlement, except for arbitrary service types or `_services._dns-sd._udp` browsing.
  - Quinn: "you've implemented your own mDNS / DNS-SD responder rather than using the system's built-in one. I *strongly* recommend against doing that… If you implement your own responder then you'll need to use multicast and that requires the multicast entitlement." He recommends a platform layer that calls the system DNS-SD where it exists. The poster in that thread fixed the issue by moving to the system DNS-SD. https://developer.apple.com/forums/thread/685181
  - mDNSResponder already owns UDP 5353 on iOS, and an `NWListener` on 5353 gets EADDRINUSE. https://developer.apple.com/forums/thread/691810
  - For the low-level API, DTS points to DNS-SD (`dns_sd.h`, DNSServiceRegister/Browse/Resolve). Its DNSServiceRegister takes **any port**, so it can advertise a socket owned by Go. https://developer.apple.com/forums/thread/689542 and https://developer.apple.com/forums/thread/742122
- **[DOC] Local Network privacy on resume.**
  - Listening and accepting TCP needs no Local Network permission.
  - Receiving UDP unicast needs none.
  - Sending UDP unicast, outgoing TCP to the LAN, and sending or receiving multicast all need it.
  - "If an iOS app is in the background and performs a local network operation while its Local Network privilege is undetermined, the system denies that operation without presenting the alert". It doesn't record the denial, and the alert appears on the next foreground attempt. So don't start the first LAN operations from a background launch and treat their failure as final.
  - With DNS-SD, a denial surfaces as `kDNSServiceErr_PolicyDenied` (-65570).
- **[DOC] Background time.** `beginBackgroundTask` gives "about 30 seconds" on current systems, with no guarantee. The expiry handler must finish in under about 1 s and runs on the main thread. https://developer.apple.com/forums/thread/85066 Closing sockets and sending mDNS goodbyes takes milliseconds, so do it synchronously when the app becomes eligible for suspension.

---

## 3. What other Go-on-iOS projects do

- **golang/go.** I found no issue that tracks the defunct-listener hang. Related history:
  - #17393: gomobile apps crashed with SIGPIPE after resume. Fixed in gomobile around 2017. Go now turns SIGPIPE on non-stdout fds into `EPIPE` on Go threads, but a SIGPIPE on a non-Go thread is forwarded to the host's handler.
  - #3395 and #6163: Accept treats ECONNABORTED as retryable.
  - #54529: Accept hangs on macOS, still open and unrelated in cause.
- **Tailscale.**
  - #21353 (open, 2026-09): libtailscale "never recovers" after iOS reclaims sockets. The proposed patch includes a "revivingListener" that self-dials and re-listens on the same address, reopens the route socket when a read fails, and makes magicsock rebind its UDP socket on read errors (ENOTCONN), backing off instead of exiting.
  - #19504 and PR #21396: magicsock ReceiveIPv4 dies for good on iOS after a read error. The fix keeps the receive func alive and does a throttled `Rebind()`.
  - Tailscale's long-standing send-path rule is `maybeRebindOnError`, which rebinds on EPIPE or ENOTCONN.
  - https://github.com/tailscale/tailscale/issues/21353, https://github.com/tailscale/tailscale/pull/21396, https://github.com/tailscale/tailscale/issues/19504
- **status-go (Status app).**
  - PR #7373 added an app-wide pause/resume lifecycle (`AppStateChange` → `ToBackground`/`ToForeground`) and made the media server reuse the same port after a restart.
  - PR #7663: the lifecycle alone was not enough. When the app was suspended before login, or cold-started in the background, nothing drove the resume. Their fix is a **1 s self-dial** in `Start()`. If nothing answers, they force-close the old server, wait for the serve goroutine, and rebind on the **same cached port**. It was validated on an iPhone.
  - https://github.com/status-im/status-go/pull/7373, https://github.com/status-im/status-go/pull/7663
- **quic-go, libp2p, berty, syncthing, zeroconf.** I found nothing specific. quic-go has no iOS-resume handling, and any non-temporary read error closes the Transport (transport.go `listen()`). The answer there is to recreate the `net.UDPConn` and the `quic.Transport`.

---

## 4. Recommendations for anytype-heart

### (a) Close and rebind on lifecycle events. Yes, this is primary.
- On `AppWentBackground`, close the TCP listener, the QUIC Transport/listener and its UDP socket, and the mDNS server and client (sending goodbye first). Mark LAN discovery as paused.
- Ideally the Swift side sends this when the app becomes *eligible for suspension*, that is, when it ends its last background task. In practice `didEnterBackground` is fine: LAN peers can't be served reliably once we may be suspended.
- On `AppWentForeground`, recreate all of them. Treat outgoing peer and node connections as expendable: close them proactively or let them fail, then redial.
- Make rebinding idempotent. `Start` on an already-running component must *verify* liveness (see (c)) and not trust a `running` flag. This is the status-go lesson.

### (b) Keeping the same port
- Go already sets `SO_REUSEADDR` on every TCP listener on darwin (`net/sockopt_bsd.go`). DTS confirmed that SO_REUSEADDR is what lets a re-listen succeed past `TIME_WAIT` connections. https://developer.apple.com/forums/thread/75997
- **Close the old fd before binding a new one [SRC+INF].** The defunct listener's PCB is `tcp_close`d but stays attached to the socket while the fd is open (`so_usecount > 0`). `in_pcblookup_local_and_cleanup` only disposes it once the socket has no references, so binding while the old fd is open may give EADDRINUSE.
- `ln.Close()` in Go is synchronous: `poll.FD.Close` waits on `csema` until the sysfd is really closed. Close, then wait for the accept goroutine to exit, then `net.Listen`. Retry EADDRINUSE a few times with a short backoff.
- For UDP 62654, Go doesn't set SO_REUSEADDR on non-multicast UDP. Close first, then rebind, the same way.
- mDNS sockets on 224.0.0.251:5353 get SO_REUSEADDR and SO_REUSEPORT from Go's multicast path. Each rebind must re-join the groups, because the defunct dropped the memberships.

### (c) Detecting dead sockets without lifecycle events
Lifecycle events are not enough: cold background launches, missed events, and suspension during a background task all slip through. Add the following:
- **UDP/QUIC/mDNS.** Treat `ENOTCONN` from `ReadFrom`/`ReadMsgUDP` on an *unconnected* UDP socket as conclusive proof of a defunct socket. Never retry on the same fd. Tear down and recreate the socket, quic Transport, and multicast joins. Rate-limit recreation, for example 100 ms growing to 5 s, so that a genuinely broken network doesn't cause churn. `EPIPE` on write is the same signal. **[SRC]**
- **TCP listener, cheapest probe.** Run `rc.Control(func(fd){ getsockopt(fd, SOL_SOCKET, SO_ERROR) })` through `(*TCPListener).SyscallConn()`. A defunct listener returns `EBADF` (set by `sodefunct`). It is a public API, needs no network traffic, and costs microseconds. Caveat: SO_ERROR is read-and-clear, so the first probe consumes it, and you must rebind on that same result. **[SRC, verify on device]**
- **More precise but private.** `getsockopt(SOL_SOCKET, 0x1101 /*SO_ISDEFUNCT*/)` returns 1 when defunct and does not clear anything. It is private SPI (not in the SDK headers). It is only a constant, but don't rely on it for App Store safety.
- **Portable fallback.** Self-dial `127.0.0.1:port` (or the LAN IP) with a 1 s timeout, as status-go and tailscale do. Connection refused on a port we think we're listening on means the listener is dead.
- **When to probe.** On `AppWentForeground` (rebind anyway), on network-change events from the existing netmon or recovery pipeline (GO-3958), and with a cheap periodic watchdog every 15–30 s while in the foreground. Also probe before advertising via mDNS, so we never advertise a dead port.
- **Wake a parked Accept.** `ln.Close()` does wake it. `poll.FD.Close` calls `pd.evict()`, which unblocks netpoll waiters with `ErrNetClosing`, and then closes the fd once the last ref drops. Closing a defunct fd is fine. **[SRC]**

### (d) mDNS
- On background, call `server.Shutdown()`. zeroconf sends TTL=0 goodbyes, which takes milliseconds. Then close the browser sockets.
- If the Swift side wants a guarantee, wrap the `AppWentBackground` RPC in `beginBackgroundTask` / `endBackgroundTask`. About 30 s is available, and we need well under 1 s.
- On foreground, rebuild both the server and the client with fresh sockets and fresh group joins. In the recv loops, a read error of ENOTCONN or EBADF should end the loop and trigger a rebuild of that socket, not back off on the same fd.
- Peers that miss the goodbye (for example on a Wi-Fi drop) rely on record TTL. Keep it modest.

### (e) Move mDNS (and maybe the listener) to the platform? Recommended for mDNS; the listener can stay in Go.
**mDNS → system DNS-SD.** This removes our own multicast responder on iOS, which Apple discourages. It also removes the multicast-entitlement dependency (only `NSBonjourServices` with our `_anytype._tcp`-style type is needed), the competition with mDNSResponder for 5353, and the ENOTCONN spin class of bugs. There are two ways to do it:
- **Option 1:** Swift uses `NWListener.service`, or better `DNSServiceRegister(port: 62654)`, so that Go keeps owning the socket. It browses with `NWBrowser` or `DNSServiceBrowse` and pushes discovered peers and addresses into Go through the existing gomobile callback.
- **Option 2 [INF]:** Go calls `dns_sd.h` directly through cgo. It is in libSystem on iOS, and gomobile builds already use cgo. This keeps the logic in heart behind a build tag.

In both cases the DNS-SD connection to mDNSResponder is itself defuncted on suspend (Quinn: a Bonjour-registered NWListener fails for exactly that reason). So deregister on background and re-register on foreground. Treat `kDNSServiceErr_ServiceNotRunning` / `kDNSServiceErr_DefunctConnection` from `DNSServiceProcessResult` as a signal to recreate the registration. Handle `kDNSServiceErr_PolicyDenied` (-65570) to detect a Local Network denial.

**TCP and QUIC listeners can stay in Go.** Network.framework's `NWListener` without Bonjour is currently not defuncted and does notify through `.failed`. But it would mean bridging every accepted connection into Go, which is a large change, and Apple still says to stop listeners on suspension. Go with close/rebind, a liveness probe, and same-port rebinding.

---

## 5. Pitfalls checklist
- Rebinding while the old defunct fd is still open risks **EADDRINUSE**, because the old PCB stays referenced. Always `Close()` and join the accept goroutine first. Go's Close is synchronous. **[SRC/INF]**
- `Close()` does wake a goroutine parked in Accept or Read on a defunct fd, with `net.ErrClosed`. Make loops treat `net.ErrClosed` as "stop" and everything else as "rebuild". **[SRC]**
- A listener that answers RST at the kernel level while Go thinks it's running gives no in-process signal at all. Only a probe or a lifecycle event will catch it.
- Do not retry reads on a defunct UDP socket. The error is permanent and kqueue reports it readable forever. That is the source of the 1.6M recvmsg/s.
- Multicast memberships are dropped by the defunct. Recreate the socket rather than calling `JoinGroup` on the old one; `bind`, `connect`, and `listen` on a defunct fd return EINVAL or EOPNOTSUPP.
- An in-flight write on a defunct socket gets **EPIPE/SIGPIPE**. Go handles SIGPIPE on Go threads. If any C or Swift code writes to these fds, set `SO_NOSIGPIPE`.
- Local Network privilege can be undetermined at a background launch. LAN operations attempted then are silently denied, and the alert is deferred, so retry on foreground. Listening and receiving unicast are exempt.
- Don't test this under Xcode's debugger: it prevents suspension, so defuncting never happens. Test by detaching or launching from the springboard. **[DOC, thread 724950]**
- The exact errors and timing are explicitly unspecified by Apple and may change. Code against "any error, or a failed probe, means rebuild", not against specific errnos. Use ENOTCONN and EBADF as fast-path hints only.
- This applies to any socket kept across suspension, including the local HTTP gateway (already handled), the pprof server, and the debug listeners.
