package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

type testItem struct {
	Id        string
	Timestamp int
	Data      string
}

func (t *testItem) Key() string {
	return t.Id
}

func (t *testItem) Less(other ItemWithOrder) bool {
	return t.Timestamp < other.(*testItem).Timestamp
}

func makeTestItem() *testItem {
	return &testItem{}
}

func runTestQueue(t *testing.T, handlerFunc HandlerFunc[*testItem]) *Queue[*testItem] {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	return runTestQueueWithDb(t, db, handlerFunc)
}

func runTestQueueWithDb(t *testing.T, db *badger.DB, handlerFunc HandlerFunc[*testItem]) *Queue[*testItem] {
	log := logging.Logger("test")

	q := New[*testItem](db, log.Desugar(), []byte("test_queue/"), makeTestItem, handlerFunc)
	q.Run()
	t.Cleanup(func() {
		q.Close()
	})
	return q
}

func TestAdd(t *testing.T) {
	t.Run("with no error from handler", func(t *testing.T) {
		testAdd(t, nil)
	})
	t.Run("with error from handler", func(t *testing.T) {
		testAdd(t, fmt.Errorf("unknown error"))
	})
}

func testAdd(t *testing.T, errFromHandler error) {
	t.Run("add to closed queue", func(t *testing.T) {
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
			return ActionDone, errFromHandler
		})
		q.Close()

		err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
		require.Error(t, err)
	})

	t.Run("add and handle item", func(t *testing.T) {
		wantItem := &testItem{Id: "1", Timestamp: 1, Data: "data1"}
		done := make(chan struct{})
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
			assert.Equal(t, wantItem, item)
			close(done)
			return ActionDone, errFromHandler
		})

		err := q.Add(wantItem)
		require.NoError(t, err)

		select {
		case <-done:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("handler not called")
		}

		assertEventually(t, func(t *testing.T) bool {
			return !q.Has("1")
		})
	})

	t.Run("add same item multiple times", func(t *testing.T) {
		wantItem := &testItem{Id: "1", Timestamp: 1, Data: "data1"}
		done := make(chan struct{})
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
			assert.Equal(t, wantItem, item)
			time.Sleep(10 * time.Millisecond)
			close(done)
			return ActionDone, errFromHandler
		})

		for i := 0; i < 10; i++ {
			err := q.Add(wantItem)
			require.NoError(t, err)
		}

		select {
		case <-done:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("handler not called")
		}

		assertEventually(t, func(t *testing.T) bool {
			return !q.Has("1")
		})
	})

	t.Run("add and retry handling several times", func(t *testing.T) {
		var timesHandled int
		const wantTimesHandled = 3
		wantItem := &testItem{Id: "1", Timestamp: 1, Data: "data1"}
		done := make(chan struct{})
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
			assert.Equal(t, wantItem, item)
			timesHandled++
			if timesHandled < wantTimesHandled {
				return ActionRetry, nil
			}
			close(done)
			return ActionDone, errFromHandler
		})

		err := q.Add(wantItem)
		require.NoError(t, err)

		select {
		case <-done:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("handler not called")
		}

		assertEventually(t, func(t *testing.T) bool {
			return !q.Has("1")
		})

		assert.Equal(t, wantTimesHandled, timesHandled)
	})
}

func TestRestore(t *testing.T) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	q := runTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
		time.Sleep(10 * time.Millisecond)
		return ActionRetry, nil
	})

	err = q.Add(&testItem{Id: "3", Timestamp: 3, Data: "data3"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "2", Timestamp: 2, Data: "data2"})
	require.NoError(t, err)

	q.Close()

	keysQueue := mb.New[string](0)
	q = runTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
		err := keysQueue.Add(ctx, item.Key())
		require.NoError(t, err)
		return ActionDone, nil
	})

	keysQueueCtx, keysQueueCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer keysQueueCancel()

	gotKeys, err := keysQueue.NewCond().WithMin(3).Wait(keysQueueCtx)
	require.NoError(t, err)

	// Sorted by timestamp
	assert.ElementsMatch(t, []string{"1", "2", "3"}, gotKeys)
}

func TestRemove(t *testing.T) {
	t.Run("removed item should not be handled", func(t *testing.T) {
		var timesHandled int
		const timesAdded = 10
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
			timesHandled++
			time.Sleep(5 * time.Millisecond)
			return ActionDone, nil
		})

		wait := make(chan struct{})
		go func() {
			for i := 0; i < timesAdded; i++ {
				id := fmt.Sprintf("%d", i)
				err := q.Add(&testItem{Id: id, Timestamp: i, Data: "data"})
				require.NoError(t, err)
				// Remove immediately
				err = q.Remove(id)
				require.NoError(t, err)
			}
			close(wait)
		}()
		<-wait

		assertEventually(t, func(t *testing.T) bool {
			// Wait for batcher to be exhausted
			return q.batcher.Len() == 0
		})

		// Some items could be handled but definitely not all
		assert.True(t, timesHandled < timesAdded)
	})

	t.Run("remove long processing item", func(t *testing.T) {
		db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
		require.NoError(t, err)

		q := runTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
			select {
			case <-ctx.Done():
				return ActionDone, nil
			}
		})

		err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
		require.NoError(t, err)
		err = q.Add(&testItem{Id: "2", Timestamp: 2, Data: "data2"})
		require.NoError(t, err)

		assert.Equal(t, 2, q.Len())

		ok := q.Has("1")
		require.True(t, ok)

		err = q.Remove("1")
		require.NoError(t, err)

		ok = q.Has("1")
		require.False(t, ok)

		q.Close()

		t.Run("restore only one item", func(t *testing.T) {
			done := make(chan struct{})
			q = runTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
				assert.Equal(t, &testItem{Id: "2", Timestamp: 2, Data: "data2"}, item)
				close(done)
				return ActionDone, nil
			})

			select {
			case <-done:
			case <-time.After(50 * time.Millisecond):
				t.Fatal("handler not called")
			}
		})
	})
}

func assertEventually(t *testing.T, pred func(t *testing.T) bool) {
	timeout := time.NewTimer(100 * time.Millisecond)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timeout")
		case <-time.After(5 * time.Millisecond):
		}

		if pred(t) {
			return
		}
	}
}
