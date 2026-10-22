package payments

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock drives wall and monotonic time separately
type fakeClock struct {
	mu  sync.Mutex
	now clockReading
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: clockReading{wall: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), mono: time.Hour}}
}

func (c *fakeClock) read() clockReading {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance moves both clocks (the process is running)
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.add(d)
}

// sleep moves only the wall clock (macOS/Linux/mobile suspend)
func (c *fakeClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now.wall = c.now.wall.Add(d)
}

// stepWall moves only the wall clock (a manual clock change)
func (c *fakeClock) stepWall(d time.Duration) { c.sleep(d) }

type fetchCall struct {
	force bool
}

// newTestController returns a V2-style controller that is not started: tests
// drive nextWait/runFetch directly
func newTestController(t *testing.T, fetch func(ctx context.Context, force bool) (bool, error)) (*refreshController, *fakeClock) {
	clock := newFakeClock()
	rc := newRefreshController(context.Background(), fetch, time.Minute, 10*time.Second)
	rc.now = clock.read
	rc.spaceForcedPolls = true
	t.Cleanup(rc.cancel)
	return rc, clock
}

func noopFetch(context.Context, bool) (bool, error) { return false, nil }

func (rc *refreshController) intent(o forceOrigin) forceIntent {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.intents[o]
}

func TestRefreshControllerAdmission(t *testing.T) {
	t.Run("Force and AdmitManual never block on a fetch in progress", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		rc := newRefreshController(context.Background(), func(ctx context.Context, force bool) (bool, error) {
			once.Do(func() { close(started) })
			select {
			case <-release:
			case <-ctx.Done():
			}
			return false, nil
		}, time.Hour, 10*time.Second)
		rc.spaceForcedPolls = true
		rc.Start()
		defer rc.Stop()
		<-started // the initial periodic fetch is blocked

		done := make(chan struct{})
		go func() {
			for range 100 {
				rc.Force(30 * time.Minute)
				rc.AdmitManual(3 * time.Minute)
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("admission blocked on the controller")
		}
		close(release)
		assert.True(t, rc.intent(forceOriginPurchase).active)
		assert.True(t, rc.intent(forceOriginManual).active)
		assert.Equal(t, int64(1), rc.stats.admissions[forceOriginManual].Load())
		assert.Equal(t, int64(99), rc.stats.manualRejected.Load())
	})

	t.Run("manual: one admission per 30s, window capped at 3m from the original start", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)

		require.True(t, rc.AdmitManual(10*time.Minute))
		first := rc.intent(forceOriginManual)
		assert.Equal(t, clock.read().add(3*time.Minute), first.deadline)

		clock.advance(29 * time.Second)
		assert.False(t, rc.AdmitManual(3*time.Minute), "within 30s only fetches")

		clock.advance(time.Second)
		require.True(t, rc.AdmitManual(3*time.Minute))
		// extension can't slide the cap past original start + 3m
		assert.Equal(t, first.limit, rc.intent(forceOriginManual).deadline)

		// a shorter extension never shortens the window
		clock.advance(30 * time.Second)
		require.True(t, rc.AdmitManual(10*time.Second))
		assert.Equal(t, first.limit, rc.intent(forceOriginManual).deadline)
	})

	t.Run("pending purchase then manual: manual never shortens the purchase window", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.Force(30 * time.Minute)
		purchase := rc.intent(forceOriginPurchase)
		require.True(t, rc.AdmitManual(3*time.Minute))
		assert.Equal(t, purchase.deadline, rc.intent(forceOriginPurchase).deadline)

		clock.advance(4 * time.Minute)
		_, _ = rc.nextWait()
		assert.False(t, rc.intent(forceOriginManual).active, "manual expired")
		assert.True(t, rc.intent(forceOriginPurchase).active, "purchase continues")
	})

	t.Run("pending manual then purchase: both kept with their own deadlines", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		require.True(t, rc.AdmitManual(3*time.Minute))
		manual := rc.intent(forceOriginManual)
		clock.advance(time.Minute)
		rc.Force(30 * time.Minute)
		assert.Equal(t, manual.deadline, rc.intent(forceOriginManual).deadline)
		assert.Equal(t, clock.read().add(30*time.Minute), rc.intent(forceOriginPurchase).deadline)
	})

	t.Run("purchase intents merge by keeping the longest deadline", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.Force(30 * time.Minute)
		long := rc.intent(forceOriginPurchase).deadline
		clock.advance(time.Minute)
		rc.Force(time.Minute)
		assert.Equal(t, long, rc.intent(forceOriginPurchase).deadline)
	})
}

