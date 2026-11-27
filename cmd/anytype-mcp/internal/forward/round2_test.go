package forward

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// uesc spells one character as a JSON unicode escape.
func uesc(r rune) string { return `\` + fmt.Sprintf("u%04x", r) }

func (h *harness) retarget(to *fakeHeart) {
	it := h.item()
	it.Port, it.Rev = to.port(), it.Rev+1
	h.st.Put(testConn, it)
	h.clock.advance(6 * time.Second)
}

// L1: a session's DELETE and the renewal's notification go only to a port
// the item still names.
func TestSessionFollowUpsStayHome(t *testing.T) {
	t.Run("a replaced session opened on the old port gets no DELETE there", func(t *testing.T) {
		h := start(t)
		h.initialize() // session-1 on port A
		moved := newFakeHeart(t)
		moved.opened = 10 // its session ids differ from the old port's
		h.retarget(moved)
		got := h.call(`{"jsonrpc":"2.0","id":"again","method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		require.Nil(t, got.Error, got.raw)
		require.Equal(t, 1, moved.count("initialize"), "the new initialize ran on the new port")
		require.Eventually(t, func() bool { return strings.Contains(h.log.String(), "left to expire") }, 3*time.Second, 5*time.Millisecond)
		assert.Zero(t, h.deletes(""), "the old port sees no DELETE and no bearer")
		// and at shutdown only the session that lives on the new port is ended
		require.NoError(t, h.in.Close())
		h.waitExit()
		assert.Zero(t, h.deletes(""))
		n := 0
		for _, r := range moved.requests() {
			if r.Method == http.MethodDelete {
				n++
			}
		}
		assert.Equal(t, 1, n)
	})

	t.Run("a renewal whose first leg outlived a port move sends nothing more to the old port", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.expireSessions()
		release := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if strings.Contains(rec.Body, "anytype-mcp-renew") {
				<-release
			}
			return false
		})
		h.send(toolCall(1))
		require.Eventually(t, func() bool { return h.heart.count("initialize") == 2 }, 3*time.Second, time.Millisecond)
		moved := newFakeHeart(t)
		h.retarget(moved)
		close(release)

		got := h.recv()
		require.Nil(t, got.Error, got.raw)
		assert.True(t, got.isError())
		assert.Contains(t, got.text(), "could not be renewed")
		time.Sleep(150 * time.Millisecond)
		for _, r := range h.heart.requests() {
			if r.Session == "session-2" {
				t.Errorf("the old port got a follow-up on the session opened there: %s %s", r.Method, r.RPC)
			}
		}
		assert.Zero(t, h.deletes(""))
		assert.Empty(t, moved.requests(), "and nothing of that renewal went to the new port")
	})
}

