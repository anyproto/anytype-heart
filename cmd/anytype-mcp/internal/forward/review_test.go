package forward

import (
	"bytes"
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
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// holdWriter passes writes through, except that a write matching hold
// blocks until release is closed.
type holdWriter struct {
	w       io.Writer
	hold    func(p []byte) bool
	release chan struct{}
	held    atomic.Int32
}

func (g *holdWriter) Write(p []byte) (int, error) {
	if g.hold(p) {
		g.held.Add(1)
		<-g.release
	}
	return g.w.Write(p)
}

func withHeldStdout(g *holdWriter) hopt {
	return withOptions(func(o *Options) {
		g.w = o.Out
		g.release = make(chan struct{})
		o.Out = g
	})
}

func (h *harness) queued() int {
	return len(h.srv.out)
}

func ping(id any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "ping"})
	return string(b)
}

// K2: the reset warning is on stdout before any later tools/call runs.
func TestResetWarningComesFirst(t *testing.T) {
	t.Run("no tools/call reaches heart while the warning line is still being written", func(t *testing.T) {
		g := &holdWriter{hold: func(p []byte) bool { return bytes.Contains(p, []byte("session expired")) }}
		h := start(t, withHeldStdout(g))
		h.initialize()
		h.heart.expireSessions()

		h.send(toolCall(1)) // hits the 404, renews, and its warning blocks in the write
		require.Eventually(t, func() bool { return g.held.Load() == 1 }, 3*time.Second, time.Millisecond)
		require.Equal(t, 1, h.heart.count("tools/call"), "the refused attempt")

		h.send(toolCall(2)) // sent while the client can't have read the warning
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 1, h.heart.count("tools/call"), "nothing runs before the warning is written")

		close(g.release)
		for i := 0; i < 2; i++ {
			got := h.recv()
			assert.True(t, got.isError(), got.raw)
			assert.Contains(t, got.text(), "session expired")
		}
		assert.Equal(t, 1, h.heart.count("tools/call"))
		h.warned()
		assert.False(t, h.call(toolCall(3)).isError())
		assert.Equal(t, 2, h.heart.count("tools/call"))
	})

	t.Run("a call admitted before a renewal never runs on the new session", func(t *testing.T) {
		paused, resume := make(chan struct{}), make(chan struct{})
		h := start(t, withOptions(func(o *Options) {
			o.afterAdmit = func(c *call) {
				if string(c.msg.id) == `"paused"` {
					close(paused)
					<-resume
				}
			}
		}))
		h.initialize()
		h.send(toolCall("paused"))
		<-paused
		h.heart.expireSessions()
		require.Nil(t, h.call(ping(2)).Error, "the ping renews the session")
		require.Equal(t, "session-2", h.srv.session().id)
		close(resume)
		got := h.recv()
		assert.Equal(t, `"paused"`, string(got.ID))
		assert.True(t, got.isError(), got.raw)
		assert.Contains(t, got.text(), "session expired")
		for _, r := range h.heart.requests() {
			if r.RPC == "tools/call" {
				assert.Equal(t, "session-1", r.Session, "only ever sent on the session it was admitted with")
			}
		}
	})
}

// blockedWriter never accepts anything.
type blockedWriter struct{ forever chan struct{} }

func (b blockedWriter) Write([]byte) (int, error) {
	<-b.forever
	return 0, io.ErrClosedPipe
}

