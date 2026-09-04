# Mobile Tool Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the API v2 task-tool wrapper (`core/api/wrapper`) to the gomobile clients through three exports, with no HTTP listener on the device and the calls authorized as the current account.

**Architecture:** The wrapper stays an HTTP client of `/v2`; its transport becomes an in-memory call into the gin engine (`server.NewInProcessTransport`). The server mints a per-process internal key that `ensureAuthenticated` resolves to a Full-scope, unscoped, in-memory session. The API component builds the engine lazily on first tool use and hands the wrapper a `Host`; the mobile package exports `ToolsManifest`, `ToolsCall` (async, callback) and `ToolsResetSession`, each returning one JSON envelope.

**Tech Stack:** Go, gin, gomobile (`clientlibrary/service`), testify + mockery mocks (`core/api/core/mock_apicore`), `objectstore.NewStoreFixture`.

**Spec:** `docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md`

## Global Constraints

- Branch: `go-7383-apiv2-clean` (worktree `/Users/roma/anytype/anytype-heart_apiv2`). Commit messages start with `GO-7383 ` and end with the trailer line `Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1`.
- No HTTP listener on mobile, ever: nothing in this plan calls `ListenAndServe` outside the existing `startServer`, which stays gated on a non-empty listen address.
- Errors are wrapped with `fmt.Errorf("operation: %w", err)`; never a bare `return err`.
- Naming follows the repo: `Api`, `Url`, `Id` (not `API`, `URL`, `ID`).
- Tests use testify (`require`/`assert`), `// given` `// when` `// then` comments, and the existing fixtures (`newV2ServerFixture` in `core/api/server`, `newFixture` in `core/api/wrapper`).
- `deps/libs` in this worktree is already a symlink to the main checkout's tantivy libs; if a test fails to link with `library 'tantivy_go' not found`, run `ln -sfn /Users/roma/anytype/anytype-heart/deps/libs deps/libs`.
- The internal key is never logged, serialized or listed; the internal session is never written to `KeyToToken`.
- The integration name constant is `"Anytype Assistant"` (spec §2, §7).

---

### Task 1: Internal key and internal session in the server

**Files:**
- Modify: `core/api/server/server.go` (struct `Server`, `NewServer`)
- Modify: `core/api/server/middleware.go:78-152` (`ensureAuthenticated`, the key lookup and mint)
- Test: `core/api/server/internalkey_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `const InternalAppName = "Anytype Assistant"`, `func (srv *Server) InternalKey() string`, and the auth behavior "a request bearing the internal key is the internal session (Scope Full, Grant nil, ExpireAt 0)". Task 2 and Task 4 rely on `InternalKey()`.

- [ ] **Step 1: Write the failing test**

Create `core/api/server/internalkey_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
)

// whoamiWith performs GET /v2/auth/whoami with the given bearer through
// the engine and returns the recorder.
func whoamiWith(fx *fixture, bearer string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v2/auth/whoami", nil)
	req.Host = localApiHost
	req.Header.Set("Authorization", "Bearer "+bearer)
	fx.Engine().ServeHTTP(w, req)
	return w
}

