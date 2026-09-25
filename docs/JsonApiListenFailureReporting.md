# JSON API listen status reporting — design

**Audience:** anytype-heart implementer; anytype-ts (and other clients) for the event/response contract.
**Type:** design spec. Implemented on `go-7537-space-lock-by-id`; kept as the design record.

## Background

The JSON API server (`core/api`, `Config.JsonApiListenAddr`) is started as a side effect of three
RPCs: `AccountSelect`, `AccountCreate` (initial bind, via the `api` component's `app.ComponentRunnable`
lifecycle) and `AccountChangeJsonApiAddr` (live port switch, via `apiService.ReassignAddress`, called
outside the component lifecycle after the app is already running).

Today, none of the three can report a bind failure (e.g. `"address already in use"`) to the caller:

```go
// core/api/service.go, before this feature (develop)
func (s *apiService) startServer() error {
    ...
    s.httpSrv = &http.Server{Addr: s.listenAddr, Handler: s.srv.Engine(), ...}
    go func() {
        if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            log.Errorf("API server error: %v", err)   // the only place the failure goes
        }
    }()
    return nil   // returned immediately, before the goroutine has attempted to bind
}
```

`ListenAndServe` — the call that actually does `net.Listen` — runs in a detached goroutine.
`startServer()` returns `nil` before that goroutine has run at all, so `Run(ctx)` (the component
lifecycle method), `ReassignAddress` and therefore `AccountSelect` / `AccountCreate` /
`AccountChangeJsonApiAddr` all report success regardless of whether the socket ever actually bound.
The only trace of a failure is the `log.Errorf` line. There is no error field for it in any of the
three RPC responses, no field on `AccountInfo`, and no event.

## Design decision

The three callers have different shapes, so they get different treatment — but they all describe
their outcome with **one shared model**, not three ad hoc ones:

- **`AccountChangeJsonApiAddr`** is a dedicated request/response call whose entire purpose is
  "(re)bind the JSON API to this address" — the bind result belongs in *its own* response, as a
  `status` field carrying the same shape the async event uses (see below). A failed bind is not an
  RPC-level error here: the RPC answered the question fully, the answer just says "no." `Error`
  stays reserved for things that prevented even attempting the bind (app not running, an unexpected
  shutdown failure of the previous listener).
- **`AccountSelect` / `AccountCreate`** exist to open the account; the JSON API is an optional side
  feature riding along (`JsonApiListenAddr` is `omitempty`, `""` disables it entirely). Failing the
  whole account-open over a JSON-API-only bind problem would be wrong, and `Run(ctx)`'s error is
  what `app.Start` uses to decide whether the app came up at all — the bind attempt must not gate it.
  → **`Run` still performs the bind synchronously and never returns a non-nil error for it** — "stays
  non-blocking" describes the *lifecycle outcome* (a bind failure can never fail account open), not
  the timing: `Run` waits for `net.Listen` to resolve before `app.Start` moves on to the next
  component. That synchronous wait is itself introduced by this change (`develop`'s `Run` returned
  immediately, the bind happening fully inside a detached goroutine) — see Background above. The
  outcome is only visible to callers via the async event.

Every bind attempt — success or failure, whether triggered by `Run` or by `ReassignAddress` —
broadcasts the same status as an event, so every open session learns the JSON API's current address
and health, not just the one that made the triggering call. `AccountChangeJsonApiAddr`'s response
additionally hands that same status directly back to its caller, synchronously, using the identical
proto type — one shape, two delivery paths, never two schemas to keep in sync.

## Protocol changes

### One new shared type — `Event.Account.JsonApiStatus`

Defined once in `pb/protos/events.proto`, inside `message Account { ... }` (next to `Recovery`,
`LinkApprovalRequest`) — this repo's existing pattern for a payload shared between the event stream
and an RPC response is to define it once under `Event.Account` and reference it by its fully
qualified name from `commands.proto`, which already imports `events.proto` and already does this
for `RecoveryState` (`anytype.Event.Account.Recovery.Snapshot snapshot = 2;`) and `Import.Statistic`:

