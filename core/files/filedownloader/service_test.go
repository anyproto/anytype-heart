package filedownloader

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

type fixture struct {
	*downloader
}

func newFixture(t *testing.T) *fixture {
	s := New().(*service)
	return &fixture{
		downloader: s.newDownloader(),
	}
}

func TestManger(t *testing.T) {
	t.Run("get one task", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.stop()

		go fx.runManager()

		want := downloadTask{
			objectId: "1",
			fileId:   "fileId1",
		}
		fx.addTaskCh <- want

		task, ok := fx.getNextTask()
		require.True(t, ok)

		assert.Equal(t, want, task)
	})

	t.Run("wait for task", func(t *testing.T) {
		fx := newFixture(t)

		go fx.runManager()

		const n = 10

		var wg sync.WaitGroup
		gotTasksCh := make(chan downloadTask, n)
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				task, ok := fx.getNextTask()
				require.True(t, ok)
				gotTasksCh <- task
			}()
		}

		var wantTasks []downloadTask
		for i := range n {
			want := downloadTask{
				objectId: fmt.Sprintf("%d", i),
				fileId:   domain.FileId(fmt.Sprintf("fileId%d", i)),
			}
			fx.addTaskCh <- want
			wantTasks = append(wantTasks, want)
		}

		wg.Wait()

		var gotTasks []downloadTask
		for range n {
			got := <-gotTasksCh
			gotTasks = append(gotTasks, got)
		}

		assert.ElementsMatch(t, wantTasks, gotTasks)
	})
}
