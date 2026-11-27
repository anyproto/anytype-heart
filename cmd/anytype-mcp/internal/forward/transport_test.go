package forward

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
)

func TestEnvelope(t *testing.T) {
	t.Run("case variants of reserved members are refused before anything runs", func(t *testing.T) {
		h := start(t)
		h.initialize()
		before := len(h.heart.requests())
		for _, line := range []string{
			`{"jsonrpc":"2.0","ID":1,"method":"tools/call","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"Method":"tools/call","params":{}}`,
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","Params":{}}`,
			`{"JSONRPC":"2.0","jsonrpc":"2.0","id":4,"method":"ping"}`,
			`{"jsonrpc":"2.0","id":5,"method":"ping","Result":1}`,
			`{"jsonrpc":"2.0","id":6,"method":"ping","Kresult":1,"reſult":1}`,
			`{"jsonrpc":"2.0","id":7,"method":"ping","method":"tools/call"}`,
		} {
			g := h.call(line)
			require.NotNil(t, g.Error, line)
			assert.Equal(t, codeInvalidRequest, g.Error.Code, line)
		}
		// the initialize-less frame with ID has no usable id
		assert.Equal(t, before, len(h.heart.requests()), "nothing may be forwarded")
	})

	t.Run("an oversized line is skipped and the next line is served", func(t *testing.T) {
		h := start(t)
		big := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("a", contract.MaxMessageBytes) + `"}}`
		go h.send(big)
		g := h.recv()
		require.NotNil(t, g.Error)
		assert.Equal(t, codeInvalidRequest, g.Error.Code)
		assert.Equal(t, "null", string(g.ID))
		g = h.call(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
		assert.Nil(t, g.Error)
		assert.Equal(t, "2", string(g.ID))
	})

	t.Run("a large integer id comes back byte for byte", func(t *testing.T) {
		h := start(t)
		h.initialize()
		g := h.call(`{"jsonrpc":"2.0","id":123456789012345678901234567890,"method":"ping"}`)
		assert.Nil(t, g.Error)
		assert.Equal(t, "123456789012345678901234567890", string(g.ID))
	})

	t.Run("a batch gets one refusal with a null id", func(t *testing.T) {
		h := start(t)
		g := h.call(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"ping"}]`)
		require.NotNil(t, g.Error)
		assert.Equal(t, codeInvalidRequest, g.Error.Code)
		assert.Equal(t, "null", string(g.ID))
		h.silent()
	})

	t.Run("not JSON, wrong shapes", func(t *testing.T) {
		h := start(t)
		assert.Equal(t, codeParse, h.call(`{nope`).Error.Code)
		assert.Equal(t, codeInvalidRequest, h.call(`"text"`).Error.Code)
		assert.Equal(t, codeInvalidRequest, h.call(`{"jsonrpc":"1.0","id":1,"method":"ping"}`).Error.Code)
		assert.Equal(t, codeInvalidRequest, h.call(`{"jsonrpc":"2.0","id":null,"method":"ping"}`).Error.Code)
		assert.Equal(t, codeInvalidRequest, h.call(`{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`).Error.Code)
		assert.Equal(t, codeInvalidRequest, h.call(`{"jsonrpc":"2.0","id":1}`).Error.Code)
	})
}

func TestGateAndLifecycle(t *testing.T) {
	t.Run("before initialize only initialize and ping run", func(t *testing.T) {
		h := start(t)
		g := h.call(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		require.NotNil(t, g.Error)
		assert.Equal(t, codeNotInitialized, g.Error.Code)
		g = h.call(toolCall(2))
		assert.Equal(t, codeNotInitialized, g.Error.Code)
		assert.Nil(t, h.call(`{"jsonrpc":"2.0","id":3,"method":"ping"}`).Error)
		assert.Empty(t, h.heart.requests())
		h.initialize()
		assert.Nil(t, h.call(`{"jsonrpc":"2.0","id":4,"method":"tools/list"}`).Error)
	})

	t.Run("the gate covers the full tier", func(t *testing.T) {
		h := start(t, withTier("full"))
		assert.Equal(t, codeNotInitialized, h.call(toolCall(1)).Error.Code)
		h.initialize()
		g := h.call(toolCall(2))
		assert.Nil(t, g.Error)
		assert.Equal(t, "done", g.text())
		for _, r := range h.heart.requests() {
			assert.Equal(t, "/mcp/full", r.Path)
			assert.Empty(t, r.Session)
		}
	})

	t.Run("notifications and client responses are forwarded and never answered", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.send(`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`)
		h.send(`{"jsonrpc":"2.0","id":9,"result":{"roots":["marker-roots"]}}`)
		require.Eventually(t, func() bool { return h.heart.count("notifications/roots/list_changed") == 1 }, 3*time.Second, 5*time.Millisecond)
		require.Eventually(t, func() bool {
			for _, r := range h.heart.requests() {
				if strings.Contains(r.Body, "marker-roots") {
					return r.Session == "session-1"
				}
			}
			return false
		}, 3*time.Second, 5*time.Millisecond, "the client response reaches heart too")
		h.silent()
	})

	t.Run("versions: the client sees its own, the header carries heart's", func(t *testing.T) {
		for _, tc := range []struct{ asked, seen string }{
			{"2024-11-05", "2024-11-05"},
			{"2025-06-18", "2025-06-18"},
			{"2025-11-25", "2025-11-25"},
			{"2025-03-26", "2024-11-05"},
			{"2099-01-01", "2025-11-25"},
			{"2020-01-01", "2025-11-25"},
		} {
			h := start(t)
			g := h.call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + tc.asked + `"}}`)
			require.Nil(t, g.Error, g.raw)
			var r struct {
				ProtocolVersion string         `json:"protocolVersion"`
				ServerInfo      map[string]any `json:"serverInfo"`
			}
			require.NoError(t, json.Unmarshal(g.Result, &r))
			assert.Equal(t, tc.seen, r.ProtocolVersion, tc.asked)
			assert.Equal(t, "anytype", r.ServerInfo["name"])
			require.Nil(t, h.call(`{"jsonrpc":"2.0","id":2,"method":"ping"}`).Error)
			reqs := h.heart.requests()
			last := reqs[len(reqs)-1]
			assert.Contains(t, contract.HeartVersions, last.Version)
			assert.Equal(t, "session-1", last.Session)
			assert.Empty(t, reqs[0].Version, "initialize carries no version header")
		}
	})
}

func TestStatusTable(t *testing.T) {
	answer := func(status int, ctype, body string) func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
		return func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" && rec.RPC != "tools/list" {
				return false
			}
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return true
		}
	}
	restBody := func(status int, msg string) string {
		return fmt.Sprintf(`{"object":"error","status":%d,"code":"x","message":%q}`, status, msg)
	}
	cases := []struct {
		name       string
		status     int
		ctype      string
		body       string
		toolIsErr  bool
		toolCode   int // when the tool call gets a JSON-RPC error instead
		otherCode  int
		wantInText string
	}{
		{"202 for a request", 202, "", "", true, 0, codeInternal, "did not confirm"},
		{"200 with another id", 200, "application/json", `{"jsonrpc":"2.0","id":"someone-else","result":{}}`, true, 0, codeInternal, "did not confirm"},
		{"200 unreadable body", 200, "application/json", `{"jsonrpc":`, true, 0, codeInternal, "did not confirm"},
		{"200 event stream", 200, "text/event-stream", "data: {}\n\n", true, 0, codeInternal, "did not confirm"},
		{"400 JSON-RPC error with a null id", 400, "application/json", `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"heart says no","data":{"secret":1}}}`, false, -32600, -32600, "heart says no"},
		{"400 REST", 400, "application/json", restBody(400, "Mcp-Session-Id is required"), false, codeInvalidRequest, codeInvalidRequest, "Mcp-Session-Id is required"},
		{"408 REST", 408, "application/json", restBody(408, "the message did not arrive"), false, codeInvalidRequest, codeInvalidRequest, "did not arrive"},
		{"413 REST", 413, "application/json", restBody(413, "too big"), false, codeInvalidRequest, codeInvalidRequest, "too big"},
		{"403", 403, "application/json", restBody(403, "scope"), true, 0, codeNotConnected, "does not allow"},
		{"429", 429, "application/json", restBody(429, "too many"), true, 0, codeInternal, "busy"},
		{"500 with the key in the message", 500, "application/json", restBody(500, "boom "+canaryKey), true, 0, codeInternal, "boom [redacted]"},
		{"302", 302, "", "", true, 0, codeInternal, "did not confirm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := start(t)
			h.initialize()
			h.heart.setHook(answer(tc.status, tc.ctype, tc.body))

			g := h.call(toolCall("tool-1"))
			assert.Equal(t, `"tool-1"`, string(g.ID))
			if tc.toolIsErr {
				require.Nil(t, g.Error, g.raw)
				assert.True(t, g.isError(), g.raw)
				assert.Contains(t, g.text(), tc.wantInText)
			} else {
				require.NotNil(t, g.Error, g.raw)
				assert.Equal(t, tc.toolCode, g.Error.Code)
				assert.Contains(t, g.Error.Message, tc.wantInText)
				assert.Empty(t, g.Error.Data, "upstream data is dropped")
			}
			g = h.call(`{"jsonrpc":"2.0","id":77,"method":"tools/list"}`)
			assert.Equal(t, "77", string(g.ID))
			require.NotNil(t, g.Error, g.raw)
			assert.Equal(t, tc.otherCode, g.Error.Code)

			assert.Equal(t, 1, h.heart.count("tools/call"), "never replayed")
			assert.Equal(t, 1, h.heart.count("tools/list"), "never replayed")
			h.silent()
			assert.NotContains(t, h.all.String(), canaryKey)
		})
	}

	t.Run("a relayed JSON-RPC error keeps heart's error object", func(t *testing.T) {
		h := start(t)
		h.initialize()
		g := h.call(`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`)
		require.NotNil(t, g.Error)
		assert.Equal(t, -32601, g.Error.Code)
	})

	t.Run("404 with no session: this Anytype is too old", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			http.NotFound(w, r)
			return true
		})
		g := h.call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		require.NotNil(t, g.Error)
		assert.Equal(t, codeMethodNotFound, g.Error.Code)
	})

	t.Run("lost after sending: outcome unconfirmed, never replayed", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" && rec.RPC != "tools/list" {
				return false
			}
			// the write was received; the receipt is dropped
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return true
		})
		g := h.call(toolCall(1))
		require.Nil(t, g.Error, g.raw)
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "check before retrying")
		assert.Equal(t, 1, h.heart.count("tools/call"))
		g = h.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		assert.Equal(t, codeInternal, g.Error.Code)
	})

	t.Run("a truncated body and a missed deadline are unconfirmed", func(t *testing.T) {
		h := start(t, withOptions(func(o *Options) { o.ExchangeTimeout = 300 * time.Millisecond }))
		h.initialize()
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			switch rec.RPC {
			case "tools/call":
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Length", "500")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0",`))
				w.(http.Flusher).Flush()
				<-release
				return true
			case "tools/list":
				<-release
				return true
			}
			return false
		})
		g := h.call(toolCall(1))
		assert.True(t, g.isError(), g.raw)
		assert.Contains(t, g.text(), "check before retrying")
		assert.Equal(t, codeInternal, h.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`).Error.Code)
	})

	t.Run("a 401 whose body stalls still fails the request quickly", func(t *testing.T) {
		h := start(t, withOptions(func(o *Options) { o.ErrorBodyWait = 100 * time.Millisecond }))
		h.initialize()
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusUnauthorized)
			w.(http.Flusher).Flush()
			<-release
			return true
		})
		began := time.Now()
		g := h.call(toolCall(1))
		require.NotNil(t, g.Error, g.raw)
		assert.Equal(t, codeNotConnected, g.Error.Code)
		assert.Less(t, time.Since(began), 3*time.Second)
		assert.Equal(t, 1, h.heart.count("tools/call"))
	})
}