// L2: only the written warning clears the reset gate; a client initialize
// does not.
func TestInitializeDoesNotClearTheResetGate(t *testing.T) {
	g := &holdWriter{hold: func(p []byte) bool { return bytes.Contains(p, []byte("session expired")) }}
	h := start(t, withHeldStdout(g))
	h.initialize()
	h.heart.expireSessions()

	h.send(toolCall(1)) // 404, renewal (session-2), and the warning blocks in the write
	require.Eventually(t, func() bool { return g.held.Load() == 1 }, 3*time.Second, time.Millisecond)
	h.send(toolCall(2))
	h.send(`{"jsonrpc":"2.0","id":"again","method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	require.Eventually(t, func() bool { return h.srv.session().id == "session-3" }, 3*time.Second, time.Millisecond,
		"the client's initialize completed while the warning is still unwritten")
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 1, h.heart.count("tools/call"), "no tools/call on any session before the warning is written")

	close(g.release)
	seen := map[string]got{}
	for i := 0; i < 3; i++ {
		r := h.recv()
		seen[string(r.ID)] = r
	}
	assert.Contains(t, seen["1"].text(), "session expired")
	assert.Contains(t, seen["2"].text(), "session expired", "the call sent before the warning was read did not run")
	assert.Nil(t, seen[`"again"`].Error)
	assert.Equal(t, 1, h.heart.count("tools/call"))

	h.warned()
	assert.False(t, h.call(toolCall(3)).isError())
	last := ""
	for _, r := range h.heart.requests() {
		if r.RPC == "tools/call" {
			last = r.Session
		}
	}
	assert.Equal(t, "session-3", last, "the next call runs on the client's session")
}

// strings collects every string a JSON document decodes to, member names
// included.
func decodedStrings(t *testing.T, doc string) []string {
	var out []string
	dec := json.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out
		}
		require.NoError(t, err, doc)
		if s, ok := tok.(string); ok {
			out = append(out, s)
		}
	}
}

// L5: the bearer check reads decoded strings, however JSON spells them.
func TestKeyWithheldHoweverItIsSpelled(t *testing.T) {
	// the canary with its first letter written as a unicode escape: no
	// byte-level search finds it, every JSON decoder does
	spelled := uesc(rune(canaryKey[0])) + canaryKey[1:]
	require.NotContains(t, spelled, canaryKey)
	bodies := map[string]string{
		"the result text":            `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"key ` + spelled + `"}]}}`,
		"a nested structuredContent": `{"jsonrpc":"2.0","id":%s,"result":{"content":[],"structuredContent":{"a":[{"b":{"c":"` + spelled + `"}}]}}}`,
		"a member name":              `{"jsonrpc":"2.0","id":%s,"result":{"` + spelled + `":1}}`,
		"error.message":              `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"bad ` + spelled + `"}}`,
		"error.data":                 `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"bad","data":{"auth":["Bearer ` + spelled + `"]}}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			h := start(t, withTier("full"))
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
				_, _ = fmt.Fprintf(w, body, msg.ID)
				return true
			})
			r := h.call(toolCall(1))
			require.Nil(t, r.Error, r.raw)
			assert.True(t, r.isError())
			assert.Contains(t, r.text(), "was withheld")
			r = h.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
			require.NotNil(t, r.Error, r.raw)
			assert.Contains(t, r.Error.Message, "was withheld")

			for _, line := range strings.Split(strings.TrimSpace(h.all.String()), "\n") {
				for _, s := range decodedStrings(t, line) {
					assert.NotContains(t, s, canaryKey, "no string the client decodes holds the key")
				}
			}
		})
	}

	t.Run("a body that can't be walked to its end is not relayed", func(t *testing.T) {
		hit, ok := carriesKey([]byte(`{"jsonrpc":"2.0","id":1,"result":{"a":"b"}} trailing {`), canaryKey)
		assert.False(t, hit)
		assert.False(t, ok)
		hit, ok = carriesKey([]byte(`{"a":["x",{"`+spelled+`":null}]}`), canaryKey)
		assert.True(t, hit)
		assert.True(t, ok)
		hit, ok = carriesKey([]byte(`{"a":1.5e300,"b":"fine"}`), canaryKey)
		assert.False(t, hit)
		assert.True(t, ok)
	})
}

// stuckStore's first read never returns until released.
type stuckStore struct {
	*store.Mem
	release chan struct{}
}

func (s *stuckStore) Read(ctx context.Context, id string) ([]byte, error) {
	<-s.release
	return s.Mem.Read(ctx, id)
}

// L9: shutdown covers the first read and hands off late sessions.
func TestShutdownEdges(t *testing.T) {
	t.Run("a signal ends the process while the first read of the item is stalled", func(t *testing.T) {
		st := &stuckStore{Mem: store.NewMem(), release: make(chan struct{})}
		t.Cleanup(func() { close(st.release) })
		ctx, cancel := context.WithCancel(context.Background())
		exit := make(chan int, 1)
		inR, inW := io.Pipe()
		t.Cleanup(func() { _ = inW.Close() })
		go func() {
			exit <- Serve(ctx, Options{ConnectionID: testConn, Store: st, In: inR, Out: io.Discard, ShutdownDeadline: time.Second})
		}()
		time.Sleep(50 * time.Millisecond)
		began := time.Now()
		cancel()
		select {
		case code := <-exit:
			assert.Equal(t, 0, code)
			assert.Less(t, time.Since(began), 2*time.Second)
		case <-time.After(5 * time.Second):
			t.Fatal("Serve did not return: the stalled read held the signal back")
		}
	})

	t.Run("a session id that arrives during shutdown is still ended", func(t *testing.T) {
		h := start(t)
		headersSent := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "initialize" {
				return false
			}
			// the headers, session id included, and then no body
			w.Header().Set(contract.SessionHeader, "late-1")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(headersSent)
			<-r.Context().Done()
			return true
		})
		h.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
		<-headersSent
		time.Sleep(50 * time.Millisecond)
		h.cancel() // SIGTERM
		assert.Equal(t, 0, h.waitExit())
		assert.Equal(t, 1, h.deletes("late-1"), "the worker handed the id over before cleanup closed")
	})
}