func TestInternalKey(t *testing.T) {
	t.Run("authenticates as a Full-scope unscoped session on /v2", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := whoamiWith(fx, fx.InternalKey())

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.WhoamiResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "full", got.Scope)
		assert.False(t, got.Grant.Scoped)
		assert.Equal(t, InternalAppName, got.Key.Name)
		assert.Equal(t, internalKeyId, got.Key.Id)
		assert.Nil(t, got.Key.ExpiresAt, "never expires")
		// the entry is the process itself, never a cached key
		assert.Empty(t, fx.KeyToToken)
	})

	t.Run("survives every RevokeToken sweep", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.RevokeToken(fx.internalSession.Token)
		fx.RevokeToken("some-other-token")

		// when
		w := whoamiWith(fx, fx.InternalKey())

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("a foreign key still goes through the mint path", func(t *testing.T) {
		// given: the wallet rejects the key
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.mwMock.On("WalletCreateSession", mock.Anything, mock.Anything).Return(&pb.RpcWalletCreateSessionResponse{
			Error: &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_BAD_INPUT},
		}).Once()

		// when
		w := whoamiWith(fx, "not-the-internal-key")

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("two servers mint different keys", func(t *testing.T) {
		// given
		a := newV2ServerFixture(t)
		b := newV2ServerFixture(t)

		// then
		assert.Len(t, a.InternalKey(), 64, "32 random bytes, hex")
		assert.NotEqual(t, a.InternalKey(), b.InternalKey())
		assert.NotEqual(t, a.internalSession.Token, b.internalSession.Token)
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/api/server/ -run TestInternalKey -v`
Expected: compile error — `fx.InternalKey undefined`, `InternalAppName undefined`.

- [ ] **Step 3: Mint the key in the server**

In `core/api/server/server.go`, add imports `"crypto/rand"`, `"crypto/subtle"`, `"encoding/hex"`, `"fmt"` and, below the `ApiSessionEntry` type, the constants:

```go
// InternalAppName is the app name of the in-process session (the mobile
// tool bridge spec, §2): it rides as the integration name on objects the
// bridge creates, so provenance can tell assistant-made objects from
// user-made ones. One constant; leaving them unstamped is a one-line change.
const InternalAppName = "Anytype Assistant"

// internalKeyId is the KeyId the internal session reports (whoami's
// key.id). It is never a wallet app-link hash.
const internalKeyId = "internal"
```

Add two fields to `Server` (after `evictGen`):

```go
	// internalKey authenticates the in-process delivery (core/api/wrapper's
	// Host over the in-process transport) as the account holder with Full
	// scope. Minted once per Server from crypto/rand, held in memory only,
	// never persisted, logged or listed; readable through InternalKey() by
	// the API component that wires the transport. internalSession is the
	// fixed entry it resolves to — never written to KeyToToken, so no
	// eviction or RevokeToken sweep can touch it.
	internalKey     string
	internalSession ApiSessionEntry
```

In `NewServer`, after `s.legacyKeyLogSeen = make(map[string]time.Time)`:

```go
	key, token, err := mintInternalCredential()
	if err != nil {
		panic(err)
	}
	s.internalKey = key
	s.internalSession = ApiSessionEntry{
		Token:     token,
		AppName:   InternalAppName,
		Scope:     model.AccountAuth_Full,
		KeyId:     internalKeyId,
		CreatedAt: time.Now().Unix(),
	}
```

Add at the end of the file:

```go
// mintInternalCredential returns the internal bearer key (32 random bytes,
// hex) and the synthetic session token behind it ("internal:" plus 16
// random bytes, hex — non-empty and unique, so a RevokeToken sweep for a
// real token never matches it).
func mintInternalCredential() (key string, token string, err error) {
	var buf [48]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", "", fmt.Errorf("mint internal api credential: %w", err)
	}
	return hex.EncodeToString(buf[:32]), "internal:" + hex.EncodeToString(buf[32:]), nil
}

// InternalKey returns the per-process bearer key that authenticates the
// in-process delivery as the account holder (Full scope, no grant, no
// expiry). Only the API component reads it, to feed the in-process
// transport; it must never be serialized or logged.
func (srv *Server) InternalKey() string { return srv.internalKey }

// isInternalKey reports whether key is the internal key, in constant time.
func (srv *Server) isInternalKey(key string) bool {
	return srv.internalKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(srv.internalKey)) == 1
}
```

- [ ] **Step 4: Resolve the session in the middleware**

In `core/api/server/middleware.go`, inside `ensureAuthenticated`, replace the block that starts at the comment `// Validate the key - if the key exists in the KeyToToken map` and ends with the cache write (`srv.mu.Lock() / if srv.evictGen == mintGen {...} / srv.mu.Unlock()` and its closing `}` of `if !exists`) with:

```go
		apiSession, ok := srv.resolveApiSession(c, mw, key)
		if !ok {
			return
		}
```

Keep everything after it (the expiry check, the context carriers, `emitKeyStatusSignals`, `c.Next()`) unchanged. Then add, after `ensureAuthenticated`:

```go
// resolveApiSession turns a presented key into its session entry: the
// internal key (the in-process delivery — the mobile tool bridge spec §2)
// resolves to the fixed internal session, never minted and never cached;
// every other key goes through the cache and, on a miss, the wallet mint.
// On a rejected key it writes the 401 and reports false.
func (srv *Server) resolveApiSession(c *gin.Context, mw apicore.ClientCommands, key string) (ApiSessionEntry, bool) {
	if srv.isInternalKey(key) {
		return srv.internalSession, true
	}

	// Validate the key - if the key exists in the KeyToToken map, it is considered valid.
	// Otherwise, attempt to create a new session using the key and add it to the map upon successful validation.
	// The eviction generation is snapshotted in the SAME critical section
	// as the cache read: the cache write below is conditional on it.
	srv.mu.Lock()
	apiSession, exists := srv.KeyToToken[key]
	mintGen := srv.evictGen
	srv.mu.Unlock()
	if exists {
		return apiSession, true
	}

	response := mw.WalletCreateSession(context.Background(), &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key}})
	if response.Error.Code != pb.RpcWalletCreateSessionResponseError_NULL {
		// An expired key gets a distinct 401 so the client knows to
		// re-issue it instead of retrying the same key (H5: ExpireAt
		// must actually be enforced).
		message := ErrInvalidApiKey.Error()
		if response.Error.Code == pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED {
			message = ErrApiKeyExpired.Error()
		}
		c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
		apiErr := util.CodeToApiError(http.StatusUnauthorized, message)
		c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
		return ApiSessionEntry{}, false
	}
	apiSession = ApiSessionEntry{
		Token:     response.Token,
		AppName:   response.AppName,
		Scope:     response.AccountScope,
		ExpireAt:  response.AppExpireAt,
		Grant:     util.ApiGrantFromProto(response.Grant),
		KeyId:     response.AppHash,
		CreatedAt: response.AppCreatedAt,
	}

	// Cache only if no eviction swept while the mint was in flight. A
	// RevokeToken in that window (LinkLocalUpdateApp persists the new
	// grant FIRST, then sweeps) found no entry for this key, so the
	// entry just minted may carry the pre-edit grant. Serving THIS
	// request from it is equivalent to the request having completed
	// before the edit; CACHING it would make the stale grant permanent
	// — so on a generation mismatch the entry is dropped and the next
	// request re-mints against what the wallet holds then.
	srv.mu.Lock()
	if srv.evictGen == mintGen {
		srv.KeyToToken[key] = apiSession
	}
	srv.mu.Unlock()
	return apiSession, true
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./core/api/server/ -run 'TestInternalKey|TestV2Routes|TestNewServer' -v`
Expected: PASS for all.

- [ ] **Step 6: Run the whole server package**

Run: `go test ./core/api/server/`
Expected: `ok`.

- [ ] **Step 7: Commit**

```bash
git add core/api/server/server.go core/api/server/middleware.go core/api/server/internalkey_test.go
git commit -m "GO-7383 API v2: the server mints an in-memory internal key

The in-process delivery of the task tools (the mobile bridge) needs to
authenticate as the account holder without an app link in the wallet. The
server now mints one random key per process and resolves it to a fixed
Full-scope, unscoped session that is never cached, listed or revocable.

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

---

### Task 2: The in-process transport

**Files:**
- Create: `core/api/server/inproc.go`
- Test: `core/api/server/inproc_test.go`

**Interfaces:**
- Consumes: `(*Server).Engine() *gin.Engine`, `(*Server).InternalKey() string` (Task 1).
- Produces: `const InProcessBaseURL = "http://127.0.0.1"`, `type InProcessResolver func() (http.Handler, string, error)`, `func NewInProcessTransport(resolve InProcessResolver) http.RoundTripper`. Task 4 relies on all three.

- [ ] **Step 1: Write the failing test**

Create `core/api/server/inproc_test.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// inProcessClient builds an http.Client over the fixture's engine, the way
// the API component does for the tools host.
func inProcessClient(fx *fixture) *http.Client {
	return &http.Client{Transport: NewInProcessTransport(func() (http.Handler, string, error) {
		return fx.Engine(), fx.InternalKey(), nil
	})}
}

func TestInProcessTransport(t *testing.T) {
	t.Run("whoami round trip through the engine with no listener", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/auth/whoami", nil)
		require.NoError(t, err)

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		var got v2model.WhoamiResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, "full", got.Scope)
		assert.Equal(t, InternalAppName, got.Key.Name)
	})

	t.Run("the transport's bearer wins over the client's", func(t *testing.T) {
		// given: a client that sends its own (foreign) key
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/auth/whoami", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer foreign-key")

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then: no mint was attempted (the mw mock has no expectation and
		// would fail the test), and the answer is the internal session
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("request bodies reach the handler", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("POST", InProcessBaseURL+"/v2/validate", strings.NewReader(`{"formatVersion":"2.0","blocks":[]}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body struct {
			Issues []any `json:"issues"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Empty(t, body.Issues)
	})

	t.Run("an unknown route is a 404 through the same path", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/no-such-route", nil)
		require.NoError(t, err)

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a resolve error is a transport error", func(t *testing.T) {
		// given: no engine (no account running)
		client := &http.Client{Transport: NewInProcessTransport(func() (http.Handler, string, error) {
			return nil, "", errors.New("api engine is not built")
		})}
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/spaces", nil)
		require.NoError(t, err)

		// when
		resp, err := client.Do(req)

		// then: http.Client wraps it in *url.Error, which is what the
		// wrapper classifies as "unreachable"
		require.Nil(t, resp)
		var ue *url.Error
		require.ErrorAs(t, err, &ue)
		assert.Contains(t, err.Error(), "api engine is not built")
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/api/server/ -run TestInProcessTransport -v`
Expected: compile error — `NewInProcessTransport undefined`, `InProcessBaseURL undefined`.

- [ ] **Step 3: Write the transport**

Create `core/api/server/inproc.go`:

```go
package server

// inproc.go — the in-process transport of the mobile tool bridge (spec §1,
// docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md): an
// http.RoundTripper that serves each request by calling the gin engine's
// ServeHTTP directly. No listener, no socket, no port — the "HTTP" is Go
// structs passed between functions in one process, and every middleware
// between the transport and the service layer runs unchanged. Responses
// are buffered whole: the chat SSE stream is not served this way (nothing
// in the wrapper's tool table calls it).

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// InProcessBaseURL is the base URL the in-process transport answers for.
// The host is an IP literal so the origin policy's Host check
// (localorigin.AllowHost) admits it; no port, because nothing listens.
const InProcessBaseURL = "http://127.0.0.1"

// inProcessRemoteAddr is the RemoteAddr stamped on every in-process
// request: the write rate limiter keys on it, and loopback is the honest
// answer.
const inProcessRemoteAddr = "127.0.0.1:0"

// InProcessResolver returns the engine to serve a request with and the
// bearer key to authenticate it. It is called per request, so a rebuilt
// engine (ReassignAddress on desktop) and its fresh internal key are
// picked up without rebuilding the transport. An error means the API is
// not available (no account running); http.Client wraps it in *url.Error,
// which the wrapper reports as "unreachable".
type InProcessResolver func() (handler http.Handler, bearer string, err error)

// NewInProcessTransport builds the transport over resolve.
func NewInProcessTransport(resolve InProcessResolver) http.RoundTripper {
	return &inProcessTransport{resolve: resolve}
}

type inProcessTransport struct {
	resolve InProcessResolver
}

// RoundTrip serves req in-process. The request is cloned before being
// handed to the engine (a RoundTripper must not modify its argument), and
// the authorization is set here rather than by the client, so a rotated
// internal key never goes stale in a long-lived client.
func (t *inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	handler, bearer, err := t.resolve()
	if err != nil {
		return nil, fmt.Errorf("in-process api: %w", err)
	}
	served := req.Clone(req.Context())
	served.RemoteAddr = inProcessRemoteAddr
	served.Header.Set("Authorization", "Bearer "+bearer)
	if served.Body == nil {
		served.Body = http.NoBody
	}
	rec := &bufferedResponse{header: http.Header{}}
	handler.ServeHTTP(rec, served)
	body := rec.body.Bytes()
	status := rec.status()
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// bufferedResponse is the http.ResponseWriter the engine writes into:
// headers, one status, the whole body. Flush is a no-op — a buffered
// writer cannot stream, and this transport does not claim to.
type bufferedResponse struct {
	header      http.Header
	code        int
	wroteHeader bool
	body        bytes.Buffer
}

func (r *bufferedResponse) Header() http.Header { return r.header }

func (r *bufferedResponse) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.code = code
	r.wroteHeader = true
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

// Flush satisfies http.Flusher for handlers that probe for it; there is
// nothing to flush to.
func (r *bufferedResponse) Flush() {}

func (r *bufferedResponse) status() int {
	if !r.wroteHeader {
		return http.StatusOK
	}
	return r.code
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./core/api/server/ -run TestInProcessTransport -v`
Expected: PASS for all five subtests.

- [ ] **Step 5: Commit**

```bash
git add core/api/server/inproc.go core/api/server/inproc_test.go
git commit -m "GO-7383 API v2: an in-process transport into the engine

A RoundTripper that serves a request by calling the gin engine's ServeHTTP
with a buffered writer. It is how the mobile tool bridge reaches /v2
without a listener: no socket, no port, every middleware still runs.

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

---

### Task 3: The wrapper's in-process delivery (`Host`)

**Files:**
- Modify: `core/api/wrapper/mcp.go` (`errorText`, lines ~302-320: extract the shared tip function)
- Create: `core/api/wrapper/host.go`
- Test: `core/api/wrapper/host_test.go`

**Interfaces:**
- Consumes: `NewRunner`, `NewMemoryStore`, `(*Runner).Run`, `ToolError`, `isTransportError` (existing).
- Produces: `type Host`, `func NewHost(client *Client) *Host`, `type CallResult struct{Text string; JSON any; IsError bool; Code string}` (JSON tags `text`, `json`, `is_error`, `code`), `const CallCodeToolError = "tool_error"`, `func (h *Host) Call(ctx context.Context, name string, args map[string]any) CallResult`, `func (h *Host) ResetSession() error`. Tasks 4 and 5 rely on these.

- [ ] **Step 1: Write the failing test**

Create `core/api/wrapper/host_test.go`:

```go
package wrapper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHostFixture builds a Host over the stub-server fixture's runner, so
// the host tests reuse the fixture's stubs and recorded requests.
func newHostFixture(t *testing.T) (*fixture, *Host) {
	fx := newFixture(t)
	return fx, &Host{runner: fx.Runner, store: fx.store}
}

func TestHostCall(t *testing.T) {
	t.Run("success carries the text and the machine shape", func(t *testing.T) {
		// given
		fx, host := newHostFixture(t)
		fx.stub("GET /v2/spaces", 200, `{"data":[{"id":"space1","name":"Work"}],"total":1,"has_more":false}`)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.False(t, got.IsError, got.Text)
		assert.Empty(t, got.Code)
		assert.Contains(t, got.Text, "Work — space1")
		require.IsType(t, spacesResult{}, got.JSON)
		assert.Equal(t, 1, got.JSON.(spacesResult).Total)
	})

	t.Run("a wrapper-side validation error is in-band", func(t *testing.T) {
		// given: find without its required space
		fx, host := newHostFixture(t)

		// when
		got := host.Call(context.Background(), "find", map[string]any{"query": "x"})

		// then
		assert.True(t, got.IsError)
		assert.Equal(t, CallCodeToolError, got.Code)
		assert.Contains(t, got.Text, `find needs "space"`)
		assert.Empty(t, fx.requests, "nothing reached the server")
	})

	t.Run("an unknown tool lists the tools", func(t *testing.T) {
		// given
		_, host := newHostFixture(t)

		// when
		got := host.Call(context.Background(), "nope", map[string]any{})

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, `unknown tool "nope"`)
		assert.Contains(t, got.Text, "spaces, find")
	})

	t.Run("a server refusal arrives as its C6 text", func(t *testing.T) {
		// given
		fx, host := newHostFixture(t)
		fx.stub("GET /v2/spaces", 500, `{"status":500,"code":"internal","message":"index unavailable","issues":[]}`)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.True(t, got.IsError)
		assert.Equal(t, "index unavailable", got.Text)
	})

	t.Run("an unreachable API says the account is not running", func(t *testing.T) {
		// given: a client pointed at a port nothing listens on
		client := NewClient("http://127.0.0.1:1", "")
		client.Backoff = func(int) time.Duration { return 0 }
		host := NewHost(client)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "account is not running")
		assert.Contains(t, got.Text, "no change to the call will help")
	})

	t.Run("ResetSession forgets the handles and the working space", func(t *testing.T) {
		// given: a session with a handle
		fx, host := newHostFixture(t)
		require.NoError(t, fx.store.Save(&Session{Space: "space1", Handles: []Handle{{N: 1, Id: "obj1", Name: "Note"}}}))

		// when
		require.NoError(t, host.ResetSession())

		// then
		session, err := fx.store.Load()
		require.NoError(t, err)
		assert.Empty(t, session.Space)
		assert.Empty(t, session.Handles)
		got := host.Call(context.Background(), "read", map[string]any{"object": "1"})
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "run find first")
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/api/wrapper/ -run TestHostCall -v`
Expected: compile error — `Host undefined`, `NewHost undefined`, `CallCodeToolError undefined`.

- [ ] **Step 3: Extract the shared repair tip from the MCP delivery**

In `core/api/wrapper/mcp.go`, replace the `errorText` method (keep its doc comment's intent) with:

```go
// deliveryTips are the two repair tips only a delivery can know — the
// conditions whose fix is outside the model's reach. Both must say "ask
// the user", or a small model burns its retries re-sending variants of a
// call that can never succeed.
type deliveryTips struct {
	// unauthorized renders a server 401.
	unauthorized func(te *ToolError) string
	// unreachable renders a transport failure (the request never got an
	// HTTP response).
	unreachable func(err error) string
}

// repairTip renders an error as the tip the model reads. Wrapper and
// server errors already carry their own steering (validateArgs, C6 hints,
// the ops→tool translation); this layer adds only the delivery's two.
func repairTip(err error, tips deliveryTips) string {
	var te *ToolError
	if errors.As(err, &te) {
		if te.Status == 401 {
			return tips.unauthorized(te)
		}
		return te.Text
	}
	if isTransportError(err) {
		return tips.unreachable(err)
	}
	return err.Error()
}

// errorText renders an error as the repair tip the MCP host shows the
// model: the shared tip chain plus the two conditions a process boundary
// knows — the API server is unreachable, the key was rejected.
func (s *MCPServer) errorText(err error) string {
	return repairTip(err, deliveryTips{
		unauthorized: func(te *ToolError) string {
			return te.Text + "\nfix: the API key was rejected — ask the user to check ANYTYPE_API_KEY (Anytype app → Settings → API keys); no change to the call will help"
		},
		unreachable: func(err error) string {
			return fmt.Sprintf("cannot reach the local Anytype API at %s — ask the user to start the Anytype app; no change to the call will help (%v)", s.runner.client.BaseURL, err)
		},
	})
}
```

Run: `go test ./core/api/wrapper/ -run TestMCP -v`
Expected: PASS (the MCP tips are byte-identical to before).

- [ ] **Step 4: Write the host**

Create `core/api/wrapper/host.go`:

```go
package wrapper

// host.go — the in-process delivery of the tool table (the mobile tool
// bridge spec, docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md
// §4): a long-lived Runner over a memory session store, called directly by
// an embedding client (the gomobile exports in clientlibrary/service).
// Fourth delivery of the ONE definition — CLI verbs, manifest, MCP stdio,
// and this — sharing the manifest, the runner and the in-band error
// contract with the MCP server.

import (
	"context"
	"fmt"
)

// CallCodeToolError marks an in-band tool failure on a CallResult: the
// text is the repair tip the model reads.
const CallCodeToolError = "tool_error"

// Host is the in-process delivery: one Runner, one session, for the life
// of the embedding process.
type Host struct {
	runner *Runner
	store  *MemoryStore
}

// NewHost builds a host over a client. The session store is in memory:
// handles live until ResetSession or the process ends.
func NewHost(client *Client) *Host {
	store := NewMemoryStore()
	return &Host{runner: NewRunner(client, store), store: store}
}

// CallResult is one tool call's outcome as the embedding client receives
// it: Text is what the model reads (the MCP content text), JSON the
// machine shape for the app (the CLI --json shape). On IsError, Text is
// the repair tip and Code is CallCodeToolError.
type CallResult struct {
	Text    string `json:"text"`
	JSON    any    `json:"json,omitempty"`
	IsError bool   `json:"is_error"`
	Code    string `json:"code,omitempty"`
}

// Call runs one tool. Every failure — wrapper-side validation, a server
// C6 refusal (already in tool vocabulary), an unknown tool name — comes
// back IN-BAND, the §8.20 repair-loop contract: the model reads the tip
// and repairs the call.
func (h *Host) Call(ctx context.Context, name string, args map[string]any) CallResult {
	if args == nil {
		args = map[string]any{}
	}
	result, err := h.runner.Run(ctx, name, args)
	if err != nil {
		return CallResult{Text: h.errorText(err), IsError: true, Code: CallCodeToolError}
	}
	return CallResult{Text: result.Text, JSON: result.JSON}
}

// ResetSession forgets the handle table and the working space — the
// conversation boundary for an embedding client.
func (h *Host) ResetSession() error {
	if err := h.store.Save(&Session{}); err != nil {
		return fmt.Errorf("reset session: %w", err)
	}
	return nil
}

// errorText renders the host's two delivery tips. Neither condition can be
// repaired by editing the call: a 401 cannot happen behind the in-process
// transport (the key is the process's own) and is a bug; an unreachable
// API means no account is running.
func (h *Host) errorText(err error) string {
	return repairTip(err, deliveryTips{
		unauthorized: func(te *ToolError) string {
			return te.Text + "\nfix: the in-process credential was rejected — this is a bug in the Anytype app, report it; no change to the call will help"
		},
		unreachable: func(err error) string {
			return fmt.Sprintf("the Anytype account is not running — ask the user to sign in; no change to the call will help (%v)", err)
		},
	})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./core/api/wrapper/ -run 'TestHostCall|TestMCP' -v`
Expected: PASS.

- [ ] **Step 6: Run the whole wrapper package**

Run: `go test ./core/api/wrapper/`
Expected: `ok`.

- [ ] **Step 7: Commit**

```bash
git add core/api/wrapper/host.go core/api/wrapper/host_test.go core/api/wrapper/mcp.go
git commit -m "GO-7383 API v2: the wrapper's in-process delivery

A Host runs the task tools for an embedding client: one Runner over a
memory session store, every failure in-band as a repair tip, the same
contract the MCP delivery has. The two delivery-specific tips are now a
parameter of one shared tip function.

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

---

### Task 4: The API component builds the engine without a listener

**Files:**
- Modify: `core/api/service.go` (interface `Service`, struct `apiService`, `startServer`, `ReassignAddress`, new methods)
- Modify: `core/application/sessions_test.go:333-343` (the `mockApiService` stub gains `ToolsHost`)
- Test: `core/api/toolshost_test.go`

**Interfaces:**
- Consumes: `server.InProcessBaseURL`, `server.NewInProcessTransport`, `(*server.Server).InternalKey()` (Tasks 1–2); `wrapper.NewClient`, `wrapper.NewHost`, `(*wrapper.Host).Call` (Task 3).
- Produces: `ToolsHost() (*wrapper.Host, error)` on the `api.Service` interface. Task 5 relies on it.

- [ ] **Step 1: Write the failing test**

Create `core/api/toolshost_test.go`:

```go
package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newToolsHostFixture builds the component the way Init would, from mocks,
// with no listen address — the mobile configuration.
func newToolsHostFixture(t *testing.T, accountMock *mock_apicore.MockAccountService) *apiService {
	crossSpaceSub := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	crossSpaceSub.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	eventMock := mock_apicore.NewMockEventService(t)
	eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	return &apiService{
		mw:                   mock_apicore.NewMockClientCommands(t),
		accountService:       accountMock,
		eventService:         eventMock,
		crossSpaceSubService: crossSpaceSub,
		chatSubService:       mock_apicore.NewMockChatSubscriptionService(t),
		fileObjectService:    mock_apicore.NewMockFileObjectService(t),
		objectReader:         mock_apicore.NewMockObjectReader(t),
		objectCreator:        mock_apicore.NewMockObjectCreator(t),
		objectMutator:        mock_apicore.NewMockObjectMutator(t),
		objectProvenance:     mock_apicore.NewMockObjectProvenance(t),
		objectStore:          objectstore.NewStoreFixture(t),
	}
}

func runningAccount(t *testing.T) *mock_apicore.MockAccountService {
	accountMock := mock_apicore.NewMockAccountService(t)
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{TechSpaceId: "tech1"}, nil)
	return accountMock
}

func TestToolsHost(t *testing.T) {
	t.Run("builds the engine without a listener and reuses it", func(t *testing.T) {
		// given
		fx := newToolsHostFixture(t, runningAccount(t))

		// when
		host, err := fx.ToolsHost()

		// then
		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Nil(t, fx.httpSrv, "nothing listens")
		require.NotNil(t, fx.srv, "the engine exists")
		again, err := fx.ToolsHost()
		require.NoError(t, err)
		assert.Same(t, host, again)
	})

	t.Run("the host reaches the engine as the account holder", func(t *testing.T) {
		// given: spaces is the bootstrap tool; an empty store lists none —
		// the call still crosses auth, the scope gate and the v2 service
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.False(t, got.IsError, got.Text)
		assert.Contains(t, got.Text, "no spaces")
	})

	t.Run("an unknown tool is answered in-band", func(t *testing.T) {
		// given
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)

		// when
		got := host.Call(context.Background(), "nope", nil)

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, `unknown tool "nope"`)
	})

	t.Run("no account info is an error, not a panic", func(t *testing.T) {
		// given
		accountMock := mock_apicore.NewMockAccountService(t)
		accountMock.On("GetInfo", mock.Anything).Return(nil, errors.New("account is stopping"))
		fx := newToolsHostFixture(t, accountMock)

		// when
		host, err := fx.ToolsHost()

		// then
		require.Error(t, err)
		assert.Nil(t, host)
		assert.Nil(t, fx.srv)
	})

	t.Run("ReassignAddress rebuilds the engine the host resolves", func(t *testing.T) {
		// given: a host built on the mobile configuration
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)
		first := fx.srv

		// when: the address changes to "still nothing" — the old server is
		// dropped and no new one is built until the next tool call
		require.NoError(t, fx.ReassignAddress(context.Background(), ""))

		// then
		assert.Nil(t, fx.srv)
		got := host.Call(context.Background(), "spaces", nil)
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "account is not running")
		again, err := fx.ToolsHost()
		require.NoError(t, err)
		assert.Same(t, host, again, "the host is kept; only the engine was rebuilt")
		assert.NotSame(t, first, fx.srv)
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/api/ -run TestToolsHost -v`
Expected: compile error — `fx.ToolsHost undefined`.

- [ ] **Step 3: Add ToolsHost to the component**

In `core/api/service.go`:

Add the import `"github.com/anyproto/anytype-heart/core/api/wrapper"`.

Extend the interface:

```go
type Service interface {
	app.ComponentRunnable
	ReassignAddress(ctx context.Context, listenAddr string) error
	RevokeToken(token string)
	// ToolsHost returns the in-process delivery of the API v2 task tools
	// (core/api/wrapper.Host), building the engine on first use — without
	// a listener. The mobile tool bridge (clientlibrary/service tools.go)
	// is its consumer.
	ToolsHost() (*wrapper.Host, error)
}
```

Add a field to `apiService` (after `httpSrv`):

```go
	// toolsHost is the in-process delivery, built lazily by ToolsHost and
	// kept for the component's life; its transport resolves the engine per
	// request (currentEngine), so a rebuilt server never strands it.
	toolsHost *wrapper.Host
```

Add a constant next to the others:

```go
	// inProcessCallTimeout bounds one in-process tool call end to end —
	// the same 60s the wrapper's client uses against a listener.
	inProcessCallTimeout = 60 * time.Second
```

Replace `startServer` with:

```go
func (s *apiService) startServer() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.listenAddr == "" {
		log.Info("API server disabled (no listen address)")
		return nil
	}

	if err := s.ensureServerLocked(); err != nil {
		return err
	}

	s.httpSrv = &http.Server{
		Addr:              s.listenAddr,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: readTimeout,
	}

	log.Infof("Starting API server on %s", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("API server error: %v", err)
		}
	}()

	return nil
}

// ensureServerLocked builds the server (engine, v1 and v2 services) if
// none exists. It never listens: listening is startServer's job and is
// gated on the listen address. Must be called with s.lock held.
func (s *apiService) ensureServerLocked() error {
	if s.srv != nil {
		return nil
	}
	// NewServer panics when the account info is unavailable (the tech
	// space id comes from it); asking first turns a bridge call during
	// shutdown into an error instead.
	if _, err := s.accountService.GetInfo(context.Background()); err != nil {
		return fmt.Errorf("account info: %w", err)
	}
	s.srv = server.NewServer(
		s.mw,
		s.accountService,
		s.eventService,
		s.crossSpaceSubService,
		s.chatSubService,
		s.fileObjectService,
		server.V2Deps{Reader: s.objectReader, Creator: s.objectCreator, Mutator: s.objectMutator, Provenance: s.objectProvenance, ChatSub: s.chatSubService, Store: s.objectStore, AccountId: s.accountId()},
		s.listenAddr,
		server.OpenApiDocs{
			V1YAML: openapiV1YAML,
			V1JSON: openapiV1JSON,
			V2YAML: openapiV2YAML,
			V2JSON: openapiV2JSON,
		},
	)
	return nil
}

// ToolsHost implements Service: the in-process delivery over an engine
// that was built for it if the listener never built one (mobile). The
// engine is ensured on EVERY call — ReassignAddress drops it, and the
// cached host must find a new one on the next tool call.
func (s *apiService) ToolsHost() (*wrapper.Host, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if err := s.ensureServerLocked(); err != nil {
		return nil, fmt.Errorf("build api engine for tools host: %w", err)
	}
	if s.toolsHost != nil {
		return s.toolsHost, nil
	}
	client := wrapper.NewClient(server.InProcessBaseURL, "")
	client.HTTP = &http.Client{
		Transport: server.NewInProcessTransport(s.currentEngine),
		Timeout:   inProcessCallTimeout,
	}
	s.toolsHost = wrapper.NewHost(client)
	return s.toolsHost, nil
}

// currentEngine resolves the engine and the internal key for the
// in-process transport, per request, so a rebuilt server is picked up.
func (s *apiService) currentEngine() (http.Handler, string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.srv == nil {
		return nil, "", errors.New("api engine is not built")
	}
	return s.srv.Engine(), s.srv.InternalKey(), nil
}
```

Replace `ReassignAddress` with:

```go
func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) error {
	if err := s.shutdownHTTP(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	// The base URL v1 serves in file links derives from the listen
	// address, so the server is rebuilt for the new one (as before this
	// method built a fresh one). The tools host, if any, follows through
	// currentEngine on its next call.
	s.lock.Lock()
	if s.srv != nil {
		s.srv.Stop()
		s.srv = nil
	}
	s.listenAddr = listenAddr
	s.lock.Unlock()

	return s.startServer()
}
```

- [ ] **Step 4: Extend the application test stub**

In `core/application/sessions_test.go`, add after the `RevokeToken` method of `mockApiService`:

```go
func (m *mockApiService) ToolsHost() (*wrapper.Host, error)                { return nil, nil }
```

and add the import `"github.com/anyproto/anytype-heart/core/api/wrapper"` to that file.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./core/api/ -run TestToolsHost -v && go test ./core/application/ -run TestLinkLocal`
Expected: PASS for all subtests; the application tests still compile and pass.

- [ ] **Step 6: Build everything that imports the interface**

Run: `go build ./... && go vet ./core/api/ ./core/application/`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add core/api/service.go core/api/toolshost_test.go core/application/sessions_test.go
git commit -m "GO-7383 API v2: the API component serves the tools host

ToolsHost builds the engine on first use without a listener and hands the
wrapper a client over the in-process transport. ReassignAddress drops the
old server explicitly, which is what it implicitly did before; the host
resolves the engine per call and is never rebuilt.

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

---

### Task 5: The gomobile exports and the mobile wiring

**Files:**
- Create: `clientlibrary/service/tools.go`
- Modify: `clientlibrary/service/lib.go:29-45` (`init`: set the API package's middleware handle)
- Test: `clientlibrary/service/tools_test.go`

**Interfaces:**
- Consumes: `api.CName`, `api.Service.ToolsHost()` (Task 4); `wrapper.ParseTier`, `wrapper.BuildManifestForTier`, `wrapper.Manifest`, `wrapper.NewClient`, `wrapper.NewHost`, `(*wrapper.Host).Call`, `(*wrapper.Host).ResetSession` (Task 3); the existing `MessageHandler` interface and `PanicHandler` var in this package.
- Produces: `func ToolsManifest(tier string) []byte`, `func ToolsCall(name string, args []byte, callback MessageHandler)`, `func ToolsResetSession() []byte`, the envelope codes `ToolsCodeAccountNotRunning`, `ToolsCodeBadRequest`, `ToolsCodeInternal`. gomobile exports them as `ServiceToolsManifest`, `ServiceToolsCall`, `ServiceToolsResetSession`.

- [ ] **Step 1: Write the failing test**

Create `clientlibrary/service/tools_test.go`:

```go
package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// withToolsHost stands in a host provider for one test.
func withToolsHost(t *testing.T, provider func() (*wrapper.Host, error)) {
	prev := toolsHostProvider
	toolsHostProvider = provider
	t.Cleanup(func() { toolsHostProvider = prev })
}

// stubHost builds a host over a tiny server that answers GET /v2/spaces.
func stubHost(t *testing.T) *wrapper.Host {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/spaces" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"space1","name":"Work"}],"total":1,"has_more":false}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"status":404,"code":"not_found","message":"no such route","issues":[]}`)
	}))
	t.Cleanup(srv.Close)
	client := wrapper.NewClient(srv.URL, "k")
	client.Backoff = func(int) time.Duration { return 0 }
	return wrapper.NewHost(client)
}

