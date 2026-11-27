package forward

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

func (h *harness) item() store.Item {
	raw, err := h.st.Read(context.Background(), testConn)
	require.NoError(h.t, err)
	it, err := store.DecodeItem(raw)
	require.NoError(h.t, err)
	return it
}

func TestStartAndPinning(t *testing.T) {
	notConnected := func(t *testing.T, h *harness) {
		t.Helper()
		for _, line := range []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
			`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
			toolCall(3),
		} {
			g := h.call(line)
			require.NotNil(t, g.Error, g.raw)
			assert.Equal(t, codeNotConnected, g.Error.Code)
			assert.Contains(t, g.Error.Message, "Settings ▸ AI apps")
		}
		h.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		h.silent()
	}

	t.Run("a start with no item stays not connected even after an item appears", func(t *testing.T) {
		h := start(t, withoutItem())
		notConnected(t, h)
		// another account connects under the same connection id
		h.st.Put(testConn, store.Item{V: 1, Rev: 1, Key: canaryKey, Account: "AccountTwo", Port: h.heart.port(), Tier: "small"})
		h.clock.advance(time.Minute)
		notConnected(t, h)
		assert.Empty(t, h.heart.requests(), "this process sends heart nothing")
		assert.Equal(t, 1, h.st.ReadCount(), "one read at start, never again")
	})

	t.Run("a start with a store that fails a check is final too", func(t *testing.T) {
		h := start(t, func(h *harness) { h.st.SetFail(store.Temporary(store.ReasonDenied, errors.New("a rule"))) })
		notConnected(t, h)
		h.st.SetFail(nil)
		h.clock.advance(time.Minute)
		notConnected(t, h)
		assert.Empty(t, h.heart.requests())
	})

	t.Run("an invalid item is no item", func(t *testing.T) {
		h := start(t, func(h *harness) {
			h.st.PutRaw(testConn, []byte(`{"v":1,"key":"x"}`))
		})
		notConnected(t, h)
	})

	t.Run("another account in the item ends the connection until restart", func(t *testing.T) {
		h := start(t)
		h.initialize()
		it := h.item()
		it.Account, it.Rev = "AccountTwo", it.Rev+1
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)
		before := len(h.heart.requests())
		g := h.call(toolCall(1))
		require.NotNil(t, g.Error, g.raw)
		assert.Equal(t, codeNotConnected, g.Error.Code)
		// the first account comes back: still dead
		it.Account, it.Rev = testAccount, it.Rev+1
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)
		assert.Equal(t, codeNotConnected, h.call(toolCall(2)).Error.Code)
		assert.Equal(t, before, len(h.heart.requests()), "no request reaches heart for the other account")
	})

	t.Run("another tier, and a deleted item, end the connection", func(t *testing.T) {
		h := start(t)
		h.initialize()
		it := h.item()
		it.Tier = "full"
		h.st.Put(testConn, it)
		h.clock.advance(6 * time.Second)
		assert.Equal(t, codeNotConnected, h.call(toolCall(1)).Error.Code)

		h = start(t)
		h.initialize()
		h.st.Remove(testConn)
		h.clock.advance(6 * time.Second)
		assert.Equal(t, codeNotConnected, h.call(toolCall(1)).Error.Code)
	})

	t.Run("a store that can't answer on a re-read keeps the last snapshot", func(t *testing.T) {
		for name, fail := range map[string]error{
			"denied":      store.Temporary(store.ReasonDenied, errors.New("a rule")),
			"unreachable": store.Temporary(store.ReasonUnreachable, errors.New("EIO")),
			"a raw error": errors.New("input/output error"),
		} {
			h := start(t)
			h.initialize()
			h.st.SetReadFail(fail)
			h.clock.advance(6 * time.Second)
			g := h.call(toolCall(1))
			assert.Nil(t, g.Error, name)
			assert.False(t, g.isError(), name)
			assert.Contains(t, h.log.String(), "keeping the last snapshot")
		}
	})

	t.Run("an item that became invalid ends the connection", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.st.PutRaw(testConn, []byte(`garbage`))
		h.clock.advance(6 * time.Second)
		assert.Equal(t, codeNotConnected, h.call(toolCall(1)).Error.Code)
	})

	t.Run("the item is re-read only when the last read is older than 5 s", func(t *testing.T) {
		h := start(t)
		h.initialize()
		reads := h.st.ReadCount()
		h.call(toolCall(1))
		h.call(toolCall(2))
		assert.Equal(t, reads, h.st.ReadCount())
		h.clock.advance(5 * time.Second)
		h.call(toolCall(3))
		assert.Equal(t, reads+1, h.st.ReadCount())
	})
}

func TestPortMove(t *testing.T) {
	t.Run("the old port still answers 401: the request fails, the next one uses the new port", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		// Anytype restarted on another port; something else answers 401 on the old one
		moved := newFakeHeart(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			rest(w, http.StatusUnauthorized, "invalid api key")
			return true
		})
		it := h.item()
		it.Port, it.Rev = moved.port(), it.Rev+1
		h.st.Put(testConn, it)

		g := h.call(toolCall(1))
		require.NotNil(t, g.Error, g.raw)
		assert.Equal(t, codeNotConnected, g.Error.Code)
		assert.Equal(t, 0, moved.count("tools/call"), "a 401 is never replayed")

		g = h.call(toolCall(2))
		assert.False(t, g.isError(), g.raw)
		assert.Nil(t, g.Error)
		assert.Equal(t, 1, moved.count("tools/call"))
	})

	t.Run("refused before sending: re-read, retried once on the new port", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		moved := newFakeHeart(t)
		h.stopHeart()
		it := h.item()
		it.Port, it.Rev = moved.port(), it.Rev+1
		h.st.Put(testConn, it)
		g := h.call(toolCall(1))
		assert.Nil(t, g.Error, g.raw)
		assert.False(t, g.isError())
		assert.Equal(t, 1, moved.count("tools/call"))
	})

	t.Run("refused twice: start Anytype", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		h.stopHeart()
		g := h.call(toolCall(1))
		require.NotNil(t, g.Error, g.raw)
		assert.Equal(t, codeNotConnected, g.Error.Code)
		assert.Contains(t, g.Error.Message, "Start Anytype")
	})
}

func TestBearerOnlyToLoopback(t *testing.T) {
	client := newHTTPClient()
	tr := client.Transport.(*http.Transport)
	assert.Nil(t, tr.Proxy)
	for _, addr := range []string{"localhost:80", "10.0.0.1:80", "[::1]:80", "example.com:443"} {
		_, err := tr.DialContext(t.Context(), "tcp", addr)
		require.Error(t, err, addr)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	conn, err := tr.DialContext(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()
	assert.Equal(t, "http://127.0.0.1:31009/mcp/full", endpoint(snapshot{port: 31009, tier: "full"}))

	t.Run("a redirect is not followed", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		other := newFakeHeart(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			http.Redirect(w, r, other.srv.URL+"/mcp/full", http.StatusTemporaryRedirect)
			return true
		})
		g := h.call(toolCall(1))
		assert.True(t, g.isError(), g.raw)
		assert.Empty(t, other.requests())
	})
}