func TestRefreshControllerScheduling(t *testing.T) {
	t.Run("explicit Refresh fetch counts as the first forced attempt", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.runFetch() // initial periodic fetch
		clock.advance(30 * time.Second)

		require.True(t, rc.AdmitManual(3*time.Minute))
		wait, ok := rc.nextWait()
		require.True(t, ok)
		assert.Equal(t, 10*time.Second, wait, "no immediate duplicate of the explicit fetch")
	})

	t.Run("a new purchase window polls immediately, extensions don't reset the next poll", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.Force(30 * time.Minute)
		wait, ok := rc.nextWait()
		require.True(t, ok)
		assert.Zero(t, wait)
		rc.runFetch()

		clock.advance(3 * time.Second)
		rc.Force(30 * time.Minute)
		wait, _ = rc.nextWait()
		assert.Equal(t, 7*time.Second, wait)

		clock.advance(31 * time.Second) // past the manual rate limit window
		rc.runFetch()
		clock.advance(2 * time.Second)
		require.True(t, rc.AdmitManual(3*time.Minute))
		wait, _ = rc.nextWait()
		assert.Equal(t, 10*time.Second, wait, "forced polls stay at least 10s apart")
	})

	t.Run("without a window the periodic interval applies", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		wait, ok := rc.nextWait()
		require.True(t, ok)
		assert.Zero(t, wait, "first fetch right away")
		rc.runFetch()
		clock.advance(10 * time.Second)
		wait, _ = rc.nextWait()
		assert.Equal(t, 50*time.Second, wait)
	})

	t.Run("sleep before the intent is consumed: wall-clock expiry drops it", func(t *testing.T) {
		var forces []bool
		rc, clock := newTestController(t, func(_ context.Context, force bool) (bool, error) {
			forces = append(forces, force)
			return false, nil
		})
		require.True(t, rc.AdmitManual(3*time.Minute))
		clock.sleep(10 * time.Minute) // monotonic paused
		rc.runFetch()
		assert.Equal(t, []bool{false}, forces, "expired before the forced fetch")
		assert.False(t, rc.intent(forceOriginManual).active)
		assert.Equal(t, int64(1), rc.stats.expired[forceOriginManual].Load())
	})

	t.Run("sleep counts toward the manual gap and the forced poll spacing", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		require.True(t, rc.AdmitManual(3*time.Minute))
		clock.sleep(30 * time.Second) // monotonic paused
		assert.True(t, rc.AdmitManual(3*time.Minute), "30s of wall time passed")

		wait, ok := rc.nextWait()
		require.True(t, ok)
		assert.Equal(t, 10*time.Second, wait)
		clock.sleep(8 * time.Second)
		wait, _ = rc.nextWait()
		assert.Equal(t, 2*time.Second, wait, "a sleep counts toward the 10s spacing")
	})

	t.Run("failure state follows the last fetch (log transitions)", func(t *testing.T) {
		var fail bool
		rc, _ := newTestController(t, func(context.Context, bool) (bool, error) {
			if fail {
				return false, net.ErrUnableToConnect
			}
			return false, nil
		})
		fail = true
		rc.runFetch()
		assert.True(t, rc.failing)
		fail = false
		rc.runFetch()
		assert.False(t, rc.failing)
	})

	t.Run("no fetch after Stop", func(t *testing.T) {
		var calls int
		var mu sync.Mutex
		// the ready 0-wait timer races ctx.Done in select: repeat
		for range 50 {
			rc := newRefreshController(context.Background(), func(ctx context.Context, force bool) (bool, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				return false, nil
			}, time.Hour, 10*time.Second)
			rc.cancel() // stopped before the first, immediate fetch
			rc.Start()
			rc.Stop()
		}
		mu.Lock()
		assert.Zero(t, calls)
		mu.Unlock()
	})

	t.Run("a backward wall-clock step doesn't stretch a deadline", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.Force(time.Minute)
		clock.stepWall(-time.Hour)
		clock.advance(61 * time.Second)
		_, _ = rc.nextWait()
		assert.False(t, rc.intent(forceOriginPurchase).active)
	})

	t.Run("deadlines are fixed at admission, not when the loop consumes the intent", func(t *testing.T) {
		rc, clock := newTestController(t, noopFetch)
		rc.Force(time.Minute)
		clock.advance(59 * time.Second)
		rc.runFetch()
		assert.True(t, rc.intent(forceOriginPurchase).active)
		clock.advance(time.Second)
		_, _ = rc.nextWait()
		assert.False(t, rc.intent(forceOriginPurchase).active)
	})

	t.Run("an older fetch's changed=true doesn't clear an intent admitted during it", func(t *testing.T) {
		admitDuring := true
		var rc *refreshController
		rc, _ = newTestController(t, func(_ context.Context, force bool) (bool, error) {
			if admitDuring {
				admitDuring = false
				require.True(t, rc.AdmitManual(3*time.Minute))
			}
			return true, nil
		})
		rc.Force(30 * time.Minute)
		rc.runFetch()
		assert.False(t, rc.intent(forceOriginPurchase).active, "the fetch's own changed=true stops the older intent")
		assert.True(t, rc.intent(forceOriginManual).active, "the newer intent survives")
		assert.Equal(t, int64(1), rc.stats.stoppedOnChange.Load())

		rc.runFetch()
		assert.False(t, rc.intent(forceOriginManual).active)
	})
}