// recordingHandler receives the one envelope ToolsCall delivers.
type recordingHandler struct{ ch chan []byte }

func newRecordingHandler() *recordingHandler { return &recordingHandler{ch: make(chan []byte, 1)} }

func (r *recordingHandler) Handle(b []byte) { r.ch <- b }

func (r *recordingHandler) await(t *testing.T) toolsEnvelope {
	t.Helper()
	select {
	case b := <-r.ch:
		var env toolsEnvelope
		require.NoError(t, json.Unmarshal(b, &env))
		return env
	case <-time.After(5 * time.Second):
		t.Fatal("no envelope delivered")
		return toolsEnvelope{}
	}
}

func decodeEnvelope(t *testing.T, b []byte) toolsEnvelope {
	t.Helper()
	var env toolsEnvelope
	require.NoError(t, json.Unmarshal(b, &env))
	return env
}

func TestToolsManifest(t *testing.T) {
	for _, tier := range []wrapper.Tier{wrapper.TierSmall, wrapper.TierLarge} {
		t.Run(string(tier), func(t *testing.T) {
			// when
			var env struct {
				JSON    wrapper.Manifest `json:"json"`
				IsError bool             `json:"is_error"`
			}
			require.NoError(t, json.Unmarshal(ToolsManifest(string(tier)), &env))

			// then
			assert.False(t, env.IsError)
			var names []string
			for _, tool := range env.JSON.Tools {
				names = append(names, tool.Name)
			}
			assert.Equal(t, wrapper.ToolNamesForTier(tier), names)
		})
	}

	t.Run("unknown tier is bad_request", func(t *testing.T) {
		env := decodeEnvelope(t, ToolsManifest("huge"))
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeBadRequest, env.Code)
		assert.Contains(t, env.Text, `unknown tier "huge"`)
	})
}

