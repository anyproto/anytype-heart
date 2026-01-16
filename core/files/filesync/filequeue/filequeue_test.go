package filequeue

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertToQueue(t *testing.T, q *fixture, it fileInfo) {
	err := q.Upsert(it.ObjectId, func(exists bool, prev fileInfo) fileInfo {
		return it
	})
	require.NoError(t, err)
}

type fixture struct {
	db anystore.DB
	*Queue[fileInfo]
}

func (fx *fixture) close() {
	fx.Queue.Close()
	fx.db.Close()
}

func newTestQueue(t *testing.T) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "store.db"), nil)
	require.NoError(t, err)

	coll, err := db.Collection(ctx, "queue")
	require.NoError(t, err)

	store := NewStorage[fileInfo](coll, marshalFileInfo, unmarshalFileInfo)
	q := NewQueue(store, func(info fileInfo) string {
		return info.ObjectId
	}, func(info fileInfo, id string) fileInfo {
		info.ObjectId = id
		return info
	})

	go func() {
		q.Run()
	}()

	return &fixture{
		db:    db,
		Queue: q,
	}
}

func TestGetById(t *testing.T) {
	t.Run("lock and unlock with update", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()

			// Lock the object
			_, err := q.GetById("obj1")
			require.ErrorIs(t, err, ErrNotFound)

			want := fileInfo{
				ObjectId: "obj1",
				State:    fileStatePendingDeletion,
			}

			go func() {
				time.Sleep(time.Second)
				err := q.ReleaseAndUpdate("obj1", want)
				require.NoError(t, err)
			}()

			got, err := q.GetById("obj1")
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	})

	t.Run("lock and unlock without update", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()

			// Lock the object
			_, err := q.GetById("obj1")
			require.ErrorIs(t, err, ErrNotFound)

			go func() {
				time.Sleep(time.Second)
				err := q.Release("obj1")
				require.NoError(t, err)
			}()

			_, err = q.GetById("obj1")
			require.ErrorIs(t, err, ErrNotFound)
		})
	})
}

func TestQueue(t *testing.T) {
	q := newTestQueue(t)
	defer q.close()

	insertToQueue(t, q, fileInfo{
		ObjectId: "obj1",
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := q.GetById("obj1")
			require.NoError(t, err)
			task.BytesToUpload++
			err = q.ReleaseAndUpdate(task.ObjectId, task)
			require.NoError(t, err)
		}()
	}

	wg.Wait()
	want := fileInfo{
		ObjectId:      "obj1",
		BytesToUpload: 100,
	}
	got, err := q.GetById("obj1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestQueueGetNext(t *testing.T) {
	t.Run("basic get next", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})

			next, err := q.GetNext(ctx, getNextRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("basic get next: no item, subscription disabled", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			req := getNextRequestUploading()
			req.Subscribe = false
			_, err := q.GetNext(ctx, req)
			require.ErrorIs(t, err, ErrNoRows)
		})
	})

	t.Run("wait for item", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			go func() {
				time.Sleep(10 * time.Minute)
				err := q.Upsert("obj1", func(exists bool, it fileInfo) fileInfo {
					return fileInfo{
						ObjectId:    "obj1",
						State:       fileStateUploading,
						ScheduledAt: time.Now().Add(time.Minute),
					}
				})
				require.NoError(t, err)
			}()

			next, err := q.GetNext(ctx, getNextRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("get next in parallel", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			const n = 100

			for i := range n {
				insertToQueue(t, q, fileInfo{
					ObjectId: fmt.Sprintf("obj%d", i),
					State:    fileStateUploading,
				})
			}

			resultsCh := make(chan string, n)
			for range n {
				go func() {
					next, err := q.GetNext(ctx, getNextRequestUploading())
					require.NoError(t, err)
					resultsCh <- next.ObjectId
				}()
			}

			var got []string
			for range n {
				got = append(got, <-resultsCh)
			}

			want := make([]string, n)
			for i := range want {
				want[i] = fmt.Sprintf("obj%d", i)
			}
			assert.ElementsMatch(t, want, got)
		})
	})

	t.Run("get next one by one", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			const n = 100

			for i := range n {
				insertToQueue(t, q, fileInfo{
					ObjectId: fmt.Sprintf("obj%d", i),
					State:    fileStateUploading,
				})
			}

			got := make([]string, 0, n)
			for range n {
				next, err := q.GetNext(ctx, getNextRequestUploading())
				require.NoError(t, err)

				next.State = fileStatePendingDeletion
				got = append(got, next.ObjectId)
				err = q.ReleaseAndUpdate(next.ObjectId, next)
				require.NoError(t, err)
			}

			want := make([]string, n)
			for i := range want {
				want[i] = fmt.Sprintf("obj%d", i)
			}
			assert.ElementsMatch(t, want, got)
		})
	})

	t.Run("cancel get next", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx, cancel := context.WithCancel(context.Background())

			go func() {
				time.Sleep(10 * time.Second)
				cancel()
			}()
			_, err := q.GetNext(ctx, getNextRequestUploading())
			require.Error(t, err, context.Canceled)
		})
	})
}

