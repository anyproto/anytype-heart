package persistentqueue

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/cheggaaa/mb/v3"
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

func (t *testItem) Less(other OrderedItem) bool {
	return t.Timestamp < other.(*testItem).Timestamp
}

func makeTestItem() *testItem {
	return &testItem{}
}

func newAnystore(t *testing.T) anystore.DB {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := anystore.Open(context.Background(), path, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func runTestQueue(t *testing.T, handlerFunc HandlerFunc[*testItem]) *Queue[*testItem] {
	db := newAnystore(t)
	q := newTestQueueWithDb(t, db, handlerFunc)
	q.Run()
	t.Cleanup(func() {
		q.Close()
	})
	return q
}

func newTestQueueWithDb(t *testing.T, db anystore.DB, handlerFunc HandlerFunc[*testItem]) *Queue[*testItem] {
	log := logging.Logger("test")

	storage, err := NewAnystoreStorage[*testItem](db, "test_queue", makeTestItem)
	require.NoError(t, err)

	q := New[*testItem](storage, log.Desugar(), handlerFunc, nil)
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

	t.Run("add to not started queue, then start", func(t *testing.T) {
		db := newAnystore(t)

		q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
			return ActionDone, errFromHandler
		})

		const numItems = 10
		var wantKeys []string
		for i := 0; i < numItems; i++ {
			key := fmt.Sprintf("%d", i)
			wantKeys = append(wantKeys, key)
			err := q.Add(&testItem{Id: key, Timestamp: i, Data: "data"})
			require.NoError(t, err)
		}

		assert.ElementsMatch(t, wantKeys, q.ListKeys())
		assert.Equal(t, numItems, q.Len())
		q.Run()

		assertEventually(t, func(t *testing.T) bool {
			return q.Len() == 0
		})
		assertEventually(t, func(t *testing.T) bool {
			return q.NumProcessedItems() == numItems
		})
		assert.Empty(t, q.ListKeys())

		err := q.Close()
		require.NoError(t, err)
	})

	t.Run("add to not started queue, close, then create new queue and start", func(t *testing.T) {
		db := newAnystore(t)

		const numItems = 10

		t.Run("with the first instance of queue, add to not started queue", func(t *testing.T) {
			q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
				return ActionDone, errFromHandler
			})

			for i := 0; i < numItems; i++ {
				err := q.Add(&testItem{Id: fmt.Sprintf("%d", i), Timestamp: i, Data: "data"})
				require.NoError(t, err)
			}
			assert.Equal(t, numItems, q.Len())

			err := q.Close()
			require.NoError(t, err)
		})

		t.Run("with the second instance of queue, run queue and handle previously added items", func(t *testing.T) {
			var numItemsHandled int64
			q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
				atomic.AddInt64(&numItemsHandled, 1)
				return ActionDone, nil
			})

			q.Run()

			assertEventually(t, func(t *testing.T) bool {
				return q.Len() == 0
			})
			assertEventually(t, func(t *testing.T) bool {
				return q.NumProcessedItems() == numItems && numItemsHandled == numItems
			})

			err := q.Close()
			require.NoError(t, err)
		})
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
			time.Sleep(20 * time.Millisecond)
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
	db := newAnystore(t)

	q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
		time.Sleep(10 * time.Millisecond)
		return ActionRetry, nil
	})
	q.Run()

	err := q.Add(&testItem{Id: "3", Timestamp: 3, Data: "data3"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "2", Timestamp: 2, Data: "data2"})
	require.NoError(t, err)

	err = q.Close()
	require.NoError(t, err)

	keysQueue := mb.New[string](0)
	q = newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
		err := keysQueue.Add(ctx, item.Key())
		require.NoError(t, err)
		return ActionDone, nil
	})
	q.Run()

	keysQueueCtx, keysQueueCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer keysQueueCancel()

	gotKeys, err := keysQueue.NewCond().WithMin(3).Wait(keysQueueCtx)
	require.NoError(t, err)

	// Sorted by timestamp
	assert.ElementsMatch(t, []string{"1", "2", "3"}, gotKeys)

	err = q.Close()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}