// K3: one hard deadline from the trigger; the logger never blocks.
func TestShutdownIsBounded(t *testing.T) {
	t.Run("EOF with a stdout that never accepts and a full reply queue: exit 1 by the deadline", func(t *testing.T) {
		gate := &gateWriter{closed: make(chan struct{})}
		h := start(t, withOptions(func(o *Options) {
			o.StdoutStall = time.Minute // the stall watchdog must not be what ends it
			o.ShutdownDeadline = 500 * time.Millisecond
			gate.w = o.Out
			o.Out = gate
		}))
		t.Cleanup(func() { close(gate.closed) })
		h.initialize()
		gate.stalled.Store(true)
		for i := 0; i < contract.QueueSize+contract.Workers+1; i++ {
			h.send(ping(i))
		}
		require.Eventually(t, func() bool { return h.queued() == contract.QueueSize }, 3*time.Second, time.Millisecond)
		began := time.Now()
		require.NoError(t, h.in.Close())
		assert.Equal(t, 1, h.waitExit(), "replies were lost: not a clean exit")
		assert.Less(t, time.Since(began), 3*time.Second)
	})

	t.Run("stdout breaking during the EOF flush is exit 1", func(t *testing.T) {
		gate := &gateWriter{closed: make(chan struct{})}
		h := start(t, withOptions(func(o *Options) {
			o.StdoutStall = time.Minute
			gate.w = o.Out
			o.Out = gate
		}))
		h.initialize()
		gate.stalled.Store(true)
		h.send(ping(1))
		require.Eventually(t, func() bool { return h.heart.count("ping") == 1 }, 3*time.Second, time.Millisecond)
		require.NoError(t, h.in.Close())
		time.AfterFunc(100*time.Millisecond, func() { close(gate.closed) })
		assert.Equal(t, 1, h.waitExit())
	})

	t.Run("a signal with a stderr nobody drains: returns by the deadline, the DELETE was sent", func(t *testing.T) {
		stuck := blockedWriter{forever: make(chan struct{})}
		t.Cleanup(func() { close(stuck.forever) })
		h := start(t, withOptions(func(o *Options) {
			o.Log = stuck
			o.ShutdownDeadline = time.Second
		}))
		h.initialize()
		// more log lines than the queue holds: none of them may block a worker
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "tools/list" {
				rest(w, 500, "boom")
				return true
			}
			return false
		})
		for i := 0; i < 3*logQueue; i++ {
			require.NotNil(t, h.call(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/list"}`, i)).Error)
		}
		began := time.Now()
		h.cancel()
		assert.Equal(t, 0, h.waitExit())
		assert.Less(t, time.Since(began), 3*time.Second)
		assert.Equal(t, 1, h.deletes("session-1"))
	})

	t.Run("control characters in a client's method never reach stderr", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if strings.HasPrefix(rec.RPC, "evil") {
				rest(w, 500, "boom")
				return true
			}
			return false
		})
		long := strings.Repeat("m", 2000)
		require.NotNil(t, h.call(`{"jsonrpc":"2.0","id":1,"method":"evil\r\u001b[31m\u0000\ttail`+long+`"}`).Error)
		require.NoError(t, h.in.Close())
		h.waitExit()
		log := h.log.String()
		require.Contains(t, log, "evil")
		for _, line := range strings.Split(strings.TrimSuffix(log, "\n"), "\n") {
			assert.LessOrEqual(t, len(line), logLineMax+len("anytype-mcp: "))
			assert.NotContains(t, line, "\r")
			assert.NotContains(t, line, "\x1b")
			assert.NotContains(t, line, "\x00")
			assert.NotContains(t, line, "\t")
		}
	})
}

