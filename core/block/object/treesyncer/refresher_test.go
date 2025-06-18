package treesyncer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRefresher(t *testing.T) {
	action := func(ctx context.Context) string {
		return "test"
	}

	r := newRefresher(action)

	require.NotNil(t, r.action, "action should not be nil")
	require.NotNil(t, r.ctx, "ctx should not be nil")
	require.NotNil(t, r.cancel, "cancel should not be nil")
	require.False(t, r.running, "running should be false initially")
	require.False(t, r.closed, "closed should be false initially")
	require.Empty(t, r.onRefreshes, "onRefreshes should be empty initially")
}

func TestDoAfterBasic(t *testing.T) {
	var actionCalled bool
	var callbackCalled bool
	var result string

	action := func(ctx context.Context) string {
		actionCalled = true
		return "hello"
	}

	callback := func(s string) {
		callbackCalled = true
		result = s
	}

	r := newRefresher(action)
	defer r.Close()

	r.doAfter(callback)

	r.wg.Wait()

	require.True(t, actionCalled, "action should have been called")
	require.True(t, callbackCalled, "callback should have been called")
	require.Equal(t, "hello", result, "expected result 'hello'")
}

func TestDoAfterMultipleCallbacks(t *testing.T) {
	var actionCallCount int32
	var callback1Called, callback2Called bool
	var result1, result2 string

	action := func(ctx context.Context) string {
		atomic.AddInt32(&actionCallCount, 1)
		return "test"
	}

	callback1 := func(s string) {
		callback1Called = true
		result1 = s
	}

	callback2 := func(s string) {
		callback2Called = true
		result2 = s
	}

	r := newRefresher(action)
	r.onRefreshes = append(r.onRefreshes, callback1)
	defer r.Close()
	r.doAfter(callback2)

	r.wg.Wait()

	require.Equal(t, int32(1), atomic.LoadInt32(&actionCallCount), "action should have been called exactly once")
	require.True(t, callback1Called, "callback1 should have been called")
	require.True(t, callback2Called, "callback2 should have been called")
	require.Equal(t, "test", result1, "expected result1 'test'")
	require.Equal(t, "test", result2, "expected result2 'test'")
}

func TestDoAfterIsRunning(t *testing.T) {
	// this test checks that multiple calls to doAfter while the action is running
	// do not cause some of the callbacks to be skipped
	var actionCallCount int32
	callbackCount := int32(0)
	isStarted := atomic.Bool{}

	callbackStarted := make(chan struct{})

	actionStarted := make(chan struct{})
	actionCanComplete := make(chan struct{})

	action := func(ctx context.Context) int {
		atomic.AddInt32(&actionCallCount, 1)
		if isStarted.Load() {
			close(actionStarted)
			<-actionCanComplete
		}
		return 42
	}

	callback := func(result int) {
		atomic.AddInt32(&callbackCount, 1)
		if isStarted.Load() {
			return
		}
		isStarted.Store(true)
		close(callbackStarted)
	}

	r := newRefresher(action)
	defer r.Close()

	r.doAfter(callback)
	// first action is completed
	<-callbackStarted

	r.doAfter(callback)
	// second action is started but not completed yet
	<-actionStarted
	r.doAfter(callback)
	close(actionCanComplete)

	// wait until all callbacks are called
	r.wg.Wait()

	require.Equal(t, int32(2), atomic.LoadInt32(&actionCallCount), "action should have been called twice")
	require.Equal(t, int32(3), atomic.LoadInt32(&callbackCount), "expected 3 callbacks")
}

func TestDoAfterSequentialCalls(t *testing.T) {
	var actionCallCount int32
	var callbackCallCount int32

	action := func(ctx context.Context) string {
		atomic.AddInt32(&actionCallCount, 1)
		return "result"
	}

	callback := func(s string) {
		atomic.AddInt32(&callbackCallCount, 1)
	}

	r := newRefresher(action)
	defer r.Close()

	r.doAfter(callback)
	r.wg.Wait()

	r.doAfter(callback)
	r.wg.Wait()

	require.Equal(t, int32(2), atomic.LoadInt32(&actionCallCount), "action should have been called twice")
	require.Equal(t, int32(2), atomic.LoadInt32(&callbackCallCount), "callbacks should have been called twice")
}