func TestRemove(t *testing.T) {
	t.Run("removed item should not be handled", func(t *testing.T) {
		const timesAdded = 10
		q := runTestQueue(t, func(ctx context.Context, item *testItem) (Action, error) {
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
			return q.Len() == 0
		})

		q.Close()

		// Some items could be handled but definitely not all
		assert.True(t, q.NumProcessedItems() < timesAdded)
	})

	t.Run("remove long processing item", func(t *testing.T) {
		db := newAnystore(t)
		q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
			select {
			case <-ctx.Done():
				return ActionDone, nil
			}
		})

		q.Run()

		err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
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
			q = newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
				assert.Equal(t, &testItem{Id: "2", Timestamp: 2, Data: "data2"}, item)
				close(done)
				return ActionDone, nil
			})
			q.Run()

			select {
			case <-done:
			case <-time.After(50 * time.Millisecond):
				t.Fatal("handler not called")
			}

			q.Close()
		})

		db.Close()
	})
}

func TestWithHandlerTickPeriod(t *testing.T) {
	t.Run("pause on ActionRetry", func(t *testing.T) {
		db := newAnystore(t)
		log := logging.Logger("test")
		storage, err := NewAnystoreStorage[*testItem](db, "test_queue", makeTestItem)
		require.NoError(t, err)

		tickerPeriod := 50 * time.Millisecond
		q := New[*testItem](storage, log.Desugar(), func(ctx context.Context, item *testItem) (Action, error) {
			return ActionRetry, nil
		}, nil, WithRetryPause(tickerPeriod))

		err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
		require.NoError(t, err)
		err = q.Add(&testItem{Id: "2", Timestamp: 2, Data: "data2"})
		require.NoError(t, err)

		q.Run()

		time.Sleep(tickerPeriod / 2)
		assert.Equal(t, 1, q.NumProcessedItems())

		time.Sleep(tickerPeriod)
		assert.Equal(t, 2, q.NumProcessedItems())
	})

	t.Run("do not pause on ActionDone", func(t *testing.T) {
		db := newAnystore(t)
		log := logging.Logger("test")

		storage, err := NewAnystoreStorage[*testItem](db, "test_queue", makeTestItem)
		require.NoError(t, err)

		tickerPeriod := 50 * time.Millisecond
		q := New[*testItem](storage, log.Desugar(), func(ctx context.Context, item *testItem) (Action, error) {
			return ActionDone, nil
		}, nil, WithRetryPause(tickerPeriod))

		err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
		require.NoError(t, err)
		err = q.Add(&testItem{Id: "2", Timestamp: 2, Data: "data2"})
		require.NoError(t, err)

		q.Run()

		time.Sleep(tickerPeriod / 2)
		assert.Equal(t, 2, q.NumProcessedItems())

		time.Sleep(tickerPeriod)
		assert.Equal(t, 2, q.NumProcessedItems())
	})
}

type testContextKeyType string

const testContextKey testContextKeyType = "testKey"

func TestWithContext(t *testing.T) {
	db := newAnystore(t)
	log := logging.Logger("test")
	testRootCtx := context.WithValue(context.Background(), testContextKey, "testValue")

	wait := make(chan struct{})
	storage, err := NewAnystoreStorage[*testItem](db, "test_queue", makeTestItem)
	require.NoError(t, err)

	q := New[*testItem](storage, log.Desugar(), func(ctx context.Context, item *testItem) (Action, error) {
		val, ok := ctx.Value(testContextKey).(string)
		assert.True(t, ok)
		assert.Equal(t, "testValue", val)
		close(wait)
		return ActionDone, nil
	}, nil, WithContext(testRootCtx))
	q.Run()
	t.Cleanup(func() {
		q.Close()
		db.Close()
	})

	err = q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)

	<-wait
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

