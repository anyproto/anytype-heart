package temp

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	store := &storage{files: make(map[string]FileInfo)}
	q := newQueue(store)

	go func() {
		q.run()
	}()

	q.release(FileInfo{
		ObjectId: "obj1",
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := q.get("obj1")
			task.BytesToUpload++
			q.release(task)
		}()
	}

	wg.Wait()
	want := FileInfo{
		ObjectId:      "obj1",
		BytesToUpload: 100,
	}
	got := q.get("obj1")
	assert.Equal(t, want, got)
}

func TestQueueSchedule(t *testing.T) {
	t.Run("basic schedule", func(t *testing.T) {
		synctest.Run(func() {
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			q.release(FileInfo{
				ObjectId:    "obj1",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})

			next := q.getNext(func(info FileInfo) bool {
				return info.State == FileStateUploading
			}, func(info FileInfo) time.Time {
				return info.ScheduledAt
			})
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("wait for suitable item", func(t *testing.T) {
		synctest.Run(func() {
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			go func() {
				time.Sleep(10 * time.Minute)
				q.release(FileInfo{
					ObjectId:    "obj1",
					State:       FileStateUploading,
					ScheduledAt: time.Now().Add(time.Minute),
				})
			}()

			next := q.getNext(func(info FileInfo) bool {
				return info.State == FileStateUploading
			}, func(info FileInfo) time.Time {
				return info.ScheduledAt
			})
			assert.Equal(t, "obj1", next.ObjectId)
		})
	})

	t.Run("skip locked", func(t *testing.T) {
		synctest.Run(func() {
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			q.release(FileInfo{
				ObjectId:    "obj1",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			q.release(FileInfo{
				ObjectId:    "obj2",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(10 * time.Minute),
			})

			// Lock obj1
			q.get("obj1")

			next := q.getNext(func(info FileInfo) bool {
				return info.State == FileStateUploading
			}, func(info FileInfo) time.Time {
				return info.ScheduledAt
			})
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})

	t.Run("schedule in parallel", func(t *testing.T) {
		synctest.Run(func() {
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			const n = 100

			for i := range n {
				q.release(FileInfo{
					ObjectId:    fmt.Sprintf("obj%d", i),
					State:       FileStateUploading,
					ScheduledAt: time.Now().Add(time.Duration(i+1) * time.Minute),
				})
			}

			resultsCh := make(chan string, n)
			for range n {
				go func() {
					next := q.getNext(func(info FileInfo) bool {
						return info.State == FileStateUploading
					}, func(info FileInfo) time.Time {
						return info.ScheduledAt
					})
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
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			q.release(FileInfo{
				ObjectId:    "obj1",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			q.release(FileInfo{
				ObjectId:    "obj2",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			go func() {
				time.Sleep(500 * time.Millisecond)

				q.release(FileInfo{
					ObjectId:    "obj2",
					State:       FileStateUploading,
					ScheduledAt: time.Now().Add(time.Millisecond),
				})
			}()

			next := q.getNext(func(info FileInfo) bool {
				return info.State == FileStateUploading
			}, func(info FileInfo) time.Time {
				return info.ScheduledAt
			})
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})

	t.Run("re-schedule when changed in mid time", func(t *testing.T) {
		synctest.Run(func() {
			store := &storage{files: make(map[string]FileInfo)}
			q := newQueue(store)

			go func() {
				q.run()
			}()
			defer q.close()

			q.release(FileInfo{
				ObjectId:    "obj1",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Minute),
			})
			q.release(FileInfo{
				ObjectId:    "obj2",
				State:       FileStateUploading,
				ScheduledAt: time.Now().Add(time.Hour),
			})

			// Object1 was locked for 2 minutes and was changed, so it was no longer satisfied a filter.
			// Object2 then should be scheduled next
			go func() {
				q.get("obj1")
				time.Sleep(2 * time.Minute)
				q.release(FileInfo{
					ObjectId: "obj1",
					State:    FileStateDeleted,
				})
			}()

			next := q.getNext(func(info FileInfo) bool {
				return info.State == FileStateUploading
			}, func(info FileInfo) time.Time {
				return info.ScheduledAt
			})
			assert.Equal(t, "obj2", next.ObjectId)
		})
	})
}
