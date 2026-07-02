package spacev2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const testSpaceId = "space1"

// fakeSpace is a stand-in for a loaded space; the engine never calls methods
// on it, so the nil embed is safe. Instances are compared by pointer.
type fakeSpace struct {
	clientspace.Space
}

// fakeBackend is a scriptable Backend. Error queues are consumed one per
// call; an exhausted queue means success. It records step calls and detects
// concurrent step execution (which the controller must never allow).
type fakeBackend struct {
	mu sync.Mutex

	status     spaceinfo.AccountStatus
	statusErrs []error

	loadErrs    []error
	unloadErrs  []error
	offloadErrs []error
	joinFn      func(ctx context.Context) error

	loadGate chan struct{} // when set, Load blocks until closed or ctx done

	calls      []string
	spaces     []*fakeSpace
	active     int32
	violations int32
}

func newFakeBackend(status spaceinfo.AccountStatus) *fakeBackend {
	return &fakeBackend{status: status}
}

func (b *fakeBackend) enter(name string) func() {
	if atomic.AddInt32(&b.active, 1) != 1 {
		atomic.AddInt32(&b.violations, 1)
	}
	b.mu.Lock()
	b.calls = append(b.calls, name)
	b.mu.Unlock()
	return func() { atomic.AddInt32(&b.active, -1) }
}

func popErr(q *[]error) error {
	if len(*q) == 0 {
		return nil
	}
	err := (*q)[0]
	*q = (*q)[1:]
	return err
}

func (b *fakeBackend) AccountStatus(ctx context.Context) (spaceinfo.AccountStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := popErr(&b.statusErrs); err != nil {
		return 0, err
	}
	return b.status, nil
}

func (b *fakeBackend) Load(ctx context.Context) (clientspace.Space, error) {
	defer b.enter("load")()
	b.mu.Lock()
	gate := b.loadGate
	err := popErr(&b.loadErrs)
	b.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	sp := &fakeSpace{}
	b.mu.Lock()
	b.spaces = append(b.spaces, sp)
	b.mu.Unlock()
	return sp, nil
}

func (b *fakeBackend) Unload(ctx context.Context, sp clientspace.Space) error {
	defer b.enter("unload")()
	b.mu.Lock()
	defer b.mu.Unlock()
	return popErr(&b.unloadErrs)
}

func (b *fakeBackend) Offload(ctx context.Context) error {
	defer b.enter("offload")()
	b.mu.Lock()
	defer b.mu.Unlock()
	return popErr(&b.offloadErrs)
}

func (b *fakeBackend) Join(ctx context.Context) error {
	defer b.enter("join")()
	b.mu.Lock()
	fn := b.joinFn
	b.mu.Unlock()
	if fn == nil {
		return errors.New("unexpected join")
	}
	return fn(ctx)
}

func (b *fakeBackend) setStatus(s spaceinfo.AccountStatus) {
	b.mu.Lock()
	b.status = s
	b.mu.Unlock()
}

func (b *fakeBackend) callList() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

func (b *fakeBackend) callCount(name string) int {
	var n int
	for _, c := range b.callList() {
		if c == name {
			n++
		}
	}
	return n
}

type ctrlFixture struct {
	*controller
	backend *fakeBackend
}

// newCtrlFixture builds a fixture; setup functions run on the backend BEFORE
// the controller (and its reconcile goroutine) starts — required whenever the
// initial status makes the loop act immediately (e.g. Joining runs Join).
func newCtrlFixture(t *testing.T, status spaceinfo.AccountStatus, setup ...func(b *fakeBackend)) *ctrlFixture {
	backend := newFakeBackend(status)
	for _, fn := range setup {
		fn(backend)
	}
	ctrl := newController(testSpaceId, backend, controllerOptions{
		retryMin: 2 * time.Millisecond,
		retryMax: 10 * time.Millisecond,
	})
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, ctrl.Close(closeCtx))
		assert.Zero(t, atomic.LoadInt32(&backend.violations), "backend steps ran concurrently")
	})
	return &ctrlFixture{controller: ctrl, backend: backend}
}

func waitState(t *testing.T, c *controller, want State) {
	t.Helper()
	require.Eventuallyf(t, func() bool { return c.State() == want },
		time.Second, time.Millisecond, "state never became %s (last: %s)", want, c.State())
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name   string
		status spaceinfo.AccountStatus
		wanted bool
		want   Target
	}{
		{"active wanted", spaceinfo.AccountStatusActive, true, TargetLoaded},
		{"active not wanted", spaceinfo.AccountStatusActive, false, TargetIdle},
		{"unknown wanted", spaceinfo.AccountStatusUnknown, true, TargetLoaded},
		{"unknown not wanted", spaceinfo.AccountStatusUnknown, false, TargetIdle},
		{"joining wanted", spaceinfo.AccountStatusJoining, true, TargetJoining},
		{"joining not wanted", spaceinfo.AccountStatusJoining, false, TargetJoining},
		{"deleted wins over demand", spaceinfo.AccountStatusDeleted, true, TargetOffloaded},
		{"deleted not wanted", spaceinfo.AccountStatusDeleted, false, TargetOffloaded},
		{"removing treated as deleted", spaceinfo.AccountStatusRemoving, true, TargetOffloaded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decide(tc.status, tc.wanted))
		})
	}
}