func TestSessions(t *testing.T) {
	t.Run("expiry found by ping: one renewal, then every tools/call until the warning is written", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.expireSessions()

		g := h.call(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		require.Nil(t, g.Error, g.raw)
		assert.Equal(t, 2, h.heart.count("initialize"), "the saved initialize was replayed once")
		assert.Equal(t, 2, h.heart.count("notifications/initialized"))
		renew := h.heart.requests()[len(h.heart.requests())-3]
		assert.Equal(t, "initialize", renew.RPC)
		assert.Contains(t, renew.Body, `"clientInfo"`, "renewal uses the saved params")
		assert.Empty(t, renew.Session)

		g = h.call(toolCall(2))
		require.Nil(t, g.Error, g.raw)
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "session expired")
		assert.Equal(t, 0, h.heart.count("tools/call"), "the call did not run")

		h.warned()
		g = h.call(toolCall(3))
		assert.False(t, g.isError(), g.raw)
		assert.Equal(t, 1, h.heart.count("tools/call"))
	})

	t.Run("a tools/call that hits the 404 gets the warning and is not replayed", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.expireSessions()
		g := h.call(toolCall(1))
		assert.True(t, g.isError(), g.raw)
		assert.Contains(t, g.text(), "session expired")
		assert.Equal(t, 1, h.heart.count("tools/call"), "only the refused attempt")
		h.warned()
		assert.False(t, h.call(toolCall(2)).isError())
	})

	t.Run("concurrent 404s open one session", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.expireSessions()
		for i := 0; i < 4; i++ {
			h.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/list"}`, i))
		}
		for i := 0; i < 4; i++ {
			assert.Nil(t, h.recv().Error)
		}
		assert.Equal(t, 2, h.heart.count("initialize"))
		h.heart.mu.Lock()
		opened := h.heart.opened
		h.heart.mu.Unlock()
		assert.Equal(t, 2, opened)
	})

	t.Run("a replaced initialize ends the session it replaced", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.initialize()
		require.Eventually(t, func() bool {
			for _, r := range h.heart.requests() {
				if r.Method == http.MethodDelete && r.Session == "session-1" {
					return true
				}
			}
			return false
		}, 3*time.Second, 5*time.Millisecond)
	})

	t.Run("a changed key on a curated tier opens a new session with the warning", func(t *testing.T) {
		h := start(t)
		h.initialize()
		const next = "CANARY-second-key-abcdefghijklmnopqrstuvwxyz"
		h.heart.mu.Lock()
		h.heart.key = next
		// heart binds a session to the key that opened it
		h.heart.sessions = map[string]bool{}
		h.heart.mu.Unlock()
		it := h.item()
		it.Key, it.Rev = next, it.Rev+1
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)

		g := h.call(toolCall(1))
		assert.True(t, g.isError(), g.raw)
		assert.Contains(t, g.text(), "session expired")
		h.warned()
		assert.False(t, h.call(toolCall(2)).isError())
	})
}