// K4: a session belongs to the snapshot that opened it.
func TestSessionOwnership(t *testing.T) {
	t.Run("after a port move the shutdown DELETE is not sent to the old port", func(t *testing.T) {
		h := start(t)
		h.initialize()
		moved := newFakeHeart(t)
		it := h.item()
		it.Port, it.Rev = moved.port(), it.Rev+1
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)
		require.NoError(t, h.in.Close())
		assert.Equal(t, 0, h.waitExit())
		assert.Zero(t, h.deletes(""), "the bearer never goes to the port Anytype left")
		for _, r := range moved.requests() {
			assert.NotEqual(t, http.MethodDelete, r.Method)
		}
	})

	t.Run("a rotated key: the old session's DELETE is not sent with the new key", func(t *testing.T) {
		h := start(t)
		h.initialize()
		it := h.item()
		it.Key, it.Rev = "CANARY-rotated-key-abcdefghijklmnopqrstuvwx", it.Rev+1
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)
		require.NoError(t, h.in.Close())
		h.waitExit()
		assert.Zero(t, h.deletes(""))
	})

	t.Run("a client initialize during a renewal keeps its session; the renewal's is ended", func(t *testing.T) {
		h := start(t)
		h.initialize()
		release := make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(release) }) })
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "notifications/initialized" && rec.Session == "session-2" {
				<-release // the renewal's second leg
			}
			return false
		})
		h.heart.expireSessions()
		h.send(ping(1)) // 404, then the renewal opens session-2 and waits
		require.Eventually(t, func() bool {
			for _, r := range h.heart.requests() {
				if r.RPC == "notifications/initialized" && r.Session == "session-2" {
					return true
				}
			}
			return false
		}, 3*time.Second, time.Millisecond)

		h.send(`{"jsonrpc":"2.0","id":"again","method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		got := h.recv()
		require.Equal(t, `"again"`, string(got.ID))
		require.Nil(t, got.Error, got.raw)
		require.Equal(t, "session-3", h.srv.session().id)

		once.Do(func() { close(release) })
		got = h.recv()
		assert.Equal(t, "1", string(got.ID))
		assert.Nil(t, got.Error, got.raw)
		assert.Equal(t, "session-3", h.srv.session().id, "the renewal did not overwrite the client's session")
		require.Nil(t, h.call(ping(2)).Error)
		last := ""
		for _, r := range h.heart.requests() {
			if r.RPC == "ping" {
				last = r.Session
			}
		}
		assert.Equal(t, "session-3", last, "the next call uses the client's session")
		require.Eventually(t, func() bool { return h.deletes("session-2") == 1 }, 3*time.Second, 5*time.Millisecond)
		h.srv.mu.Lock()
		pending := h.srv.resetPending
		h.srv.mu.Unlock()
		assert.False(t, pending, "the client's own initialize needs no warning")
	})

	t.Run("a failed initialize that got a session id: its DELETE arrives even with EOF right behind", func(t *testing.T) {
		h := start(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "initialize" {
				return false
			}
			w.Header().Set(contract.SessionHeader, "leaked-1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad params"}}`))
			return true
		})
		h.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
		require.NoError(t, h.in.Close())
		got := h.recv()
		require.NotNil(t, got.Error)
		assert.Equal(t, -32602, got.Error.Code)
		assert.Equal(t, 0, h.waitExit())
		assert.Equal(t, 1, h.deletes("leaked-1"))
	})

	renewal := func(rec recorded) bool { return strings.Contains(rec.Body, "anytype-mcp-renew") }
	for name, hook := range map[string]func(w http.ResponseWriter, r *http.Request, rec recorded) bool{
		"a 401 on the renewal's initialize": func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if renewal(rec) {
				rest(w, 401, "invalid api key")
				return true
			}
			return false
		},
		"a 401 on the renewal's notification": func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "notifications/initialized" && rec.Session != "session-1" {
				rest(w, 401, "invalid api key")
				return true
			}
			return false
		},
	} {
		t.Run(name+" is -32001, key rejected", func(t *testing.T) {
			h := start(t)
			h.initialize()
			h.heart.expireSessions()
			h.heart.setHook(hook)
			for _, line := range []string{ping(1), toolCall(2)} {
				got := h.call(line)
				require.NotNil(t, got.Error, got.raw)
				assert.Equal(t, codeNotConnected, got.Error.Code)
				assert.Contains(t, got.Error.Message, "no longer accepts")
			}
			assert.Equal(t, 1, h.heart.count("tools/call"), "only the attempt heart refused with 404")
		})
	}

	for name, hook := range map[string]func(w http.ResponseWriter, r *http.Request, rec recorded) bool{
		"a 200 JSON-RPC error answering the notification": func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "notifications/initialized" && rec.Session == "session-2" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"no"}}`))
				return true
			}
			return false
		},
		"an initialize reply under another id": func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if renewal(rec) {
				w.Header().Set(contract.SessionHeader, "session-2")
				rpcResult(w, json.RawMessage(`"someone-else"`), map[string]any{"protocolVersion": "2025-11-25"})
				return true
			}
			return false
		},
		"an initialize reply that is not JSON": func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if renewal(rec) {
				w.Header().Set(contract.SessionHeader, "session-2")
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"anytype-mcp-renew-1","result":{"protocolVersion":"2025-11-25"}}`))
				return true
			}
			return false
		},
	} {
		t.Run(name+" fails the renewal", func(t *testing.T) {
			h := start(t)
			h.initialize()
			h.heart.expireSessions()
			h.heart.setHook(hook)
			got := h.call(toolCall(1))
			require.Nil(t, got.Error, got.raw)
			assert.True(t, got.isError())
			assert.Contains(t, got.text(), "could not be renewed")
			assert.Equal(t, "session-1", h.srv.session().id, "nothing was installed")
			require.Eventually(t, func() bool { return h.deletes("session-2") == 1 }, 3*time.Second, 5*time.Millisecond,
				"the session the failed renewal opened is ended")
		})
	}
}

