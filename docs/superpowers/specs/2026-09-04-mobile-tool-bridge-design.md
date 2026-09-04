# In-process tool bridge for mobile clients — design

2026-09-04. Decisions: the wrapper's task tools reach mobile clients through
a gomobile bridge; the wrapper stays a client of `/v2` but its transport is
an in-memory call into the gin engine, so nothing listens on the device;
the bridge authorizes as the current account with Full scope through a
per-process internal key that exists only in memory. The as-built behavior
is to be recorded in `core/api/APIV2.md` §8 alongside the other deliveries.

## Goals

- anytype-swift (and, for free, anytype-kotlin) can run an on-device model
  against the `core/api/wrapper` tool table: list the tools of a tier, call
  a tool by name with JSON arguments, get back the text the model reads
  and the machine shape the app renders.
- No HTTP listener on mobile, ever. No port, no socket, no key the user has
  to manage.
- No blocked client thread per call: the call returns immediately and the
  result arrives through a callback, the way events already do.
- One tool definition, one execution path. The wrapper, the `/v2` handlers,
  the scope gates, the idempotency store, dry-run and the C6 error envelopes
  run unchanged. The bridge adds a transport and a credential, nothing else.

## Non-goals

- A direct in-process backend over `v2service.Service` that bypasses the
  handler layer. Measured: 23 wrapper call sites and 38 HTTP-coupled spots
  would need an abstraction, and roughly 1.5–2k lines of handler and
  middleware logic (space refs, id and key shaping, idempotency, dry-run,
  If-Match, error envelopes) would be duplicated — the third surface the
  wrapper's own design record forbids. Declined.
- Streaming routes. The in-process transport buffers whole responses; the
  chat SSE stream is not in the tool table and is not supported through it.
- Cancellation of an in-flight tool call. Calls are short; a later
  `ToolsCancel` can be added without changing the envelope.
- Any change to the desktop delivery: the HTTP listener, the CLI and the
  MCP stdio server keep working exactly as today.
- The Swift side itself. Its contract is stated in §9 so the two repos can
  be built independently.

## 1. Transport: in-memory dispatch into the gin engine

`core/api/server/inproc.go`:

```go
// InProcessBaseURL is the base URL the in-process transport answers for.
// The host is an IP literal so the origin policy's Host check admits it.
const InProcessBaseURL = "http://127.0.0.1"

// NewInProcessTransport returns a RoundTripper that serves each request
// by calling the engine's ServeHTTP directly — no listener, no socket.
// resolve is called per request so a rebuilt engine (ReassignAddress on
// desktop) is picked up, and returns the bearer key the transport sets.
func NewInProcessTransport(resolve func() (handler http.Handler, bearer string, err error)) http.RoundTripper
```

Per request the transport clones the request, sets `RemoteAddr` to
`127.0.0.1:0` (the write rate limiter keys on it), sets
`Authorization: Bearer <internal key>` (overriding whatever the client
put there), runs `handler.ServeHTTP` against a buffering
`http.ResponseWriter`, and returns an `*http.Response` with the recorded
status, headers and body. A `resolve` error is returned as a transport
error, which the wrapper already classifies as "API unreachable".

The wrapper's client is constructed as `wrapper.NewClient(server.InProcessBaseURL, "")`
with `client.HTTP = &http.Client{Transport: <the transport>, Timeout: 60s}`.
Its default backoff (1s, 2s on 429) stays: the shared write limiter is the
guard against a looping small model, and it applies here on purpose.

Everything between the transport and the service layer is untouched:
`ensureTrustedOrigin` (no Origin header, loopback Host → pass),
`ensureAuthenticated` (§2), `ensureJsonApiScope` (Full passes), the space
grant gate (nil grant → unscoped), the C8 idempotency store, C9 dry-run,
analytics.

## 2. Credential: the per-process internal key

`server.Server` mints one internal key at construction: 32 random bytes,
hex-encoded, held in memory only. `Server.InternalKey()` returns it; only
the API component reads it, to feed the transport.

`ensureAuthenticated` gains one branch before the cache lookup: if the
presented key equals the internal key (constant-time compare), the request
is authenticated as the **internal session**:

| Field | Value | Why |
|---|---|---|
| `Scope` | `AccountAuth_Full` | "full permissions" — passes the `/v2` scope gate |
| `Grant` | nil | unscoped: every space the account can see |
| `ExpireAt` | 0 | never expires; dies with the process |
| `Token` | `internal:` + 16 random bytes hex | non-empty and unique so `RevokeToken` sweeps never match it |
| `AppName` | `InternalAppName` = `"Anytype Assistant"` | stamped as integration name on created objects (§7) |
| `KeyId` | `"internal"` | shows in whoami; never a wallet app link |

