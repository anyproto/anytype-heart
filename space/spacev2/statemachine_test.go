package spacev2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingProcess records Start/Close ordering across all processes of a
// factory, so tests can assert close-old-before-start-new (§9.4).
type recordingProcess struct {
	mode     Mode
	rec      *recorder
	startErr error
}

type recorder struct {
	mu     sync.Mutex
	events []string // "start:<mode>" / "close:<mode>"
}

func (r *recorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.events...)
}

func (p *recordingProcess) Start(ctx context.Context) error {
	if p.startErr != nil {
		return p.startErr
	}
	p.rec.add("start:" + modeName(p.mode))
	return nil
}

func (p *recordingProcess) Close(ctx context.Context) error {
	p.rec.add("close:" + modeName(p.mode))
	return nil
}

func modeName(m Mode) string {
	switch m {
	case ModeInitial:
		return "initial"
	case ModeLoading:
		return "loading"
	case ModeOffloading:
		return "offloading"
	case ModeJoining:
		return "joining"
	default:
		return "unknown"
	}
}

type recordingFactory struct {
	mu        sync.Mutex
	rec       *recorder
	failModes map[Mode]error
	calls     map[Mode]int
}

func newRecordingFactory() *recordingFactory {
	return &recordingFactory{
		rec:       &recorder{},
		failModes: map[Mode]error{},
		calls:     map[Mode]int{},
	}
}

func (f *recordingFactory) Process(md Mode) smProcess {
	f.mu.Lock()
	f.calls[md]++
	failErr := f.failModes[md]
	f.mu.Unlock()
	return &recordingProcess{mode: md, rec: f.rec, startErr: failErr}
}

func (f *recordingFactory) callCount(md Mode) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[md]
}

func newTestStateMachine(t *testing.T, factory processFactory) *stateMachine {
	sm, err := newStateMachine(factory, log)
	require.NoError(t, err)
	t.Cleanup(sm.Close)
	return sm
}

func TestStateMachine_InitialStarted(t *testing.T) {
	// given / when
	f := newRecordingFactory()
	sm := newTestStateMachine(t, f)

	// then
	assert.Equal(t, ModeInitial, sm.GetMode())
	assert.Equal(t, []string{"start:initial"}, f.rec.list())
}

func TestStateMachine_SameModeNoOp(t *testing.T) {
	// given
	f := newRecordingFactory()
	sm := newTestStateMachine(t, f)
	proc1, err := sm.ChangeMode(ModeLoading)
	require.NoError(t, err)

	// when: same target again
	proc2, err := sm.ChangeMode(ModeLoading)

	// then: no rebuild, the same live process is returned
	require.NoError(t, err)
	assert.Same(t, proc1, proc2)
	assert.Equal(t, 1, f.callCount(ModeLoading))
}

func TestStateMachine_CloseOldBeforeStartNew(t *testing.T) {
	// given
	f := newRecordingFactory()
	sm := newTestStateMachine(t, f)

	// when
	_, err := sm.ChangeMode(ModeLoading)
	require.NoError(t, err)
	_, err = sm.ChangeMode(ModeOffloading)
	require.NoError(t, err)

	// then: strict close-then-start sequence (§9.4 — the only thing
	// serializing offload vs load on storage)
	want := []string{"start:initial", "close:initial", "start:loading", "close:loading", "start:offloading"}
	assert.Equal(t, want, f.rec.list())
	assert.Equal(t, ModeOffloading, sm.GetMode())
}

func TestStateMachine_ConcurrentSameTargetPiggyback(t *testing.T) {
	// given
	f := newRecordingFactory()
	sm := newTestStateMachine(t, f)

	// when: many concurrent callers ask for the same transition
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = sm.ChangeMode(ModeLoading)
		}(i)
	}
	wg.Wait()

	// then: all succeed, exactly one loading process was built
	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, f.callCount(ModeLoading))
}

func TestStateMachine_DivergentTargetRejected(t *testing.T) {
	// given: a transition blocked in Start
	f := newRecordingFactory()
	blocker := make(chan struct{})
	blockingFactory := &funcFactory{fn: func(md Mode) smProcess {
		if md == ModeLoading {
			return &funcProcess{start: func(ctx context.Context) error {
				<-blocker
				return nil
			}}
		}
		return f.Process(md)
	}}
	sm := newTestStateMachine(t, blockingFactory)
	go func() { _, _ = sm.ChangeMode(ModeLoading) }()
	require.Eventually(t, func() bool {
		sm.Lock()
		defer sm.Unlock()
		return sm.next == ModeLoading
	}, time.Second, time.Millisecond)

	// when: a different target while the first is in flight
	_, err := sm.ChangeMode(ModeOffloading)

	// then
	require.ErrorIs(t, err, ErrTransitionInProcess)
	close(blocker)
}

func TestStateMachine_StartFailureFallsBackWithCause(t *testing.T) {
	// given
	f := newRecordingFactory()
	cause := errors.New("pipeline exploded")
	f.failModes[ModeLoading] = cause
	sm := newTestStateMachine(t, f)

	// when
	_, err := sm.ChangeMode(ModeLoading)

	// then: the waiter learns the CAUSE (v1 lost it: nil → bare ErrFailedToStart)
	require.ErrorIs(t, err, ErrFailedToStart)
	require.ErrorIs(t, err, cause)
	assert.Equal(t, ModeInitial, sm.GetMode())

	// and: the machine is not wedged — clearing the failure lets a retry through
	f.mu.Lock()
	delete(f.failModes, ModeLoading)
	f.mu.Unlock()
	_, err = sm.ChangeMode(ModeLoading)
	require.NoError(t, err)
	assert.Equal(t, ModeLoading, sm.GetMode())
}

func TestStateMachine_FreshProcessPerTransition(t *testing.T) {
	// given
	f := newRecordingFactory()
	sm := newTestStateMachine(t, f)

	// when: Loading → Offloading → Loading (CancelLeave shape, §11.3)
	_, err := sm.ChangeMode(ModeLoading)
	require.NoError(t, err)
	_, err = sm.ChangeMode(ModeOffloading)
	require.NoError(t, err)
	_, err = sm.ChangeMode(ModeLoading)
	require.NoError(t, err)

	// then: every entry into Loading minted a fresh process (§9.5)
	assert.Equal(t, 2, f.callCount(ModeLoading))
	assert.Equal(t, ModeLoading, sm.GetMode())
}

type funcFactory struct {
	fn func(md Mode) smProcess
}

func (f *funcFactory) Process(md Mode) smProcess { return f.fn(md) }

type funcProcess struct {
	start func(ctx context.Context) error
}

func (p *funcProcess) Start(ctx context.Context) error {
	if p.start != nil {
		return p.start(ctx)
	}
	return nil
}

func (p *funcProcess) Close(ctx context.Context) error { return nil }