func TestToolsCall(t *testing.T) {
	t.Run("no account delivers account_not_running", func(t *testing.T) {
		// given: the default provider and no account app
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeAccountNotRunning, env.Code)
	})

	t.Run("malformed args are bad_request before the host is consulted", func(t *testing.T) {
		// given
		withToolsHost(t, func() (*wrapper.Host, error) {
			t.Fatal("the host must not be consulted")
			return nil, nil
		})
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", []byte(`[1,2]`), h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeBadRequest, env.Code)
		assert.Contains(t, env.Text, "must be a JSON object")
	})

	t.Run("round trip through a host", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", []byte(`{}`), h)

		// then
		env := h.await(t)
		assert.False(t, env.IsError, env.Text)
		assert.Contains(t, env.Text, "Work — space1")
		assert.NotNil(t, env.JSON)
	})

	t.Run("empty args mean no args", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		assert.False(t, h.await(t).IsError)
	})

	t.Run("a tool error is in-band", func(t *testing.T) {
		// given: find without its space
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })
		h := newRecordingHandler()

		// when
		ToolsCall("find", []byte(`{"query":"x"}`), h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, wrapper.CallCodeToolError, env.Code)
		assert.Contains(t, env.Text, `find needs "space"`)
	})

	t.Run("a panic delivers internal and reaches PanicHandler", func(t *testing.T) {
		// given
		withToolsHost(t, func() (*wrapper.Host, error) { panic("boom") })
		var recovered any
		prev := PanicHandler
		PanicHandler = func(v any) { recovered = v }
		t.Cleanup(func() { PanicHandler = prev })
		h := newRecordingHandler()

		// when
		ToolsCall("spaces", nil, h)

		// then
		env := h.await(t)
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeInternal, env.Code)
		assert.Equal(t, "boom", recovered)
	})
}