func TestRefreshControllerV1Compatibility(t *testing.T) {
	t.Run("every Force polls immediately", func(t *testing.T) {
		clock := newFakeClock()
		rc := newRefreshController(context.Background(), noopFetch, time.Minute, 10*time.Second)
		rc.now = clock.read
		defer rc.cancel()

		rc.Force(30 * time.Minute)
		wait, _ := rc.nextWait()
		assert.Zero(t, wait)
		rc.runFetch()
		clock.advance(time.Second)
		wait, _ = rc.nextWait()
		assert.Equal(t, 9*time.Second, wait)
		rc.Force(30 * time.Minute)
		wait, _ = rc.nextWait()
		assert.Zero(t, wait, "V1 keeps resetting the poll on Force")
	})

	t.Run("no periodic fetch with interval 0", func(t *testing.T) {
		rc := newRefreshController(context.Background(), noopFetch, 0, 10*time.Second)
		defer rc.cancel()
		_, ok := rc.nextWait()
		assert.False(t, ok)
	})

	t.Run("a Force during a running fetch is honoured right after it", func(t *testing.T) {
		var (
			mu    sync.Mutex
			calls []bool
		)
		started := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		forced := make(chan struct{})
		rc := newRefreshController(context.Background(), func(ctx context.Context, force bool) (bool, error) {
			mu.Lock()
			calls = append(calls, force)
			mu.Unlock()
			once.Do(func() { close(started) })
			if force {
				close(forced)
				return true, nil
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return false, nil
		}, time.Hour, 10*time.Second)
		rc.Start()
		defer rc.Stop()
		<-started
		rc.Force(30 * time.Minute)
		close(release)
		select {
		case <-forced:
		case <-time.After(time.Second):
			t.Fatal("forced fetch didn't follow")
		}
	})
}