The entry is never written to `KeyToToken`, never persisted, never listed
by `ListApps`, and cannot be revoked — it is the process itself. No wallet
app link is created, so the user's linked-apps list does not change.

`emitKeyStatusSignals` treats it as a Full-scope key: the status header is
`legacy` (nil grant) and the legacy-key notice and log line do not fire,
because they address JsonAPI-scope keys only. No change there.

## 3. The API component: engine without a listener

`core/api/service.go` (`apiService`):

- `Run` behaves as today: with a listen address it builds the server and
  starts `ListenAndServe`; with an empty address it logs "disabled" and
  builds nothing. Mobile never passes a listen address, so mobile never
  listens.
- New: `ToolsHost() (*wrapper.Host, error)` on the `api.Service`
  interface. Under the component lock it builds the `server.Server`
  lazily if none exists (same constructor call `startServer` uses, with the
  empty listen address, so `apiBaseUrl` is `http://`, which no tool
  reads), then constructs the wrapper client over the in-process transport
  and hands it to `wrapper.NewHost`, which owns the `Runner` and the
  `MemoryStore`. The host is cached; later calls return the same one.
- Before calling `server.NewServer` the component asks
  `accountService.GetInfo` itself and returns its error, so a bridge call
  during shutdown returns an error instead of hitting `NewServer`'s panic.
- `currentEngine()` resolves the engine and the internal key for the
  transport per request, under the same lock. On desktop, where
  `ReassignAddress` rebuilds the server, the host keeps working against the
  new engine and new key without being rebuilt.
- `Close` needs no host teardown: the `MemoryStore` holds no resources.
  The existing `srv.Stop()` and HTTP shutdown remain as they are.

## 4. The wrapper's in-process delivery

`core/api/wrapper/host.go` — the fourth delivery of the one tool table
(CLI verbs, manifest, MCP stdio, in-process host):

```go
// Host is the long-lived in-process delivery: one Runner over a memory
// session store, called directly by an embedding client.
type Host struct { runner *Runner; store *MemoryStore }

func NewHost(client *Client) *Host

// CallResult is one tool call's outcome as the embedding client receives
// it: Text is what the model reads, JSON the machine shape for the app.
type CallResult struct {
    Text    string `json:"text"`
    JSON    any    `json:"json,omitempty"`
    IsError bool   `json:"is_error"`
    Code    string `json:"code,omitempty"` // "tool_error" on IsError
}

func (h *Host) Call(ctx context.Context, name string, args map[string]any) CallResult
func (h *Host) ResetSession() error   // clears handles and the working space
```

`Call` mirrors `MCPServer.handleToolsCall`: wrapper-side validation
errors, server C6 errors (already translated to tool vocabulary) and
unknown-tool errors all come back **in-band** (`IsError: true`, text is
the repair tip). The tip logic `MCPServer.errorText` is refactored into a
shared function that takes the two delivery-specific closures (the 401 tip
and the unreachable tip); the host's versions say "internal error, report
it" and "the account is not running", since neither condition is something
the model or the user can repair by editing the call.

Tiering is a manifest concern, not a call concern: `Host.Call` accepts any
tool in the table (the runner already refuses unknown names with the full
list). The model can only call what its manifest gave it, and a host that
served the small manifest never puts a large-tier name in front of it.

## 5. The gomobile exports

`clientlibrary/service/tools.go`, exported through the existing `Lib`
framework / AAR as `ServiceToolsManifest`, `ServiceToolsCall`,
`ServiceToolsResetSession`:

```go
// ToolsManifest renders the tier's tool manifest ("small" | "large").
// Pure: works before login, so a client can build its tools early.
func ToolsManifest(tier string) []byte

// ToolsCall runs one tool asynchronously as the current account and
// delivers exactly one envelope to callback, from a Go-owned thread.
func ToolsCall(name string, args []byte, callback MessageHandler)

// ToolsResetSession forgets the handle table (a new conversation).
func ToolsResetSession() []byte
```

Every return and callback payload is one **envelope** JSON object:

```json
{"text": "...", "json": {...}, "is_error": false, "code": "..."}
```

| `code` | When | `text` |
|---|---|---|
| (absent) | success | the model-facing text; `json` carries the machine shape (for the manifest: the manifest itself) |
| `tool_error` | in-band tool failure | the repair tip |
| `account_not_running` | no account app | "no account is running" |
| `bad_request` | `args` is not a JSON object, unknown tier | what was wrong |
| `internal` | panic inside the call (recovered) | "internal error" |