// K5: a canceled request is never answered.
func TestCanceledRequestGetsNoReply(t *testing.T) {
	stalled := func(t *testing.T) (*harness, *gateWriter) {
		gate := &gateWriter{closed: make(chan struct{})}
		h := start(t, withTier("full"), withOptions(func(o *Options) {
			o.StdoutStall = time.Minute
			gate.w = o.Out
			o.Out = gate
		}))
		h.initialize()
		return h, gate
	}
	cancelLine := func(id any) string {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": id}})
		return string(b)
	}

	t.Run("canceled while its worker waits for room in the reply queue", func(t *testing.T) {
		h, hold := stalled(t)
		hold.stalled.Store(true)
		for i := 0; i <= contract.QueueSize; i++ {
			h.send(ping(i))
		}
		// one reply is in the blocked write, 32 wait behind it
		require.Eventually(t, func() bool { return h.queued() == contract.QueueSize }, 3*time.Second, time.Millisecond)
		before := h.heart.count("ping")
		h.send(ping("late"))
		require.Eventually(t, func() bool { return h.heart.count("ping") == before+1 }, 3*time.Second, time.Millisecond)
		time.Sleep(50 * time.Millisecond) // its worker is now blocked on the full queue
		h.send(cancelLine("late"))
		require.Eventually(t, func() bool {
			h.srv.mu.Lock()
			defer h.srv.mu.Unlock()
			return h.srv.live[`s:late`] == nil
		}, 3*time.Second, time.Millisecond, "the blocked enqueue gave up")
		hold.open()
		seen := map[string]bool{}
		for i := 0; i <= contract.QueueSize; i++ {
			seen[string(h.recv().ID)] = true
		}
		h.silent()
		assert.False(t, seen[`"late"`])
		assert.Len(t, seen, contract.QueueSize+1)
	})

	t.Run("canceled while its reply is already queued", func(t *testing.T) {
		h, hold := stalled(t)
		hold.stalled.Store(true)
		h.send(ping("first"))
		require.Eventually(t, func() bool { return h.heart.count("ping") == 1 }, 3*time.Second, time.Millisecond)
		h.send(ping("second"))
		require.Eventually(t, func() bool { return h.queued() == 1 }, 3*time.Second, time.Millisecond)
		h.send(cancelLine("second"))
		require.Eventually(t, func() bool {
			h.srv.mu.Lock()
			defer h.srv.mu.Unlock()
			c := h.srv.live[`s:second`]
			return c != nil && c.canceled.Load()
		}, 3*time.Second, time.Millisecond)
		hold.open()
		assert.Equal(t, `"first"`, string(h.recv().ID))
		h.silent()
	})

	t.Run("a cancel after the line was written changes nothing", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		assert.Equal(t, "7", string(h.call(ping(7)).ID))
		h.send(cancelLine(7))
		got := h.call(ping(7))
		assert.Equal(t, "7", string(got.ID))
		assert.Nil(t, got.Error, "the id is free again and the new request is answered")
	})
}

// open lets a stalled gateWriter pass writes again.
func (g *gateWriter) open() {
	g.stalled.Store(false)
	g.reopen <- struct{}{}
}

// K6: the bearer is never relayed.
func TestKeyInA200ReplyIsWithheld(t *testing.T) {
	const htmlKey = "CANARY<key>&0123456789abcdefghijklmnopqrstuv"
	body := func(v any) string {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(v)
		return b.String()
	}
	cases := map[string]struct {
		key  string
		body func(id json.RawMessage, key string) string
	}{
		"in result text": {canaryKey, func(id json.RawMessage, key string) string {
			return body(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"content": []map[string]string{{"type": "text", "text": "your key is " + key}}, "isError": true}})
		}},
		"in error.message": {canaryKey, func(id json.RawMessage, key string) string {
			return body(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": "bad " + key}})
		}},
		"in error.data": {canaryKey, func(id json.RawMessage, key string) string {
			return body(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": "bad", "data": map[string]any{"auth": "Bearer " + key}}})
		}},
		"with HTML characters, unescaped": {htmlKey, func(id json.RawMessage, key string) string {
			return body(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"echo": key}})
		}},
		"with HTML characters, escaped the way gin does": {htmlKey, func(id json.RawMessage, key string) string {
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"echo": key}})
			return string(b)
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := start(t, withTier("full"), func(h *harness) {
				h.heart.key = tc.key
				it := h.item()
				it.Key = tc.key
				h.st.Put(testConn, it)
			})
			h.initialize()
			h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
				if rec.RPC != "tools/call" && rec.RPC != "tools/list" {
					return false
				}
				var msg struct {
					ID json.RawMessage `json:"id"`
				}
				_ = json.Unmarshal([]byte(rec.Body), &msg)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body(msg.ID, tc.key)))
				return true
			})
			got := h.call(toolCall(1))
			require.Nil(t, got.Error, got.raw)
			assert.True(t, got.isError())
			assert.Contains(t, got.text(), "was withheld")
			got = h.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
			require.NotNil(t, got.Error, got.raw)
			assert.Equal(t, codeInternal, got.Error.Code)
			assert.Contains(t, got.Error.Message, "was withheld")

			escaped, _ := json.Marshal(tc.key)
			out := h.all.String()
			assert.NotContains(t, out, tc.key)
			assert.NotContains(t, out, string(escaped[1:len(escaped)-1]))
			assert.NotContains(t, h.log.String(), tc.key)
			assert.Contains(t, h.log.String(), "withheld")
		})
	}
}