func TestQueueSchedule(t *testing.T) {
	t.Run("basic schedule", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("basic schedule: no items, no subscription", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			req := getNextScheduledRequestUploading()
			req.Subscribe = false
			_, err := q.GetNextScheduled(ctx, req)
			require.ErrorIs(t, err, ErrNoRows)
		})
	})

	t.Run("wait for suitable item", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			go func() {
				time.Sleep(10 * time.Minute)
				insertToQueue(t, q, fileInfo{
					ObjectId:    "obj1",
					State:       fileStateUploading,
					ScheduledAt: time.Now().Add(time.Minute),
				})
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("skip locked", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj2",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(10 * time.Minute),
			})

			// Lock obj1
			_, err := q.GetById("obj1")
			require.NoError(t, err)

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})

	t.Run("schedule in parallel", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			const n = 100

			for i := range n {
				insertToQueue(t, q, fileInfo{
					ObjectId:    fmt.Sprintf("obj%d", i),
					State:       fileStateUploading,
					ScheduledAt: time.Now().Add(time.Duration(i+1) * time.Minute),
				})
			}

			resultsCh := make(chan string, n)
			for range n {
				go func() {
					next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
					require.NoError(t, err)
					resultsCh <- next.ObjectId
				}()
			}

			var got []string
			for range n {
				got = append(got, <-resultsCh)
			}

			want := make([]string, n)
			for i := range want {
				want[i] = fmt.Sprintf("obj%d", i)
			}
			assert.Equal(t, want, got)
		})
	})

	t.Run("the second object became scheduled for earlier", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj2",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			go func() {
				time.Sleep(500 * time.Millisecond)

				insertToQueue(t, q, fileInfo{
					ObjectId:    "obj2",
					State:       fileStateUploading,
					ScheduledAt: time.Now().Add(time.Millisecond),
				})
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})

	t.Run("re-schedule when changed in mid time", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj2",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			// Object1 was locked for 2 minutes and was changed, so it was no longer satisfied a filter.
			// Object2 then should be scheduled next
			go func() {
				_, err := q.GetById("obj1")
				require.NoError(t, err)
				time.Sleep(2 * time.Minute)
				err = q.ReleaseAndUpdate("obj1", fileInfo{
					ObjectId: "obj1",
					State:    fileStateDeleted,
				})
				require.NoError(t, err)
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})

	t.Run("cancel get next scheduled", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx, cancel := context.WithCancel(context.Background())

			go func() {
				time.Sleep(10 * time.Second)
				cancel()
			}()

			_, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.Error(t, err, context.Canceled)
		})
	})

	t.Run("cancel scheduled", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx, cancel := context.WithCancel(context.Background())

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			go func() {
				time.Sleep(10 * time.Second)
				cancel()
			}()

			_, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.Error(t, err, context.Canceled)
		})
	})

	t.Run("wait for an item to be released, release the item without update", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})

			// Lock the object
			_, err := q.GetById("obj1")
			require.NoError(t, err)

			go func() {
				time.Sleep(10 * time.Minute)
				// Release without update
				err = q.Release("obj1")
				require.NoError(t, err)
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})
}

func TestComplex(t *testing.T) {
	t.Run("get next but item is scheduled", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			go func() {
				time.Sleep(1 * time.Minute)
				next, err := q.GetNext(ctx, getNextRequestUploading())
				require.NoError(t, err)
				next.Imported = true
				assert.Equal(t, "obj1", next.ObjectId)
				err = q.ReleaseAndUpdate(next.ObjectId, next)
				require.NoError(t, err)
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj1", next.ObjectId)
			assert.True(t, next.Imported)
		})
	})

	t.Run("get next, change item, schedule next", func(t *testing.T) {
		synctest.Run(func() {
			q := newTestQueue(t)
			defer q.close()
			ctx := context.Background()

			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj1",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})
			insertToQueue(t, q, fileInfo{
				ObjectId:    "obj2",
				State:       fileStateUploading,
				ScheduledAt: time.Now().Add(2 * time.Hour),
			})

			go func() {
				time.Sleep(1 * time.Minute)
				next, err := q.GetNext(ctx, getNextRequestUploading())
				require.NoError(t, err)
				next.State = fileStatePendingDeletion
				assert.Equal(t, "obj1", next.ObjectId)
				err = q.ReleaseAndUpdate(next.ObjectId, next)
				require.NoError(t, err)
			}()

			next, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
			require.NoError(t, err)
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})
}

func TestClose(t *testing.T) {
	synctest.Run(func() {
		q := newTestQueue(t)
		ctx := context.Background()
		var wg sync.WaitGroup

		sendRequests := func() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := q.GetNext(ctx, getNextRequestUploading())
				assert.Error(t, ErrClosed, err)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := q.GetNextScheduled(ctx, getNextScheduledRequestUploading())
				assert.Error(t, ErrClosed, err)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := q.GetById("obj10")
				assert.Error(t, ErrClosed, err)
			}()
		}

		sendRequests()
		q.close()
		wg.Wait()

		// Send to closed queue
		sendRequests()
		wg.Wait()
	})
}

func getNextRequestUploading() GetNextRequest[fileInfo] {
	return GetNextRequest[fileInfo]{
		StoreFilter: query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(fileStateUploading)),
		},
		StoreOrder: nil,
		Filter: func(info fileInfo) bool {
			return info.State == fileStateUploading
		},
		Subscribe: true,
	}
}

func getNextScheduledRequestUploading() GetNextScheduledRequest[fileInfo] {
	return GetNextScheduledRequest[fileInfo]{
		StoreFilter: query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(fileStateUploading)),
		},
		StoreOrder: &query.SortField{
			Field:   "scheduledAt",
			Path:    []string{"scheduledAt"},
			Reverse: false,
		},
		Filter: func(info fileInfo) bool {
			return info.State == fileStateUploading
		},
		ScheduledAt: func(info fileInfo) time.Time {
			return info.ScheduledAt
		},
		Subscribe: true,
	}
}