func TestBoundsAndCancellation(t *testing.T) {
	// block holds every tools/call upstream until released
	block := func(h *harness) (release func(), inFlight *atomic.Int32) {
		ch := make(chan struct{})
		var once sync.Once
		var n atomic.Int32
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" {
				return false
			}
			n.Add(1)
			select {
			case <-ch:
			case <-r.Context().Done():
			}
			return false
		})
		release = func() { once.Do(func() { close(ch) }) }
		t.Cleanup(release)
		return release, &n
	}

	t.Run("4 run, 32 wait, the next is busy at once", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		release, inFlight := block(h)
		total := contract.Workers + contract.QueueSize
		for i := 0; i < contract.Workers; i++ {
			h.send(toolCall(i))
		}
		require.Eventually(t, func() bool { return inFlight.Load() == contract.Workers }, 3*time.Second, 5*time.Millisecond)
		for i := contract.Workers; i < total; i++ {
			h.send(toolCall(i))
		}
		g := h.call(toolCall("one-too-many"))
		require.NotNil(t, g.Error, g.raw)
		assert.Equal(t, codeBusy, g.Error.Code)
		assert.Equal(t, `"one-too-many"`, string(g.ID))
		assert.EqualValues(t, contract.Workers, inFlight.Load(), "never more than 4 in flight")

		release()
		seen := map[string]bool{}
		for i := 0; i < total; i++ {
			g := h.recv()
			assert.False(t, g.isError(), g.raw)
			assert.False(t, seen[string(g.ID)], "one reply per request")
			seen[string(g.ID)] = true
		}
		assert.Len(t, seen, total)
		h.silent()
	})

	t.Run("a cancel that arrives at a full queue still works", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		release, inFlight := block(h)
		total := contract.Workers + contract.QueueSize
		for i := 0; i < contract.Workers; i++ {
			h.send(toolCall(i))
		}
		require.Eventually(t, func() bool { return inFlight.Load() == contract.Workers }, 3*time.Second, 5*time.Millisecond)
		for i := contract.Workers; i < total; i++ {
			h.send(toolCall(i))
		}
		// a queued one first: the queue stays full, and the reader still serves the cancel
		h.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":%d}}`, total-1))
		assert.Equal(t, codeBusy, h.call(toolCall("probe")).Error.Code)
		// then a running one
		h.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":0,"reason":"user"}}`)
		release()
		seen := map[string]bool{}
		for i := 0; i < total-2; i++ {
			seen[string(h.recv().ID)] = true
		}
		h.silent()
		assert.False(t, seen["0"], "a cancelled running request gets no reply")
		assert.False(t, seen[fmt.Sprint(total-1)], "a cancelled queued request gets no reply")
		assert.Equal(t, total-1, h.heart.count("tools/call"), "a cancelled queued request never starts")
		assert.Equal(t, 0, h.heart.count("notifications/cancelled"), "cancellation is not forwarded")
	})

	t.Run("a duplicate id in flight is refused", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		release, inFlight := block(h)
		h.send(toolCall(1))
		require.Eventually(t, func() bool { return inFlight.Load() == 1 }, 3*time.Second, 5*time.Millisecond)
		assert.Equal(t, codeInvalidRequest, h.call(toolCall(1)).Error.Code)
		release()
		assert.False(t, h.recv().isError())
	})
}

