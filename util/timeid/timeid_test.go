package timeid

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestAtomicCounterSequential checks that repeated calls in a single goroutine produce strictly increasing values.
func TestNewNanoSequential(t *testing.T) {
	const calls = 10
	fmt.Println(time.Now().Unix())
	fmt.Println(time.Now().UnixMilli())
	fmt.Println(time.Now().UnixNano())

	prev := NewNano()
	for i := 1; i < calls; i++ {
		val := NewNano()
		fmt.Println(val)
		if val <= prev {
			t.Fatalf("Value did not strictly increase. got=%d, prev=%d", val, prev)
		}
		prev = val
	}
}

// TestAtomicCounterConcurrent checks that calls from multiple goroutines produce unique, strictly increasing values overall.
func TestNewNanoConcurrent(t *testing.T) {
	const (
		goroutines = 10
		callsPerG  = 100
		totalCalls = goroutines * callsPerG
	)

	var wg sync.WaitGroup
	results := make([]int64, totalCalls)

	// We'll start goroutines, each generating callsPerG IDs.
	// Each goroutine fills its portion of the `results` slice.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			offset := gid * callsPerG
			for i := 0; i < callsPerG; i++ {
				results[offset+i] = NewNano()
			}
		}(g)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Now we sort all the IDs and ensure they are strictly increasing.
	sort.Slice(results, func(i, j int) bool {
		return results[i] < results[j]
	})

	for i := 1; i < totalCalls; i++ {
		if results[i] <= results[i-1] {
			t.Fatalf("Duplicate or non-increasing value at index %d: %d <= %d",
				i, results[i], results[i-1])
		}
	}
}

// (Optional) TestAtomicCounterTimeDrift checks behavior if we sleep between calls
// to ensure the timestamp portion also changes. This is mostly to illustrate timing
// but not strictly necessary for correctness.
func TestNewNanoTimeDrift(t *testing.T) {
	// Get one value
	first := NewNano()
	// Sleep to ensure a new millisecond passes
	time.Sleep(time.Millisecond)
	second := NewNano()
	// second should definitely be greater
	if second <= first {
		t.Fatalf("Value did not increase across millisecond boundary. first=%d, second=%d", first, second)
	}
}