func TestIsRunning(t *testing.T) {
	actionStarted := make(chan struct{})
	actionCanComplete := make(chan struct{})

	action := func(ctx context.Context) bool {
		close(actionStarted)
		<-actionCanComplete
		return true
	}

	r := newRefresher(action)
	defer r.Close()

	require.False(t, r.IsRunning(), "should not be running initially")

	r.doAfter(func(bool) {})

	<-actionStarted

	require.True(t, r.IsRunning(), "should be running after doAfter called")

	close(actionCanComplete)
	r.wg.Wait()

	require.False(t, r.IsRunning(), "should not be running after completion")
}

func TestClose(t *testing.T) {
	actionStarted := make(chan struct{})

	action := func(ctx context.Context) string {
		close(actionStarted)
		<-ctx.Done()
		return "cancelled"
	}

	r := newRefresher(action)

	var callbackCalled bool
	callback := func(s string) {
		callbackCalled = true
	}

	r.doAfter(callback)

	<-actionStarted
	r.Close()
	require.False(t, callbackCalled, "callback should not have been called after close")
}

func TestDoAfterOnClosed(t *testing.T) {
	action := func(ctx context.Context) string {
		return "test"
	}

	r := newRefresher(action)
	r.Close()

	var callbackCalled bool
	callback := func(s string) {
		callbackCalled = true
	}

	r.doAfter(callback)
	time.Sleep(10 * time.Millisecond)

	require.False(t, callbackCalled, "callback should not have been called on closed refresher")
	require.False(t, r.IsRunning(), "should not be running after close")
}

func TestMultipleClose(t *testing.T) {
	action := func(ctx context.Context) string {
		return "test"
	}

	r := newRefresher(action)

	r.Close()
	r.Close()
	r.Close()
}

func TestConcurrentOperations(t *testing.T) {
	var actionCallCount int32
	var callbackCallCount int32

	action := func(ctx context.Context) int {
		return int(atomic.AddInt32(&actionCallCount, 1))
	}

	callback := func(result int) {
		atomic.AddInt32(&callbackCallCount, 1)
	}

	r := newRefresher(action)
	defer r.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.doAfter(callback)
		}()
	}

	wg.Wait()
	r.wg.Wait()

	require.Equal(t, int32(10), atomic.LoadInt32(&callbackCallCount), "expected 10 callback calls")

	actionCalls := atomic.LoadInt32(&actionCallCount)
	require.GreaterOrEqual(t, actionCalls, int32(1), "action should be called at least once")
	require.LessOrEqual(t, actionCalls, int32(10), "action should not be called more than 10 times")
}

func TestContextCancellation(t *testing.T) {
	actionCalled := make(chan struct{})
	contextCancelledReceived := make(chan struct{})

	action := func(ctx context.Context) string {
		close(actionCalled)
		select {
		case <-ctx.Done():
			close(contextCancelledReceived)
			return "cancelled"
		case <-time.After(100 * time.Millisecond):
			return "timeout"
		}
	}

	r := newRefresher(action)

	r.doAfter(func(s string) {})

	<-actionCalled

	r.Close()

	select {
	case <-contextCancelledReceived:
	case <-time.After(50 * time.Millisecond):
		require.Fail(t, "context should have been cancelled")
	}
}

func TestGenericTypes(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		r := newRefresher(func(ctx context.Context) string { return "test" })
		defer r.Close()

		var result string
		r.doAfter(func(s string) { result = s })
		r.wg.Wait()

		require.Equal(t, "test", result, "expected 'test'")
	})

	t.Run("int", func(t *testing.T) {
		r := newRefresher(func(ctx context.Context) int { return 42 })
		defer r.Close()

		var result int
		r.doAfter(func(i int) { result = i })
		r.wg.Wait()

		require.Equal(t, 42, result, "expected 42")
	})

	t.Run("struct", func(t *testing.T) {
		type testStruct struct {
			Name string
			ID   int
		}

		expected := testStruct{Name: "test", ID: 123}
		r := newRefresher(func(ctx context.Context) testStruct { return expected })
		defer r.Close()

		var result testStruct
		r.doAfter(func(ts testStruct) { result = ts })
		r.wg.Wait()

		require.Equal(t, expected, result, "expected struct values to match")
	})
}