// K8: ids are matched by value, not by spelling.
func TestIDsAreMatchedByValue(t *testing.T) {
	t.Run("heart escapes HTML characters in the echoed id: still this request's reply", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		got := h.call(`{"jsonrpc":"2.0","id":"a&b<c>","method":"tools/call","params":{"name":"x"}}`)
		require.Nil(t, got.Error, got.raw)
		assert.False(t, got.isError(), got.raw)
		assert.Equal(t, "done", got.text())
		assert.Equal(t, `"a&b<c>"`, string(got.ID), "the client gets its own spelling back")
		reqs := h.heart.requests()
		assert.Contains(t, reqs[len(reqs)-1].Body, `"a&b<c>"`)
	})

	t.Run("a cancel that spells the id differently cancels it", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		release := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC == "tools/call" {
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}
			return false
		})
		h.send(`{"jsonrpc":"2.0","id":"a&b","method":"tools/call","params":{"name":"x"}}`)
		require.Eventually(t, func() bool { return h.heart.count("tools/call") == 1 }, 3*time.Second, time.Millisecond)
		h.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"a&b"}}`)
		h.silent()
		close(release)
		h.silent()
	})

	t.Run("numbers keep their digits; a string and a number are different ids", func(t *testing.T) {
		assert.Equal(t, "n:123456789012345678901234567890", idKey(json.RawMessage(`123456789012345678901234567890`)))
		assert.NotEqual(t, idKey(json.RawMessage(`1`)), idKey(json.RawMessage(`"1"`)))
		assert.Equal(t, idKey(json.RawMessage(`"a&b"`)), idKey(json.RawMessage(` "a&b" `)))
		assert.Empty(t, idKey(json.RawMessage(`null`)))
		assert.Empty(t, idKey(nil))
	})
}

// slowWriter takes a moment for every write call.
type slowWriter struct {
	w     io.Writer
	every time.Duration
	calls atomic.Int32
}

func (s *slowWriter) Write(p []byte) (int, error) {
	s.calls.Add(1)
	time.Sleep(s.every)
	return s.w.Write(p)
}

// K9: a large reply that keeps moving is not a stalled stdout.
func TestStallTimerMeasuresProgress(t *testing.T) {
	const stall = 300 * time.Millisecond
	slow := &slowWriter{every: stall / 2}
	h := start(t, withTier("full"), withOptions(func(o *Options) {
		o.StdoutStall = stall
		slow.w = o.Out
		o.Out = slow
	}))
	h.initialize()
	big := strings.Repeat("x", 6*writePiece+100)
	h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
		if rec.RPC != "tools/call" {
			return false
		}
		rpcResult(w, json.RawMessage("1"), map[string]any{"content": []map[string]string{{"type": "text", "text": big}}})
		return true
	})
	before := slow.calls.Load()
	began := time.Now()
	got := h.call(toolCall(1))
	assert.Equal(t, big, got.text(), "the whole line arrived")
	assert.Greater(t, time.Since(began), 3*stall, "the write took longer than the stall window")
	assert.GreaterOrEqual(t, slow.calls.Load()-before, int32(7), "written in 64 KiB pieces")
	assert.Nil(t, h.call(ping(2)).Error, "and the process is still serving")
}

// K11: a notification sent just after a port move is not lost.
func TestNotificationSurvivesAPortMove(t *testing.T) {
	h := start(t, withTier("full"))
	h.initialize()
	moved := newFakeHeart(t)
	h.stopHeart()
	it := h.item()
	it.Port, it.Rev = moved.port(), it.Rev+1
	h.st.Put(testConn, it)
	h.send(`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`)
	require.Eventually(t, func() bool { return moved.count("notifications/roots/list_changed") == 1 }, 3*time.Second, 5*time.Millisecond)
	h.silent()
}

// the K1 rule the transport relies on, stated once more at this level
func TestOnlyConfirmedAbsenceEndsAConnection(t *testing.T) {
	h := start(t)
	h.initialize()
	h.st.SetReadFail(store.Temporary(store.ReasonUnreachable, nil))
	h.clock.advance(6 * time.Second)
	require.Nil(t, h.call(ping(1)).Error)
	h.st.SetReadFail(nil)
	h.st.Remove(testConn)
	h.clock.advance(6 * time.Second)
	assert.Equal(t, codeNotConnected, h.call(ping(2)).Error.Code)
}