func TestToolsResetSession(t *testing.T) {
	t.Run("no account delivers account_not_running", func(t *testing.T) {
		env := decodeEnvelope(t, ToolsResetSession())
		assert.True(t, env.IsError)
		assert.Equal(t, ToolsCodeAccountNotRunning, env.Code)
	})

	t.Run("resets through the host", func(t *testing.T) {
		// given
		host := stubHost(t)
		withToolsHost(t, func() (*wrapper.Host, error) { return host, nil })

		// when
		env := decodeEnvelope(t, ToolsResetSession())

		// then
		assert.False(t, env.IsError)
		assert.Equal(t, "session reset", env.Text)
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./clientlibrary/service/ -run 'TestTools' -v`
Expected: compile error — `ToolsManifest undefined`, `toolsHostProvider undefined`, `toolsEnvelope undefined`.

- [ ] **Step 3: Write the exports**

Create `clientlibrary/service/tools.go`:

```go
package service

// tools.go — the mobile tool bridge (docs/superpowers/specs/
// 2026-09-04-mobile-tool-bridge-design.md §5): the API v2 task tools
// (core/api/wrapper) for an on-device model, exported through gomobile as
// ServiceToolsManifest / ServiceToolsCall / ServiceToolsResetSession.
// Nothing listens: the tools run in-process through the API component's
// engine, authorized as the current account (api.Service.ToolsHost).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// Envelope codes. Success carries no code; an in-band tool failure carries
// wrapper.CallCodeToolError.
const (
	// ToolsCodeAccountNotRunning: no account app is running.
	ToolsCodeAccountNotRunning = "account_not_running"
	// ToolsCodeBadRequest: the arguments were not a JSON object, or the
	// tier is unknown.
	ToolsCodeBadRequest = "bad_request"
	// ToolsCodeInternal: a fault inside the bridge (a recovered panic, an
	// engine that could not be built).
	ToolsCodeInternal = "internal"
)

// toolsEnvelope is the one JSON shape every export returns or delivers.
// It mirrors wrapper.CallResult field for field; the type exists so the
// bridge can mint its own codes without reaching into the wrapper.
type toolsEnvelope struct {
	Text    string `json:"text"`
	JSON    any    `json:"json,omitempty"`
	IsError bool   `json:"is_error"`
	Code    string `json:"code,omitempty"`
}

var errAccountNotRunning = errors.New("no account is running")

// toolsHostProvider resolves the in-process delivery; a var so tests can
// stand in a host without an account.
var toolsHostProvider = defaultToolsHost

// defaultToolsHost resolves the host through the running account app's API
// component.
func defaultToolsHost() (*wrapper.Host, error) {
	a := mw.GetApp()
	if a == nil {
		return nil, errAccountNotRunning
	}
	svc, ok := a.Component(api.CName).(api.Service)
	if !ok {
		return nil, errAccountNotRunning
	}
	host, err := svc.ToolsHost()
	if err != nil {
		return nil, fmt.Errorf("tools host: %w", err)
	}
	return host, nil
}

// ToolsManifest renders the tier's ("small" | "large") tool manifest in
// the envelope's json field. Pure: it works before login, so a client can
// build its tools early.
func ToolsManifest(tier string) []byte {
	parsed, err := wrapper.ParseTier(tier)
	if err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeBadRequest})
	}
	manifest, err := wrapper.BuildManifestForTier(parsed)
	if err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal})
	}
	return encodeToolsEnvelope(toolsEnvelope{Text: fmt.Sprintf("%d tools", len(manifest.Tools)), JSON: manifest})
}

