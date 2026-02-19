package dedupqueue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDedupQueue(t *testing.T) {
	dq := New(10)
	require.NotNil(t, dq)
	assert.NotNil(t, dq.ctx)
	assert.NotNil(t, dq.cancel)
	assert.NotNil(t, dq.batch)
	assert.NotNil(t, dq.entries)
	assert.Equal(t, uint64(0), dq.cnt.Load())
	
	err := dq.Close()
	assert.NoError(t, err)
}

func TestDedupQueue_Replace_SingleCall(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount atomic.Int32
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		close(done)
	})
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 1)
	assert.Contains(t, dq.entries, "test1")
	dq.mx.Unlock()
	
	dq.Run()
	
	<-done
	
	assert.Equal(t, int32(1), callCount.Load())
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_Replace_MultipleCalls(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount atomic.Int32
	var lastValue atomic.Int32
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		lastValue.Store(1)
	})
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		lastValue.Store(2)
	})
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		lastValue.Store(3)
		close(done)
	})
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 1)
	assert.Contains(t, dq.entries, "test1")
	dq.mx.Unlock()
	
	dq.Run()
	
	<-done
	
	assert.Equal(t, int32(1), callCount.Load())
	assert.Equal(t, int32(3), lastValue.Load())
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_Replace_DifferentIds(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount1 atomic.Int32
	var callCount2 atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	
	dq.Replace("test1", func() {
		callCount1.Add(1)
		wg.Done()
	})
	
	dq.Replace("test2", func() {
		callCount2.Add(1)
		wg.Done()
	})
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 2)
	assert.Contains(t, dq.entries, "test1")
	assert.Contains(t, dq.entries, "test2")
	dq.mx.Unlock()
	
	dq.Run()
	
	go func() {
		wg.Wait()
		close(done)
	}()
	
	<-done
	
	assert.Equal(t, int32(1), callCount1.Load())
	assert.Equal(t, int32(1), callCount2.Load())
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_Replace_NilCall(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	
	dq.Replace("test1", nil)
	dq.Replace("test2", func() {
		close(done)
	})
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 2)
	assert.Contains(t, dq.entries, "test1")
	dq.mx.Unlock()
	
	dq.Run()
	
	<-done
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_Replace_FullQueue(t *testing.T) {
	dq := New(1)
	defer dq.Close()
	
	var callCount1 atomic.Int32
	var callCount2 atomic.Int32
	
	dq.Replace("test1", func() {
		callCount1.Add(1)
	})
	
	dq.Replace("test2", func() {
		callCount2.Add(1)
	})
	
	dq.mx.Lock()
	entriesLen := len(dq.entries)
	dq.mx.Unlock()
	
	assert.Equal(t, 1, entriesLen)
	assert.Equal(t, int32(0), callCount1.Load())
	assert.Equal(t, int32(0), callCount2.Load())
}

func TestDedupQueue_Run_MultipleRuns(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount atomic.Int32
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		close(done)
	})
	
	dq.Run()
	dq.Run()
	dq.Run()
	
	<-done
	
	assert.Equal(t, int32(1), callCount.Load())
}