func TestHandleNext(t *testing.T) {
	t.Run("Remove: item is deleted after processing has started", func(t *testing.T) {
		testCase := func(t *testing.T, action Action) {
			t.Run(fmt.Sprintf("action: %s", action), func(t *testing.T) {
				var q *Queue[*testItem]
				db := newAnystore(t)
				q = newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
					err := q.Remove(item.Key())
					require.NoError(t, err)
					return action, nil
				})

				err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
				require.NoError(t, err)

				err = q.handleNext()
				require.NoError(t, err)

				assert.False(t, q.Has("1"))
			})
		}

		testCase(t, ActionDone)
		testCase(t, ActionRetry)
	})

	t.Run("RemoveWait", func(t *testing.T) {
		t.Run("processing of item has not yet started", func(t *testing.T) {
			testCase := func(t *testing.T, action Action) {
				t.Run(fmt.Sprintf("action: %s", action), func(t *testing.T) {
					var q *Queue[*testItem]
					db := newAnystore(t)

					q = newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
						return action, nil
					})

					err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
					require.NoError(t, err)

					waitCh, err := q.RemoveWait("1")
					require.NoError(t, err)

					// Wait channel should be closed
					<-waitCh

					err = q.handleNext()
					require.ErrorIs(t, err, errRemoved)

					assert.False(t, q.Has("1"))
				})
			}

			testCase(t, ActionDone)
			testCase(t, ActionRetry)
		})
		t.Run("processing has started", func(t *testing.T) {
			testCase := func(t *testing.T, action Action) {
				t.Run(fmt.Sprintf("action: %s", action), func(t *testing.T) {
					var q *Queue[*testItem]
					db := newAnystore(t)

					var waitCh chan struct{}
					q = newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
						var err error
						waitCh, err = q.RemoveWait(item.Key())
						require.NoError(t, err)
						return action, nil
					})

					err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
					require.NoError(t, err)

					err = q.handleNext()
					require.NoError(t, err)

					// Wait channel should be closed
					<-waitCh
					assert.False(t, q.Has("1"))

					// Remove again, expect that wait channel is closed immediately
					waitCh, err = q.RemoveWait("1")
					require.NoError(t, err)
					<-waitCh
				})
			}

			testCase(t, ActionDone)
			testCase(t, ActionRetry)
		})
	})

}

func TestRemoveBy(t *testing.T) {
	db := newAnystore(t)
	processed := make(chan *testItem)
	q := newTestQueueWithDb(t, db, func(ctx context.Context, item *testItem) (Action, error) {
		select {
		case processed <- item:
			return ActionDone, nil
		case <-ctx.Done():
			return ActionDone, nil
		}
	})

	err := q.Add(&testItem{Id: "1", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "1/a", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)
	err = q.Add(&testItem{Id: "2", Timestamp: 1, Data: "data1"})
	require.NoError(t, err)

	err = q.RemoveBy(func(key string) bool {
		return strings.HasPrefix(key, "1")
	})
	require.NoError(t, err)

	q.Run()

	select {
	case got := <-processed:
		assert.Equal(t, "2", got.Id)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}
}

func TestMultipleWorkers(t *testing.T) {
	db := newAnystore(t)
	log := logging.Logger("test")

	storage, err := NewAnystoreStorage[*testItem](db, "test_queue", makeTestItem)
	require.NoError(t, err)

	handledItemsCh := make(chan *testItem)
	q := New[*testItem](storage, log.Desugar(), func(ctx context.Context, item *testItem) (Action, error) {
		handledItemsCh <- item
		return ActionDone, nil
	}, nil, WithWorkersNumber(10))
	q.Run()

	itemsCount := 20
	for i := range itemsCount {
		err = q.Add(&testItem{Id: fmt.Sprintf("%d", i)})
		require.NoError(t, err)
	}

	timer := time.NewTimer(50 * time.Millisecond)
	handled := make([]*testItem, 0, itemsCount)
	for {
		select {
		case <-timer.C:
			t.Fatal("timeout")
		case it := <-handledItemsCh:
			handled = append(handled, it)
		}

		if len(handled) == itemsCount {
			break
		}
	}

	wantIds := map[string]struct{}{}
	for i := range itemsCount {
		wantIds[fmt.Sprintf("%d", i)] = struct{}{}
	}

	gotIds := map[string]struct{}{}
	for _, it := range handled {
		gotIds[it.Id] = struct{}{}
	}

	assert.Equal(t, wantIds, gotIds)

	err = q.Close()
	require.NoError(t, err)
}
