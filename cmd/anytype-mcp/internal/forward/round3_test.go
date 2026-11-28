package forward

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingEndpoint's reads never return while blocked is set.
type blockingEndpoint struct {
	Endpoint
	blocked atomic.Bool
	forever chan struct{}
	stuck   atomic.Int32
}

func (b *blockingEndpoint) Read(ctx context.Context) (int, error) {
	if b.blocked.Load() {
		b.stuck.Add(1)
		<-b.forever
	}
	return b.Endpoint.Read(ctx)
}

func withBlockingEndpoint(b *blockingEndpoint, blockedAtStart bool) hopt {
	return withOptions(func(o *Options) {
		b.Endpoint = o.Endpoint
		b.forever = make(chan struct{})
		b.blocked.Store(blockedAtStart)
		o.Endpoint = b
		o.ReadTimeout = 100 * time.Millisecond
	})
}

// M1: endpoint I/O can never strand the forwarder past its deadlines.
func TestStuckStoreDoesNotStrandServe(t *testing.T) {
	t.Run("a read that never returns: not connected after the timeout, nothing sent, EOF still exits", func(t *testing.T) {
		b := &blockingEndpoint{}
		h := start(t, withBlockingEndpoint(b, false))
		t.Cleanup(func() { close(b.forever) })
		h.initialize()
		sent := len(h.heart.requests())
		b.blocked.Store(true)
		h.clock.advance(6 * time.Second)

		began := time.Now()
		got := h.call(toolCall(1))
		require.NotNil(t, got.Error, got.raw)
		assert.Equal(t, codeNotConnected, got.Error.Code)
		assert.Equal(t, msgUnreachable, got.Error.Message)
		assert.Less(t, time.Since(began), 2*time.Second, "waited for the read timeout, not for the read")
		require.Eventually(t, func() bool { return strings.Contains(h.log.String(), "did not answer in time") },
			3*time.Second, 5*time.Millisecond)

		// the stuck read is not waited for again, and not started twice
		h.clock.advance(6 * time.Second)
		began = time.Now()
		assert.Equal(t, codeNotConnected, h.call(toolCall(2)).Error.Code)
		assert.Less(t, time.Since(began), 80*time.Millisecond)
		assert.EqualValues(t, 1, b.stuck.Load(), "one abandoned read, not one per request")

		h.clock.advance(6 * time.Second)
		began = time.Now()
		require.NoError(t, h.in.Close())
		assert.Equal(t, 0, h.waitExit())
		assert.Less(t, time.Since(began), 2*time.Second, "EOF shuts down within the deadline")
		assert.Len(t, h.heart.requests(), sent, "nothing was sent, no DELETE at shutdown either")
	})

	t.Run("a read that never returns from the first request on: not connected, and a signal still exits", func(t *testing.T) {
		b := &blockingEndpoint{}
		h := start(t, withBlockingEndpoint(b, true))
		t.Cleanup(func() { close(b.forever) })
		began := time.Now()
		got := h.call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
		require.NotNil(t, got.Error, got.raw)
		assert.Equal(t, codeNotConnected, got.Error.Code)
		assert.Less(t, time.Since(began), 2*time.Second)
		assert.Empty(t, h.heart.requests())
		h.cancel()
		assert.Equal(t, 0, h.waitExit())
	})

	t.Run("after a late read the next attempt reads afresh", func(t *testing.T) {
		b := &blockingEndpoint{}
		h := start(t, withBlockingEndpoint(b, false), withTier("full"))
		h.initialize()
		moved := newFakeHeart(t)
		h.ep.set(moved.port())
		b.blocked.Store(true)
		h.clock.advance(6 * time.Second)
		require.NotNil(t, h.call(toolCall(1)).Error, "never the old port")
		b.blocked.Store(false)
		close(b.forever) // the stuck read comes back with the new port
		require.Eventually(t, func() bool {
			h.clock.advance(6 * time.Second)
			g := h.call(toolCall("again"))
			return g.Error == nil && !g.isError() && moved.count("tools/call") > 0
		}, 3*time.Second, 20*time.Millisecond)
	})
}

// M5: the stall timer runs on the monotonic clock; a wall-clock jump
// neither breaks a healthy stdout nor hides a stalled one.
func TestStallTimerIgnoresWallClockJumps(t *testing.T) {
	const stall = 300 * time.Millisecond

	t.Run("the clock jumps a day ahead during a slow write: the line still arrives", func(t *testing.T) {
		slow := &slowWriter{every: stall / 2}
		h := start(t, withTier("full"), withOptions(func(o *Options) {
			o.StdoutStall = stall
			slow.w = o.Out
			o.Out = slow
		}))
		h.initialize()
		big := strings.Repeat("x", 4*writePiece)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" {
				return false
			}
			rpcResult(w, []byte("1"), map[string]any{"content": []map[string]string{{"type": "text", "text": big}}})
			return true
		})
		h.send(toolCall(1))
		before := slow.calls.Load()
		require.Eventually(t, func() bool { return slow.calls.Load() > before }, 3*time.Second, time.Millisecond)
		h.clock.advance(24 * time.Hour)
		assert.Equal(t, big, h.recv().text())
		h.clock.advance(-48 * time.Hour)
		assert.Nil(t, h.call(ping(2)).Error, "and the process is still serving")
	})

	t.Run("the clock jumps a day back: a stalled stdout is still found", func(t *testing.T) {
		gate := &gateWriter{closed: make(chan struct{})}
		h := start(t, withOptions(func(o *Options) {
			o.StdoutStall = stall
			gate.w = o.Out
			o.Out = gate
		}))
		t.Cleanup(func() { close(gate.closed) })
		h.initialize()
		gate.stalled.Store(true)
		h.send(ping(1))
		h.clock.advance(-24 * time.Hour)
		began := time.Now()
		assert.Equal(t, 1, h.waitExit())
		assert.Less(t, time.Since(began), 3*time.Second)
	})
}
