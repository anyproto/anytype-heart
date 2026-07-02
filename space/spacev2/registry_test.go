package spacev2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type fakeController struct {
	spaceId string
	mu      sync.Mutex
	closed  bool
}

func (f *fakeController) SpaceId() string                 { return f.spaceId }
func (f *fakeController) Start(ctx context.Context) error { return nil }
func (f *fakeController) Mode() Mode                      { return ModeLoading }
func (f *fakeController) WaitLoad(ctx context.Context) (clientspace.Space, error) {
	return nil, nil
}
func (f *fakeController) Update() error { return nil }
func (f *fakeController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}
func (f *fakeController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return nil
}
func (f *fakeController) GetStatus() spaceinfo.AccountStatus    { return spaceinfo.AccountStatusActive }
func (f *fakeController) GetLocalStatus() spaceinfo.LocalStatus { return spaceinfo.LocalStatusOk }
func (f *fakeController) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeController) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func TestRegistry_EnsureThenAwait(t *testing.T) {
	// given
	r := newRegistry()
	want := &fakeController{spaceId: "space1"}

	// when
	got, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then
	require.NoError(t, err)
	assert.Same(t, want, got)
	awaited, err := r.await(context.Background(), "space1")
	require.NoError(t, err)
	assert.Same(t, want, awaited)
}

func TestRegistry_AwaitBeforeEnsure(t *testing.T) {
	// given: a waiter arrives before any build (unidirectional Wait semantics)
	r := newRegistry()
	want := &fakeController{spaceId: "space1"}
	done := make(chan struct{})
	var got SpaceController
	var gotErr error
	go func() {
		got, gotErr = r.await(context.Background(), "space1")
		close(done)
	}()

	// when: the watcher builds later
	time.Sleep(10 * time.Millisecond)
	_, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then
	require.NoError(t, err)
	<-done
	require.NoError(t, gotErr)
	assert.Same(t, want, got)
}

func TestRegistry_FailedBuildIsRetryable(t *testing.T) {
	// given: first build fails (v1 would poison the waiting map for the session — §9.8)
	r := newRegistry()
	buildErr := errors.New("boom")
	_, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return nil, buildErr
	})
	require.ErrorIs(t, err, buildErr)

	// await between failure and retry returns the failure
	_, err = r.await(context.Background(), "space1")
	require.ErrorIs(t, err, buildErr)

	// when: the next watcher event retries
	want := &fakeController{spaceId: "space1"}
	got, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then: the retry wins and later awaits see the controller
	require.NoError(t, err)
	assert.Same(t, want, got)
	awaited, err := r.await(context.Background(), "space1")
	require.NoError(t, err)
	assert.Same(t, want, awaited)
}

func TestRegistry_GetSemantics(t *testing.T) {
	r := newRegistry()

	// unknown id: not-exists, does not block
	_, err := r.get(context.Background(), "nope")
	require.ErrorIs(t, err, ErrSpaceNotExists)

	// placeholder created by a waiter still reads as not-exists for get()
	waiterCtx, waiterCancel := context.WithCancel(context.Background())
	defer waiterCancel()
	go func() { _, _ = r.await(waiterCtx, "waited") }()
	require.Eventually(t, func() bool {
		_, err := r.get(context.Background(), "waited")
		return errors.Is(err, ErrSpaceNotExists)
	}, time.Second, 5*time.Millisecond)

	// ready entry returns the controller
	want := &fakeController{spaceId: "ready"}
	_, err = r.ensure(context.Background(), "ready", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})
	require.NoError(t, err)
	got, err := r.get(context.Background(), "ready")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestRegistry_StaticEntry(t *testing.T) {
	r := newRegistry()
	want := &fakeController{spaceId: "marketplace"}
	r.addStatic("marketplace", want)

	got, err := r.get(context.Background(), "marketplace")
	require.NoError(t, err)
	assert.Same(t, want, got)
	assert.Equal(t, []string{"marketplace"}, r.allIds())
}

func TestRegistry_CloseAll(t *testing.T) {
	r := newRegistry()
	c1 := &fakeController{spaceId: "s1"}
	_, err := r.ensure(context.Background(), "s1", func(ctx context.Context) (SpaceController, error) {
		return c1, nil
	})
	require.NoError(t, err)

	require.NoError(t, r.closeAll(context.Background()))
	assert.True(t, c1.isClosed())

	// post-close: ensure and await refuse
	_, err = r.ensure(context.Background(), "s2", func(ctx context.Context) (SpaceController, error) {
		return &fakeController{spaceId: "s2"}, nil
	})
	require.ErrorIs(t, err, ErrSpaceIsClosing)
	_, err = r.await(context.Background(), "s2")
	require.ErrorIs(t, err, ErrSpaceIsClosing)
}

func TestRegistry_CloseAllWakesBlockedWaiters(t *testing.T) {
	// given: a waiter blocked on a space that never gets built
	r := newRegistry()
	done := make(chan error, 1)
	go func() {
		_, err := r.await(context.Background(), "never")
		done <- err
	}()
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.entries) == 1
	}, time.Second, 5*time.Millisecond)

	// when
	require.NoError(t, r.closeAll(context.Background()))

	// then: the waiter is released with ErrSpaceIsClosing
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrSpaceIsClosing)
	case <-time.After(time.Second):
		t.Fatal("waiter not released by closeAll")
	}
}