func TestConverged(t *testing.T) {
	tests := []struct {
		state  State
		target Target
		want   bool
	}{
		{StateIdle, TargetIdle, true},
		{StateOffloaded, TargetIdle, true},
		{StateLoaded, TargetLoaded, true},
		{StateOffloaded, TargetOffloaded, true},
		{StateIdle, TargetLoaded, false},
		{StateOffloaded, TargetLoaded, false},
		{StateLoaded, TargetIdle, false},
		{StateLoaded, TargetOffloaded, false},
		{StateIdle, TargetOffloaded, false},
		{StateIdle, TargetJoining, false},
		{StateOffloaded, TargetJoining, false},
	}
	for _, tc := range tests {
		assert.Equalf(t, tc.want, converged(tc.state, tc.target),
			"converged(%s, %s)", tc.state, tc.target)
	}
}

func TestLoadHappyPath(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)

	// when
	fx.SetWanted(true)
	sp, err := fx.WaitLoaded(context.Background())

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, StateLoaded, fx.State())
	assert.Equal(t, 1, fx.backend.callCount("load"))
	assert.Same(t, sp, fx.SpaceIfLoaded())
}

func TestNotWantedStaysIdle(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)

	// when
	fx.Poke()
	fx.Poke()
	time.Sleep(50 * time.Millisecond)

	// then
	assert.Equal(t, StateIdle, fx.State())
	assert.Zero(t, fx.backend.callCount("load"))
}

func TestPauseUnloadReload(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.SetWanted(true)
	first, err := fx.WaitLoaded(context.Background())
	require.NoError(t, err)

	// when: pause
	fx.SetWanted(false)
	waitState(t, fx.controller, StateIdle)

	// then
	assert.Equal(t, 1, fx.backend.callCount("unload"))
	assert.Nil(t, fx.SpaceIfLoaded())

	// when: re-promote
	fx.SetWanted(true)
	second, err := fx.WaitLoaded(context.Background())

	// then: a fresh space instance was built
	require.NoError(t, err)
	assert.Equal(t, 2, fx.backend.callCount("load"))
	assert.NotSame(t, first, second)
}

func TestDeleteWhileLoaded(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.SetWanted(true)
	_, err := fx.WaitLoaded(context.Background())
	require.NoError(t, err)

	// when
	fx.backend.setStatus(spaceinfo.AccountStatusDeleted)
	fx.Poke()
	waitState(t, fx.controller, StateOffloaded)

	// then: unload strictly precedes offload
	assert.Equal(t, []string{"load", "unload", "offload"}, fx.backend.callList())
	_, err = fx.WaitLoaded(context.Background())
	assert.ErrorIs(t, err, ErrSpaceDeleted)
}

func TestOffloadWithoutLoad(t *testing.T) {
	// given: a space that was never resident this session
	fx := newCtrlFixture(t, spaceinfo.AccountStatusDeleted)

	// when
	fx.Poke()
	waitState(t, fx.controller, StateOffloaded)

	// then
	assert.Equal(t, []string{"offload"}, fx.backend.callList())
}

func TestRestoreAfterOffload(t *testing.T) {
	// given: an offloaded (deleted) space
	fx := newCtrlFixture(t, spaceinfo.AccountStatusDeleted)
	waitState(t, fx.controller, StateOffloaded)

	// when: cancel-leave / restore
	fx.backend.setStatus(spaceinfo.AccountStatusActive)
	fx.SetWanted(true)
	sp, err := fx.WaitLoaded(context.Background())

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, []string{"offload", "load"}, fx.backend.callList())
}

func TestJoinAcceptedThenLoads(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusJoining, func(b *fakeBackend) {
		b.joinFn = func(ctx context.Context) error {
			b.setStatus(spaceinfo.AccountStatusActive)
			return nil
		}
	})

	// when
	fx.SetWanted(true)
	sp, err := fx.WaitLoaded(context.Background())

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, []string{"join", "load"}, fx.backend.callList())
}

