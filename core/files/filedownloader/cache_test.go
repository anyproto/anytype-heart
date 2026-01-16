package filedownloader

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestCacheWarmer(t *testing.T) {
	type testMessage struct {
		spaceId string
		cid     domain.FileId
	}

	t.Run("add a task and process it", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gotCh := make(chan testMessage)
		w := newCacheWarmer(ctx, 10, 10, time.Minute, func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error {
			gotCh <- testMessage{spaceId: spaceId, cid: cid}
			return nil
		})
		go w.run()
		go w.runWorker()

		w.enqueue("space1", "file1")

		got := <-gotCh
		assert.Equal(t, testMessage{spaceId: "space1", cid: domain.FileId("file1")}, got)
	})

	t.Run("multiple tasks, multiple workers", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gotCh := make(chan testMessage)
		w := newCacheWarmer(ctx, 10, 10, time.Minute, func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error {
			gotCh <- testMessage{spaceId: spaceId, cid: cid}
			return nil
		})
		go w.run()
		for range 5 {
			go w.runWorker()
		}

		n := 10

		for i := range n {
			w.enqueue("space1", domain.FileId(fmt.Sprintf("file%d", i)))
		}

		timeout := time.After(time.Second)
		want := make([]testMessage, n)
		var got []testMessage
		for i := range n {
			want[i] = testMessage{spaceId: "space1", cid: domain.FileId(fmt.Sprintf("file%d", i))}

			select {
			case g := <-gotCh:
				got = append(got, g)
			case <-timeout:
				t.Fatal("timeout")
			}
		}

		assert.ElementsMatch(t, want, got)
	})

	t.Run("cancel a task", func(t *testing.T) {
		synctest.Run(func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			doneCh := make(chan struct{})
			w := newCacheWarmer(ctx, 10, 10, time.Minute, func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error {
				<-ctx.Done()
				close(doneCh)
				return nil
			})
			go w.run()
			go w.runWorker()

			w.enqueue("space1", "file1")

			w.cancelTask("file1")

			// It's either canceled in a downloder func, or cancelled before returning to a worker
			select {
			case <-doneCh:
			case <-time.After(time.Hour):
			}
		})
	})

	t.Run("multiple tasks, limit exceeded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		n := 10
		gotCh := make(chan testMessage)
		w := newCacheWarmer(ctx, 10, n, time.Minute, func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error {
			gotCh <- testMessage{spaceId: spaceId, cid: cid}
			return nil
		})
		go w.run()

		for i := range 100 {
			w.enqueue("space1", domain.FileId(fmt.Sprintf("file%d", i)))
		}

		for range 5 {
			go w.runWorker()
		}

		timeout := time.After(time.Second)
		want := make([]testMessage, 10)
		var got []testMessage
		for i := range n {
			want[i] = testMessage{spaceId: "space1", cid: domain.FileId(fmt.Sprintf("file%d", 90+i))}

			select {
			case g := <-gotCh:
				got = append(got, g)
			case <-timeout:
				t.Fatal("timeout")
			}
		}

		assert.ElementsMatch(t, want, got)
	})
}
