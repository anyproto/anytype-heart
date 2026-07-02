package crossspacesub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

func addMsg(id string) *pb.EventMessage {
	return event.NewMessage("s", &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: id, SubId: "cs"},
	})
}

func countersMsg(total int64) *pb.EventMessage {
	return event.NewMessage("s", &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{Total: total, SubId: "cs"},
	})
}

func countersTotals(msgs []*pb.EventMessage) []int64 {
	var out []int64
	for _, m := range msgs {
		if c := m.GetSubscriptionCounters(); c != nil {
			out = append(out, c.Total)
		}
	}
	return out
}

func TestCoalesceCounters(t *testing.T) {
	t.Run("keeps only the last counters, at the tail", func(t *testing.T) {
		in := []*pb.EventMessage{countersMsg(1), addMsg("a"), countersMsg(2), addMsg("b"), countersMsg(3)}
		out := coalesceCounters(in)
		// 2 adds + exactly 1 counters
		require.Len(t, out, 3)
		assert.Equal(t, []int64{3}, countersTotals(out))
		assert.Nil(t, out[len(out)-1].GetSubscriptionAdd(), "counters must be last")
		assert.NotNil(t, out[len(out)-1].GetSubscriptionCounters())
	})
	t.Run("no counters: unchanged", func(t *testing.T) {
		in := []*pb.EventMessage{addMsg("a"), addMsg("b")}
		assert.Equal(t, in, coalesceCounters(in))
	})
}

func adds(n int) []*pb.EventMessage {
	out := make([]*pb.EventMessage, n)
	for i := range out {
		out[i] = addMsg("x")
	}
	return out
}

func flatLen(batches [][]*pb.EventMessage) int {
	n := 0
	for _, b := range batches {
		n += len(b)
	}
	return n
}

func TestCoalescer(t *testing.T) {
	t0 := time.Unix(1000, 0)
	grace := 200 * time.Millisecond
	window := 50 * time.Millisecond

	t.Run("first flush held until createdAt+grace", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		c.push(t0, adds(3))
		assert.Nil(t, c.ready(t0), "must hold during grace")
		assert.Nil(t, c.ready(t0.Add(grace-time.Millisecond)), "still within grace")
		out := c.ready(t0.Add(grace))
		require.Len(t, out, 1, "single broadcast batch")
		assert.Equal(t, 3, flatLen(out), "flush at grace deadline")
	})

	t.Run("late first event flushes after only window, not grace", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		late := t0.Add(grace + time.Second) // first event arrives well after grace
		c.push(late, adds(2))
		assert.Nil(t, c.ready(late), "held only for the window from the first message")
		out := c.ready(late.Add(window))
		require.Len(t, out, 1)
		assert.Equal(t, 2, flatLen(out))
	})

	t.Run("large first wave still held for grace (M1 regression)", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		c.push(t0, adds(maxFlushSize+50)) // > cap
		assert.Nil(t, c.ready(t0.Add(grace-time.Millisecond)), "size cap must not pre-empt grace")
		out := c.ready(t0.Add(grace))
		require.Len(t, out, 2)
		assert.Len(t, out[0], maxFlushSize)
		assert.Len(t, out[1], 50)
	})

	t.Run("window coalesces waves that arrive close together", func(t *testing.T) {
		c2 := newCoalescer(t0, 0, window, maxFlushSize)
		c2.push(t0, adds(2))
		assert.Nil(t, c2.ready(t0), "within window")
		c2.push(t0.Add(window/2), adds(3)) // second wave inside the window
		assert.Nil(t, c2.ready(t0.Add(window/2)))
		out := c2.ready(t0.Add(window))
		require.Len(t, out, 1, "both waves coalesced into a single broadcast batch")
		assert.Equal(t, 5, flatLen(out), "both waves in one flush")
	})

	t.Run("size cap chunks to a hard bound; sub-cap remainder waits for window", func(t *testing.T) {
		c := newCoalescer(t0, 0, window, maxFlushSize)
		c.firstDone = true // later flush: no grace
		c.push(t0, adds(maxFlushSize+120))
		out := c.ready(t0) // not timeUp, but over cap -> one full chunk only
		require.Len(t, out, 1)
		assert.Len(t, out[0], maxFlushSize)
		assert.Nil(t, c.ready(t0), "remainder (120) waits for window")
		out = c.ready(t0.Add(window))
		require.Len(t, out, 1)
		assert.Len(t, out[0], 120)
	})

	t.Run("empty buffer: no batches, zero deadline", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		assert.Nil(t, c.ready(t0))
		assert.True(t, c.nextDeadline().IsZero())
	})
}