func TestJoinRejectedOffloads(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusJoining, func(b *fakeBackend) {
		b.joinFn = func(ctx context.Context) error {
			b.setStatus(spaceinfo.AccountStatusDeleted)
			return nil
		}
	})

	// when
	fx.SetWanted(true)
	waitState(t, fx.controller, StateOffloaded)

	// then
	assert.Equal(t, []string{"join", "offload"}, fx.backend.callList())
	_, err := fx.WaitLoaded(context.Background())
	assert.ErrorIs(t, err, ErrSpaceDeleted)
}

func TestRetryOnTransientLoadError(t *testing.T) {
	// given: two transient failures, then success
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.backend.mu.Lock()
	fx.backend.loadErrs = []error{errors.New("boom"), errors.New("boom")}
	fx.backend.mu.Unlock()

	// when
	fx.SetWanted(true)
	sp, err := fx.WaitLoaded(context.Background())

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, 3, fx.backend.callCount("load"))
}

func TestFatalLoadErrorParksAndSurfaces(t *testing.T) {
	// given
	fatal := errors.New("storage gone")
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.backend.mu.Lock()
	fx.backend.loadErrs = []error{Fatal(fatal)}
	fx.backend.mu.Unlock()

	// when
	fx.SetWanted(true)
	_, err := fx.WaitLoaded(context.Background())

	// then: the fatal error surfaces to waiters
	require.ErrorIs(t, err, fatal)

	// and: no timer-driven retry happens while parked
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, fx.backend.callCount("load"))

	// when: an input event arrives
	fx.Poke()
	sp, err := fx.WaitLoaded(context.Background())

	// then: the controller re-attempted and recovered
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, 2, fx.backend.callCount("load"))
}

func TestStatusReadErrorRetries(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.backend.mu.Lock()
	fx.backend.statusErrs = []error{errors.New("view not ready"), errors.New("view not ready")}
	fx.backend.mu.Unlock()

	// when
	fx.SetWanted(true)
	sp, err := fx.WaitLoaded(context.Background())

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
}

func TestConcurrentWaitersGetSameSpace(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	const waiters = 10
	spaces := make([]clientspace.Space, waiters)
	var wg sync.WaitGroup

	// when
	for i := range waiters {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sp, err := fx.WaitLoaded(context.Background())
			assert.NoError(t, err)
			spaces[i] = sp
		}(i)
	}
	fx.SetWanted(true)
	wg.Wait()

	// then
	assert.Equal(t, 1, fx.backend.callCount("load"))
	for i := 1; i < waiters; i++ {
		assert.Same(t, spaces[0], spaces[i])
	}
}

func TestStatusFlipDuringLoadIsSerialized(t *testing.T) {
	// given: a load that blocks until released
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	gate := make(chan struct{})
	fx.backend.mu.Lock()
	fx.backend.loadGate = gate
	fx.backend.mu.Unlock()
	fx.SetWanted(true)
	waitState(t, fx.controller, StateLoading)

	// when: the space is deleted mid-load, then the load completes
	fx.backend.setStatus(spaceinfo.AccountStatusDeleted)
	fx.Poke()
	fx.backend.mu.Lock()
	fx.backend.loadGate = nil
	fx.backend.mu.Unlock()
	close(gate)
	waitState(t, fx.controller, StateOffloaded)

	// then: offload ran only after the in-flight load fully finished
	assert.Equal(t, []string{"load", "unload", "offload"}, fx.backend.callList())
}

func TestCloseDuringLoad(t *testing.T) {
	// given: a load blocked until ctx cancellation
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.backend.mu.Lock()
	fx.backend.loadGate = make(chan struct{})
	fx.backend.mu.Unlock()
	fx.SetWanted(true)
	waitState(t, fx.controller, StateLoading)

	waitErr := make(chan error, 1)
	go func() {
		_, err := fx.WaitLoaded(context.Background())
		waitErr <- err
	}()

	// when
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, fx.Close(closeCtx))

	// then
	assert.Equal(t, StateClosed, fx.State())
	assert.ErrorIs(t, <-waitErr, ErrClosed)
	// no resident space existed, so nothing was unloaded
	assert.Zero(t, fx.backend.callCount("unload"))
}

func TestCloseUnloadsResidentSpace(t *testing.T) {
	// given
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	fx.SetWanted(true)
	_, err := fx.WaitLoaded(context.Background())
	require.NoError(t, err)

	// when
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, fx.Close(closeCtx))

	// then
	assert.Equal(t, 1, fx.backend.callCount("unload"))
	assert.Equal(t, StateClosed, fx.State())
	// Close is idempotent and must not unload twice
	require.NoError(t, fx.Close(closeCtx))
	assert.Equal(t, 1, fx.backend.callCount("unload"))
}

func TestWaitLoadedHonorsCallerCtx(t *testing.T) {
	// given: a controller that will never load (no demand)
	fx := newCtrlFixture(t, spaceinfo.AccountStatusActive)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// when
	_, err := fx.WaitLoaded(ctx)

	// then
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