// ToolsCall runs one tool asynchronously as the current account and
// delivers exactly one envelope to callback, from a Go-owned thread. The
// caller's thread is never blocked.
func ToolsCall(name string, args []byte, callback MessageHandler) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if PanicHandler != nil {
					PanicHandler(r)
				}
				callback.Handle(encodeToolsEnvelope(toolsEnvelope{Text: "internal error in the tool bridge", IsError: true, Code: ToolsCodeInternal}))
			}
		}()
		callback.Handle(encodeToolsEnvelope(toolsCall(name, args)))
	}()
}

// toolsCall is the synchronous body of ToolsCall.
func toolsCall(name string, args []byte) toolsEnvelope {
	parsed, err := parseToolsArgs(args)
	if err != nil {
		return toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeBadRequest}
	}
	host, err := toolsHostProvider()
	if err != nil {
		return hostErrorEnvelope(err)
	}
	result := host.Call(context.Background(), name, parsed)
	return toolsEnvelope{Text: result.Text, JSON: result.JSON, IsError: result.IsError, Code: result.Code}
}

// ToolsResetSession forgets the handle table — a new conversation.
func ToolsResetSession() []byte {
	host, err := toolsHostProvider()
	if err != nil {
		return encodeToolsEnvelope(hostErrorEnvelope(err))
	}
	if err := host.ResetSession(); err != nil {
		return encodeToolsEnvelope(toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal})
	}
	return encodeToolsEnvelope(toolsEnvelope{Text: "session reset"})
}