func TestDedupQueue_ConcurrentReplace(t *testing.T) {
	dq := New(100)
	defer dq.Close()
	
	dq.Run()
	
	var wg sync.WaitGroup
	var totalCalls atomic.Int32
	numGoroutines := 10
	numReplacesPerGoroutine := 100
	done := make(chan struct{})
	
	wg.Add(numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numReplacesPerGoroutine; j++ {
				key := "test"
				dq.Replace(key, func() {
					totalCalls.Add(1)
				})
				time.Sleep(100 * time.Microsecond)
			}
		}(i)
	}
	
	go func() {
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
		close(done)
	}()
	
	<-done
	
	calls := totalCalls.Load()
	assert.Greater(t, calls, int32(0))
	assert.LessOrEqual(t, calls, int32(numGoroutines*numReplacesPerGoroutine))
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_ConcurrentReplaceDifferentIds(t *testing.T) {
	dq := New(100)
	defer dq.Close()
	
	dq.Run()
	
	var wg sync.WaitGroup
	var totalCalls atomic.Int32
	numGoroutines := 5
	numReplacesPerGoroutine := 5
	done := make(chan struct{})
	
	wg.Add(numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numReplacesPerGoroutine; j++ {
				key := "test" + string(rune('a'+id))
				dq.Replace(key, func() {
					totalCalls.Add(1)
				})
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}
	
	go func() {
		wg.Wait()
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()
	
	<-done
	
	calls := totalCalls.Load()
	assert.Greater(t, calls, int32(0))
	assert.LessOrEqual(t, calls, int32(numGoroutines*numReplacesPerGoroutine))
	
	dq.mx.Lock()
	assert.Len(t, dq.entries, 0)
	dq.mx.Unlock()
}

func TestDedupQueue_Close(t *testing.T) {
	dq := New(10)
	
	var callCount atomic.Int32
	
	dq.Replace("test1", func() {
		callCount.Add(1)
	})
	
	dq.Run()
	
	err := dq.Close()
	assert.NoError(t, err)
	
	assert.Equal(t, int32(0), callCount.Load())
}

func TestDedupQueue_CloseMultipleTimes(t *testing.T) {
	dq := New(10)
	
	err1 := dq.Close()
	assert.NoError(t, err1)
	
	err2 := dq.Close()
	assert.Error(t, err2)
}

func TestDedupQueue_CounterIncrement(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	initialCnt := dq.cnt.Load()
	
	dq.Replace("test1", func() {})
	assert.Equal(t, initialCnt+1, dq.cnt.Load())
	
	dq.Replace("test2", func() {})
	assert.Equal(t, initialCnt+2, dq.cnt.Load())
	
	dq.Replace("test1", func() {})
	assert.Equal(t, initialCnt+3, dq.cnt.Load())
}

func TestDedupQueue_EntryCounter(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	dq.Replace("test1", func() {})
	
	dq.mx.Lock()
	entry1 := dq.entries["test1"]
	cnt1 := entry1.cnt
	dq.mx.Unlock()
	
	dq.Replace("test1", func() {})
	
	dq.mx.Lock()
	entry2 := dq.entries["test1"]
	cnt2 := entry2.cnt
	dq.mx.Unlock()
	
	assert.Greater(t, cnt2, cnt1)
}

func TestDedupQueue_CallLoopExecution(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	executed := make(chan string, 10)
	
	dq.Replace("test1", func() {
		executed <- "test1"
	})
	
	dq.Replace("test2", func() {
		executed <- "test2"
	})
	
	dq.Run()
	
	var results []string
	for i := 0; i < 2; i++ {
		select {
		case result := <-executed:
			results = append(results, result)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for execution")
		}
	}
	
	assert.Len(t, results, 2)
	assert.Contains(t, results, "test1")
	assert.Contains(t, results, "test2")
}

func TestDedupQueue_ReplaceDuringExecution(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	var executionOrder []int
	var mu sync.Mutex
	started := make(chan struct{})
	finished := make(chan struct{})
	
	dq.Replace("test1", func() {
		mu.Lock()
		executionOrder = append(executionOrder, 1)
		mu.Unlock()
		close(started)
		time.Sleep(20 * time.Millisecond)
	})
	
	dq.Run()
	
	<-started
	
	dq.Replace("test1", func() {
		mu.Lock()
		executionOrder = append(executionOrder, 2)
		mu.Unlock()
		close(finished)
	})
	
	<-finished
	
	mu.Lock()
	assert.Equal(t, []int{1, 2}, executionOrder)
	mu.Unlock()
}

func TestDedupQueue_EmptyId(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var called atomic.Bool
	
	dq.Replace("", func() {
		called.Store(true)
		close(done)
	})
	
	dq.Run()
	
	<-done
	
	assert.True(t, called.Load())
}

func TestDedupQueue_LongRunningCall(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	var wg sync.WaitGroup
	wg.Add(2)
	
	var call1Started atomic.Bool
	var call1Done atomic.Bool
	var call2Done atomic.Bool
	call1StartedChan := make(chan struct{})
	
	dq.Replace("test1", func() {
		call1Started.Store(true)
		close(call1StartedChan)
		time.Sleep(20 * time.Millisecond)
		call1Done.Store(true)
		wg.Done()
	})
	
	dq.Replace("test2", func() {
		<-call1StartedChan
		call2Done.Store(true)
		wg.Done()
	})
	
	dq.Run()
	
	wg.Wait()
	
	assert.True(t, call1Started.Load())
	assert.True(t, call1Done.Load())
	assert.True(t, call2Done.Load())
}

func TestDedupQueue_PanicInCall(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var call2Done atomic.Bool
	
	dq.Replace("test1", func() {
		defer func() {
			recover()
		}()
		panic("test panic")
	})
	
	dq.Replace("test2", func() {
		call2Done.Store(true)
		close(done)
	})
	
	dq.Run()
	
	<-done
	
	assert.True(t, call2Done.Load())
}

func TestDedupQueue_ZeroMaxSize(t *testing.T) {
	dq := New(0)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount atomic.Int32
	
	dq.Replace("test1", func() {
		callCount.Add(1)
		close(done)
	})
	
	dq.Run()
	
	select {
	case <-done:
		assert.Equal(t, int32(1), callCount.Load())
	case <-time.After(50 * time.Millisecond):
		assert.Equal(t, int32(0), callCount.Load())
	}
}

func TestDedupQueue_ReplaceAfterClose(t *testing.T) {
	dq := New(10)
	
	var callCount atomic.Int32
	
	dq.Close()
	
	dq.Replace("test1", func() {
		callCount.Add(1)
	})
	
	assert.Equal(t, int32(0), callCount.Load())
}

func TestDedupQueue_ReplaceWithSameFunction(t *testing.T) {
	dq := New(10)
	defer dq.Close()
	
	done := make(chan struct{})
	var callCount atomic.Int32
	fn := func() {
		callCount.Add(1)
		select {
		case <-done:
		default:
			close(done)
		}
	}
	
	dq.Replace("test1", fn)
	dq.Replace("test1", fn)
	dq.Replace("test1", fn)
	
	dq.Run()
	
	<-done
	
	assert.Equal(t, int32(1), callCount.Load())
}

func TestDedupQueue_ManyUniqueIds(t *testing.T) {
	dq := New(1000)
	defer dq.Close()
	
	var totalCalls atomic.Int32
	numIds := 100
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numIds)
	
	for i := range numIds {
		id := "test" + string(rune(i))
		dq.Replace(id, func() {
			totalCalls.Add(1)
			wg.Done()
		})
	}
	
	dq.Run()
	
	go func() {
		wg.Wait()
		close(done)
	}()
	
	<-done
	
	assert.Equal(t, int32(numIds), totalCalls.Load())
}