func TestShutdown(t *testing.T) {
	deletes := func(h *harness) int {
		n := 0
		for _, r := range h.heart.requests() {
			if r.Method == http.MethodDelete {
				n++
			}
		}
		return n
	}

	t.Run("stdin EOF: accepted requests finish, the session is deleted, exit 0", func(t *testing.T) {
		h := start(t)
		h.initialize()
		slow := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "tools/call" {
				<-slow
			}
			return false
		})
		h.send(toolCall(1))
		require.Eventually(t, func() bool { return h.heart.count("tools/call") == 1 }, 3*time.Second, 5*time.Millisecond)
		require.NoError(t, h.in.Close())
		time.AfterFunc(100*time.Millisecond, func() { close(slow) })
		g := h.recv()
		assert.False(t, g.isError(), g.raw)
		assert.Equal(t, 0, h.waitExit())
		assert.Equal(t, 1, deletes(h))
	})

	t.Run("a signal cancels everything and deletes the session", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "tools/call" {
				<-r.Context().Done()
				return true
			}
			return false
		})
		h.send(toolCall(1))
		require.Eventually(t, func() bool { return h.heart.count("tools/call") == 1 }, 3*time.Second, 5*time.Millisecond)
		h.cancel()
		assert.Equal(t, 0, h.waitExit())
		assert.Equal(t, 1, deletes(h))
	})

	t.Run("stdout stalled, then closed, with stdin open: exit within the deadline, one DELETE", func(t *testing.T) {
		gate := &gateWriter{closed: make(chan struct{})}
		h := start(t, withOptions(func(o *Options) {
			o.StdoutStall = 200 * time.Millisecond
			gate.w = o.Out
			o.Out = gate
		}))
		h.initialize()
		// from here on stdout accepts nothing
		gate.stalled.Store(true)
		h.send(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		h.send(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
		began := time.Now()
		code := h.waitExit()
		assert.Equal(t, 1, code)
		assert.Less(t, time.Since(began), 3*time.Second)
		assert.Equal(t, 1, deletes(h))
		close(gate.closed)
	})

	t.Run("a closed stdout is a broken stdout", func(t *testing.T) {
		h := start(t)
		h.initialize()
		require.NoError(t, h.outR.Close())
		h.send(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		assert.Equal(t, 1, h.waitExit())
		assert.Equal(t, 1, deletes(h))
	})
}

// gateWriter passes writes through until it is stalled; a stalled write
// blocks until the gate is opened again (the write then goes through) or
// closed (it fails).
type gateWriter struct {
	w       io.Writer
	stalled atomic.Bool
	closed  chan struct{}
	reopen  chan struct{}
	once    sync.Once
}

func (g *gateWriter) Write(p []byte) (int, error) {
	g.once.Do(func() {
		if g.reopen == nil {
			g.reopen = make(chan struct{}, 1)
		}
	})
	if g.stalled.Load() {
		select {
		case <-g.closed:
			return 0, io.ErrClosedPipe
		case <-g.reopen:
		}
	}
	return g.w.Write(p)
}

func TestNoSecretsOnStdoutOrStderr(t *testing.T) {
	h := start(t)
	h.initialize()
	h.call(toolCall(1))
	h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
		if rec.RPC == "tools/call" {
			rest(w, 500, "echo Bearer "+canaryKey)
			return true
		}
		return false
	})
	h.call(toolCall(2))
	h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
		if rec.RPC == "tools/call" {
			rest(w, 401, "bad "+canaryKey)
			return true
		}
		return false
	})
	h.call(toolCall(3))
	require.NoError(t, h.in.Close())
	h.waitExit()
	assert.NotContains(t, h.all.String(), canaryKey)
	assert.NotContains(t, h.log.String(), canaryKey)
	assert.NotContains(t, h.log.String(), "Bearer")
	assert.NotEmpty(t, h.log.String())
	for _, r := range h.heart.requests() {
		assert.Equal(t, "Bearer "+canaryKey, r.Auth, "the bearer goes to heart")
	}
}