`ToolsCall` resolves the host through `mw.GetApp()` → the `api` component
→ `ToolsHost()`; a nil app is `account_not_running`. It parses `args`
(empty or `null` → `{}`), spawns a goroutine, recovers panics with the
package `PanicHandler` (as `CommandAsync` does) and calls
`callback.Handle(envelope)` once. The runner serializes tool execution, so
concurrent `ToolsCall`s are safe and simply queue.

The mobile library init (`clientlibrary/service/lib.go`) sets the API
package's middleware handle (`api.SetMiddlewareParams(mw)`), which today
only the desktop gRPC binary does; without it the API component's
`mw` is nil on mobile.

## 6. Concurrency and lifecycle

- `ToolsCall` never blocks the caller. The goroutine calls back on a Go
  thread; the Swift side resumes a continuation (the event handler already
  crosses this way).
- `Runner.Run` holds one mutex per host: tool calls are serialized, which
  is also what the handle table's read-modify-write needs.
- Session state lives for the process. `ToolsResetSession` is the
  conversation boundary. Account switch: the account app is torn down and
  rebuilt, and with it the API component and its host — a fresh key, a
  fresh session.
- The engine is built once per account start, on first use. Cost: gin
  route table, v1 and v2 service structs (lazy caches). No goroutine.

## 7. Security notes

- The internal key is 256 bits from `crypto/rand`, never serialized,
  never logged, readable only through `Server.InternalKey()` inside the
  process. On desktop, where a listener also exists, presenting it over TCP
  would authenticate too — and cannot happen without already having code
  execution in the process.
- The bridge acts as the account holder with Full scope by design (the
  user's decision). The shared write rate limit stays on as the guard
  against a model looping on writes.
- Objects created through the bridge are stamped with the integration name
  `Anytype Assistant`, so provenance can tell assistant-made objects from
  user-made ones. The wrapper's tool table has no object DELETE tool, so
  the provenance-based delete rule is not reachable from the bridge. The
  name is one constant; leaving objects unstamped is a one-line change.

## 8. Testing

- `core/api/server`: the in-process transport against the v2 test fixture
  (the one that registers `/v2`): the internal key answers
  `GET /v2/auth/whoami` with Full scope and no grant; a foreign key still
  gets 401 and the mint path is exercised; the internal entry never appears
  in `KeyToToken`; `RevokeToken` of any token leaves it working; a request
  built from `InProcessBaseURL` passes the origin policy; a `resolve`
  error surfaces as a transport error.
- `core/api/wrapper`: `Host.Call` over the existing stub-server fixture —
  success returns text and JSON; a wrapper-side validation error is
  in-band with the exact tip; an unknown tool lists the tools; a server C6
  error arrives translated; `ResetSession` makes a handle reference fail
  with the no-session tip.
- `clientlibrary/service`: manifest envelopes for both tiers decode and
  their tool names equal `wrapper.ToolNamesForTier`; an unknown tier is
  `bad_request`; `ToolsCall` with no account delivers
  `account_not_running`; malformed args deliver `bad_request`; a round
  trip through a host over a tiny `httptest` server delivers the tool's
  text; a panicking host delivers `internal` and calls `PanicHandler`.
- `core/api`: `ToolsHost` on a component built from mocks returns a host
  without an HTTP server, returns the same host twice, and its host
  answers an unknown-tool call in-band (wiring proof without data).
- Existing suites stay green; `TestWrapperRoutesRegistered` is unaffected.

## 9. Contract for the Swift side (not in this repo)

- Call `ServiceToolsManifest("small")` once (before or after login), decode
  the envelope's `json` as the wrapper manifest (`version`, `tools[]` with
  `name`, `description`, `parameters`, `example`, `gbnf`, and
  `filterGrammar`). Build one Foundation Models `Tool` per entry; convert
  `parameters` (flat, strict, non-recursive JSON Schema) into a
  `DynamicGenerationSchema`; feed `description` and `example` into the tool
  description.
- Each tool's `call` wraps `ServiceToolsCall(name, argsJSON, handler)` in a
  continuation and returns the envelope's `text` to the model; keep `json`
  for rendering (handles → tappable objects).
- Call `ServiceToolsResetSession()` when a new conversation starts.
- The app may prefill `space` arguments with the current space before
  dispatching, so the model never has to emit a space id.

## 10. Documentation

- `core/api/APIV2.md` §8: a new "The in-process delivery (mobile bridge)
  — decisions as built" section recording §1–§7 above in the file's own
  style, and the wrapper package doc updated from three deliveries to
  four.
- `docs/proto.md` is unaffected (no proto change).