```proto
message Account {
  ...
  message JsonApiStatus {
    bool success = 1;
    string listenAddr = 2;  // the effective, OS-confirmed bound address on success (ln.Addr().String());
                             // the requested address on failure, since nothing bound
    string error = 3;       // raw OS error text — debug/log only; empty when success = true
  }
}
```

Registered in the `Event.Message` oneof as `accountJsonApiStatus` (next free field after
`accountRecoveryUpdate = 206`):

```proto
Account.JsonApiStatus accountJsonApiStatus = 207;
```

Delivery: broadcast, `spaceId = ""` (account-level, not space-level — same as
`ensureAnalyticsEvent`'s `event.NewEventSingleMessage("", ...)` calls already in `core/api/server/middleware.go`).
Emitted on every `startServer()` attempt — from the initial `Run` at account select/create *and*
from every `ReassignAddress`. Not emitted when the server is disabled (`listenAddr == ""`).

### `AccountChangeJsonApiAddr` response gets the same type

`pb/protos/commands.proto`, `Rpc.Account.ChangeJsonApiAddr.Response` — no new error code needed;
`status` carries the bind outcome, `error` stays for RPC-level failures only:

```proto
message ChangeJsonApiAddr {
    message Request {
        string listenAddr = 1;
    }
    message Response {
        anytype.Event.Account.JsonApiStatus status = 1;  // new — same type the event carries; nil only
                                                           // when listenAddr == "" (disabling) or error != NULL
        Error error = 2;                                  // unchanged: NULL, UNKNOWN_ERROR, BAD_INPUT,
                                                           // ACCOUNT_IS_NOT_RUNNING

        message Error {
            Code code = 1;
            string description = 2;
            enum Code {
                NULL = 0;
                UNKNOWN_ERROR = 1;
                BAD_INPUT = 2;
                ACCOUNT_IS_NOT_RUNNING = 4;
            }
        }
    }
}
```

## Middleware implementation plan

### `core/api/service.go`

`startServer()` no longer returns a Go `error` for a bind failure — that outcome is a value
(`status.success == false`), not an exceptional condition. Everything that mutates `apiService`'s
state (`bindLocked`, `shutdownLocked`) is written as a "must be called with `s.lock` held" helper, and
the two public entry points (`startServer`, `ReassignAddress`) take the lock once for their *entire*
operation — including, for `ReassignAddress`, the shutdown of whatever was there before:

```go
func (s *apiService) startServer(listenAddr string) *pb.EventAccountJsonApiStatus {
    s.lock.Lock()
    defer s.lock.Unlock()

    status := s.bindLocked(listenAddr)
    if status != nil {
        s.publishStatus(status)
    }
    return status
}

// bindLocked does the actual (re)bind. Must be called with s.lock held.
func (s *apiService) bindLocked(listenAddr string) *pb.EventAccountJsonApiStatus {
    s.listenAddr = listenAddr
    if listenAddr == "" {
        log.Info("API server disabled (no listen address)")
        return nil
    }

    ln, err := net.Listen("tcp", listenAddr)
    if err != nil {
        return &pb.EventAccountJsonApiStatus{Success: false, ListenAddr: listenAddr, Error: err.Error()}
    }

    closeListener := true // guards against leaking ln if server.NewServer panics below
    defer func() {
        if closeListener {
            _ = ln.Close()
        }
    }()

    s.srv = server.NewServer(...)
    httpSrv := &http.Server{Handler: s.srv.Engine(), ReadHeaderTimeout: readTimeout}
    s.httpSrv = httpSrv
    s.listener = ln

    status := &pb.EventAccountJsonApiStatus{Success: true, ListenAddr: ln.Addr().String()}
    log.Infof("Starting API server on %s", status.ListenAddr)

    go func() {
        // httpSrv, not s.httpSrv: this goroutine outlives s.lock, and a
        // later bindLocked call can overwrite s.httpSrv before this line
        // runs — a live field read would then serve ln through the wrong
        // (newer) server.
        if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
            log.Errorf("API server error: %v", err)
        }
    }()

    closeListener = false
    return status
}

// publishStatus dispatches Broadcast on its own goroutine, unconditionally.
// Every caller of startServer/ReassignAddress holds s.lock for the whole
// call, and AccountSelect/AccountCreate/AccountChangeJsonApiAddr additionally
// hold application.Service's own lock for their whole duration. Broadcast
// delivers synchronously to the client callback on the mobile/library
// sender, so a client that reacts to this event by calling back into this
// service (retrying AccountChangeJsonApiAddr, or calling AccountStop) would
// deadlock on either lock if this ran inline.
func (s *apiService) publishStatus(status *pb.EventAccountJsonApiStatus) {
    go s.eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountJsonApiStatus{
        AccountJsonApiStatus: status,
    }))
}
```

`Run` (component lifecycle — truly never fails over this now; the event already covers telling
anyone who's listening):

```go
func (s *apiService) Run(ctx context.Context) error {
    s.startServer(s.listenAddr)
    return nil
}
```

`ReassignAddress` shuts down whatever is currently bound and binds the new address as **one**
operation under `s.lock`, not two separately-locked steps. `AccountChangeJsonApiAddr`'s caller only
takes a read lock at the application layer, so nothing stops two reassignments from racing; without
this, the loser's shutdown+bind could interleave with the winner's and overwrite the winner's still-
live `s.httpSrv`/`s.listener`, orphaning its listener with nothing left able to track or close it —
found and reproduced in review, alongside the field-capture fix above:

```go
func (s *apiService) shutdownLocked(ctx context.Context) error {
    httpSrv := s.httpSrv
    listener := s.listener
    s.httpSrv = nil
    s.listener = nil

    if httpSrv == nil {
        return nil
    }

    shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
    defer cancel()

    err := httpSrv.Shutdown(shutdownCtx)
    if listener != nil {
        _ = listener.Close() // Shutdown alone can't close a listener Serve hasn't registered yet
    }
    if err != nil {
        return fmt.Errorf("close previous http server: %w", err)
    }
    return nil
}

func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) (*pb.EventAccountJsonApiStatus, error) {
    s.lock.Lock()
    defer s.lock.Unlock()

    if err := s.shutdownLocked(ctx); err != nil {
        return nil, fmt.Errorf("shutdown server: %w", err)
    }

    status := s.bindLocked(listenAddr)
    if status != nil {
        s.publishStatus(status)
    }
    return status, nil
}
```

`shutdownHTTP` (used only by `Close`, which has nothing to bind afterward) is the thin, lock-taking
wrapper around `shutdownLocked`.

**Event ordering is not guaranteed, full stop.** The bind itself is atomic (one `s.lock`-held
operation per `startServer`/`ReassignAddress` call), but `publishStatus`'s dispatch is fire-and-forget
(`go s.eventService.Broadcast(...)`) — this is what makes it safe to call while holding a lock (see
above), but it also means the order two events are *delivered* in is not guaranteed to match the
order the two binds *completed* in. This applies to any two bind attempts on the same instance,
overlapping or not, and to the very first bind (`Run`) racing a fast subsequent `ReassignAddress`
just as much as two reassignments racing each other. **This is not self-correcting**: if B's bind
happens after A's but B's event is delivered first, and no further bind ever occurs, a client that
only trusts "the latest event I received = current state" is left believing A's (stale) status
indefinitely — nothing forces a corrective event afterward. A client that needs a strict ordering
guarantee cannot get one from this event alone and would need to poll or otherwise reconcile state;
for the intended use (surfacing bind failures, showing the current port on a settings screen) treating
each event as "informational, may occasionally be stale under rapid changes" is enough.

`Service` interface signature changes accordingly:

```go
type Service interface {
    app.ComponentRunnable
    ReassignAddress(ctx context.Context, listenAddr string) (*pb.EventAccountJsonApiStatus, error)
    RevokeToken(token string)
}
```

### `core/application/account_config_update.go`

```go
func (s *Service) AccountChangeJsonApiAddr(ctx context.Context, addr string) (*pb.EventAccountJsonApiStatus, error) {
    s.lock.RLock()
    defer s.lock.RUnlock()
    if s.app == nil {
        return nil, ErrApplicationIsNotRunning
    }
    apiService := app.MustComponent[api.Service](s.app)
    status, err := apiService.ReassignAddress(ctx, addr)
    if err != nil {
        return nil, fmt.Errorf("reassign json api address: %w", err)
    }
    return status, nil
}
```

### `core/account.go`

```go
func (mw *Middleware) AccountChangeJsonApiAddr(ctx context.Context, req *pb.RpcAccountChangeJsonApiAddrRequest) *pb.RpcAccountChangeJsonApiAddrResponse {
    status, err := mw.applicationService.AccountChangeJsonApiAddr(ctx, req.ListenAddr)
    code := mapErrorCode(err,
        errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountChangeJsonApiAddrResponseError_ACCOUNT_IS_NOT_RUNNING),
    )
    return &pb.RpcAccountChangeJsonApiAddrResponse{
        Error:  &pb.RpcAccountChangeJsonApiAddrResponseError{Code: code, Description: getErrorDescription(err)},
        Status: status,
    }
}
```

`status` is `nil` on this response exactly when: the request disabled the server (`listenAddr == ""`),
or `error.code != NULL` (the bind was never attempted at all).

## Client contract (summary)

- **`AccountChangeJsonApiAddr`**: read `response.status` first. `status.success == true` means the
  new address is live (`status.listenAddr` is the effective bound address — use it, don't assume it
  equals what you sent, in case a future ephemeral-port mode is added). `status.success == false`
  means the switch failed; `status.error` is raw debug text, not user-facing copy. `status == nil`
  with `error.code == NULL` means the server was disabled (empty `listenAddr`); `status == nil` with
  a non-NULL `error.code` means the switch was never attempted (e.g. `ACCOUNT_IS_NOT_RUNNING`).
- **The RPC response and the broadcast event for that same call have no defined ordering.**
  `publishStatus` dispatches the broadcast on its own goroutine and neither
  `AccountChangeJsonApiAddr`'s handler nor the application layer waits for it, so the event for your
  own call can arrive before or after your own RPC response. For the outcome of the call **you**
  made, trust `response.status` — it's authoritative and already synchronous. Use the event stream
  only for learning about changes made by *other* sessions, or for a general "what's the JSON API's
  state right now" signal.
- **`AccountSelect` / `AccountCreate`**: unchanged. Continue to block and return exactly as before.
- **New:** subscribe to `Event.Message.accountJsonApiStatus` — same fields as the RPC's `status`. It
  fires after the initial bind (success or failure) and after every later `AccountChangeJsonApiAddr`,
  regardless of which session triggered it. A client with a JSON-API-related UI (local API keys,
  MCP/task-tool surface, a settings page showing the port) should track the latest one; a client with
  no such UI can ignore the event entirely.
- Ignore unknown event fields/kinds as usual (forward compatibility).

## Open questions

- **`ReassignAddress` has no rollback.** It already shuts down the current server *before* trying to
  bind the new address (pre-existing behavior, not introduced by this change) — so a failed bind on
  the new address leaves the JSON API down entirely, not still serving the old one. `status.success =
  false` correctly reports that, but the practical effect (API down) is the same either way: worth
  fixing alongside this (attempt the new bind before tearing down the old listener) but is a
  separate, slightly bigger change; flagging so it isn't assumed to be in scope here silently.
- **Proto field number** (`accountJsonApiStatus = 207`, and `status = 1` on
  `ChangeJsonApiAddr.Response`) are the next free slots as of this writing — reconfirm against
  `develop` at merge time in case another in-flight branch claimed them first.
- **`publishStatus` spawns one goroutine per bind attempt, unbounded.** Normal usage (occasional
  account open, occasional manual port change) never gets close to this mattering. A pathological
  client that both (a) hammers `AccountChangeJsonApiAddr` in a tight loop and (b) has an event
  callback that blocks forever would accumulate one goroutine per call indefinitely (reproduced in
  review: 256 rapid failed reassignments → 256 retained goroutines). A callback that never returns is
  already a client bug independent of this feature; not fixing it here to avoid adding a bounded
  dispatcher/queue for a misuse scenario, but flagging in case usage patterns change.

## Non-goals

- No automatic retry/backoff on bind failure.
- No ephemeral-port fallback (e.g. listening on `:0` and reporting back whatever the OS picked) —
  though `listenAddr` already reports `ln.Addr().String()` rather than echoing the config string, so
  this would slot in later without another protocol change.
- No change to `/v1`/`/v2` route behavior, auth, or any other part of the JSON API surface.