// parseToolsArgs decodes the arguments: empty or null is no arguments;
// anything but a JSON object is refused with the shape named.
func parseToolsArgs(args []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, fmt.Errorf(`arguments must be a JSON object, e.g. {"space":"..."}: %w`, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// hostErrorEnvelope maps a host resolution failure to its envelope.
func hostErrorEnvelope(err error) toolsEnvelope {
	if errors.Is(err, errAccountNotRunning) {
		return toolsEnvelope{
			Text:    "no account is running — ask the user to sign in; no change to the call will help",
			IsError: true,
			Code:    ToolsCodeAccountNotRunning,
		}
	}
	return toolsEnvelope{Text: err.Error(), IsError: true, Code: ToolsCodeInternal}
}

// encodeToolsEnvelope marshals an envelope. A tool result that cannot be
// encoded is reported instead of dropped — the envelope itself always can.
func encodeToolsEnvelope(e toolsEnvelope) []byte {
	data, err := json.Marshal(e)
	if err != nil {
		data, _ = json.Marshal(toolsEnvelope{Text: fmt.Sprintf("encode result: %v", err), IsError: true, Code: ToolsCodeInternal})
	}
	return data
}
```

- [ ] **Step 4: Wire the API package's middleware handle on mobile**

In `clientlibrary/service/lib.go`, add the import `"github.com/anyproto/anytype-heart/core/api"` and, in `init()`, right after the `registerClientCommandsHandler(...)` call:

```go
	// The API component reads its middleware handle at construction
	// (api.New); the desktop gRPC binary sets it and mobile never did,
	// because mobile never listened. The tool bridge runs the same
	// component in-process, so mobile sets it too.
	api.SetMiddlewareParams(mw)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./clientlibrary/service/ -run 'TestTools' -v`
Expected: PASS for every subtest.

- [ ] **Step 6: Build the mobile package with the mobile tags and vet it**

Run: `go build -tags "gomobile nogrpcserver nowatchdog nosigar" ./clientlibrary/service/ && go vet ./clientlibrary/service/`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add clientlibrary/service/tools.go clientlibrary/service/tools_test.go clientlibrary/service/lib.go
git commit -m "GO-7383 Export the API v2 task tools to mobile clients

ToolsManifest, ToolsCall and ToolsResetSession, through gomobile: a tier's
manifest, one asynchronous tool call answered by a single JSON envelope on
a callback, and the conversation boundary. Mobile now sets the API
component's middleware handle, which only the desktop binary did.

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

---

### Task 6: Record the delivery and verify the whole tree

**Files:**
- Modify: `core/api/APIV2.md` (append §8.57 after §8.56, which ends before the file's final section list if any)
- Modify: `core/api/wrapper/manifest.go:1-5` (package doc: three deliveries → four)

**Interfaces:** none.

- [ ] **Step 1: Update the wrapper package doc**

In `core/api/wrapper/manifest.go`, replace the first five lines of the package comment with:

```go
// Package wrapper is the task-tool layer over the /v2 REST surface: one
// curated tool table, exposed as CLI verbs (cmd/anytype), as a
// machine-readable function-calling manifest, as an MCP stdio server for
// local models (mcp.go, tier-filtered per tier.go/§8.20), and as an
// in-process host for embedding clients (host.go, the mobile bridge) —
// ONE definition, four deliveries.
```

- [ ] **Step 2: Append the decisions-as-built section to APIV2.md**

Append after §8.56 (the last `### 8.` section) in `core/api/APIV2.md`:

```markdown
### 8.57 The in-process delivery: the mobile tool bridge (2026-09-04 — as built)

The tool table reaches the gomobile clients (anytype-swift, anytype-kotlin)
for on-device models. Spec: `docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md`.

**No listener on mobile, ever.** The wrapper stays a client of `/v2`
(§8.6: one enforcement point), but its transport is an in-memory call
into the gin engine — `server.NewInProcessTransport` serves each request
by calling `ServeHTTP` with a buffered writer. No socket, no port; every
middleware runs unchanged (origin, auth, scope gate, space grant, the C8
idempotency store, C9 dry-run, the shared write rate limit, analytics).
Responses are buffered whole; the chat SSE stream is not served this way
and no tool calls it. A direct backend over `v2service.Service` was
measured and declined: 23 wrapper call sites, 38 HTTP-coupled spots, and
roughly 1.5–2k lines of handler logic (space refs, id and key shaping,
idempotency, dry-run, If-Match, error envelopes) to duplicate — the
third surface this document forbids.

**The credential is the process.** `server.Server` mints one internal
key per process (32 random bytes, in memory only, never persisted, logged
or listed). `ensureAuthenticated` resolves it, in constant time, to a
fixed session: Full scope, nil grant, no expiry, app name
`Anytype Assistant`, key id `internal`. It is never written to
`KeyToToken`, so no eviction or `RevokeToken` sweep touches it, and no
wallet app link exists for it. The app name rides as the integration name
on created objects, so provenance can tell assistant-made objects from
user-made ones; the table has no object DELETE tool, so the
provenance-based delete rule is not reachable from here.

**The engine is built lazily.** `api.Service.ToolsHost()` builds the
server (engine, v1 and v2 services) on first tool use when the listener
never did, and hands the wrapper a client over the in-process transport;
the transport resolves the engine and the key per request, so a rebuilt
server (`ReassignAddress`, which now drops the old one explicitly) never
strands the host. Mobile sets the API component's middleware handle in
the library init, which only the desktop binary did.

**The fourth delivery.** `wrapper.Host` is one long-lived Runner over a
`MemoryStore`; `Call` answers every failure IN-BAND (the §8.20 repair
loop), through the same tip function the MCP delivery uses, with the two
delivery-specific tips replaced: a 401 is "a bug in the app, report it",
unreachable is "the account is not running". `ResetSession` is the
conversation boundary.

**The exports.** `ServiceToolsManifest(tier)` (pure, works before login),
`ServiceToolsCall(name, argsJSON, callback)` (returns immediately, runs on
a goroutine, delivers exactly one envelope), `ServiceToolsResetSession()`.
One envelope shape everywhere: `{text, json, is_error, code}`; codes
`tool_error`, `account_not_running`, `bad_request`, `internal`. Tiering is
a manifest concern, not a call concern: the model can only call what its
manifest gave it.

**Not built, stated.** No cancellation of an in-flight call; no streaming;
no tier check on `Call`; no desktop use of the host (desktop clients keep
the listener, the CLI and MCP).
```

- [ ] **Step 3: Run the full verification**

Run:

```bash
go build ./... && \
go vet ./core/api/... ./clientlibrary/service/ ./core/application/ && \
go test ./core/api/... ./clientlibrary/service/ ./core/application/
```

Expected: build and vet silent; every package `ok`.

- [ ] **Step 4: Commit**

```bash
git add core/api/APIV2.md core/api/wrapper/manifest.go
git commit -m "GO-7383 API v2: record the in-process delivery as built

Claude-Session: https://claude.ai/code/session_01JbxuoYSEZHneqLhHRSBSs1"
```

- [ ] **Step 5: Push the branch**

```bash
PATH="/Users/roma/Library/Python/3.9/bin:$PATH" git push origin go-7383-apiv2-clean
```

Expected: the branch advances on origin; no PR is opened (the branch is the long-running API v2 branch).
