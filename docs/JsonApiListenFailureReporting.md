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
// core/api/service.go, current code
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
  → **stays non-blocking**; the outcome is only visible via the async event.

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
(`status.success == false`), not an exceptional condition. It publishes the event itself and returns
the same status struct, so every caller (the component lifecycle and `ReassignAddress`) reads it off
one place:

```go
func (s *apiService) startServer() *pb.EventAccountJsonApiStatus {
    s.lock.Lock()
    defer s.lock.Unlock()

    if s.listenAddr == "" {
        log.Info("API server disabled (no listen address)")
        return nil
    }

    ln, err := net.Listen("tcp", s.listenAddr)
    if err != nil {
        status := &pb.EventAccountJsonApiStatus{Success: false, ListenAddr: s.listenAddr, Error: err.Error()}
        s.publishStatus(status)
        return status
    }

    s.srv = server.NewServer(...)
    s.httpSrv = &http.Server{Handler: s.srv.Engine(), ReadHeaderTimeout: readTimeout}

    status := &pb.EventAccountJsonApiStatus{Success: true, ListenAddr: ln.Addr().String()}
    log.Infof("Starting API server on %s", status.ListenAddr)
    s.publishStatus(status)

    go func() {
        if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
            log.Errorf("API server error: %v", err)
        }
    }()
    return status
}

func (s *apiService) publishStatus(status *pb.EventAccountJsonApiStatus) {
    s.eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountJsonApiStatus{
        AccountJsonApiStatus: status,
    }))
}
```

`Run` (component lifecycle — truly never fails over this now; the event already covers telling
anyone who's listening):

```go
func (s *apiService) Run(ctx context.Context) error {
    s.startServer()
    return nil
}
```

`ReassignAddress` now returns the status directly to its caller, alongside a Go `error` reserved for
genuine RPC-level failures (shutting down the previous listener):

```go
func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) (*pb.EventAccountJsonApiStatus, error) {
    if err := s.shutdownHTTP(ctx); err != nil {
        return nil, fmt.Errorf("shutdown server: %w", err)
    }
    s.listenAddr = listenAddr
    return s.startServer(), nil
}
```

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
    return apiService.ReassignAddress(ctx, addr)
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

## Non-goals

- No automatic retry/backoff on bind failure.
- No ephemeral-port fallback (e.g. listening on `:0` and reporting back whatever the OS picked) —
  though `listenAddr` already reports `ln.Addr().String()` rather than echoing the config string, so
  this would slot in later without another protocol change.
- No change to `/v1`/`/v2` route behavior, auth, or any other part of the JSON API surface.
