package filesync

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatcher(t *testing.T) {
	t.Run("add file with large blocks", func(t *testing.T) {
		maxBatchSize := 100
		queue := make(chan blockPushManyRequest, 100)
		b := newRequestsBatcher(maxBatchSize, 10*time.Millisecond, queue)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go b.run(ctx)

		wantRequests := 5
		bs := make([]blocks.Block, 5)
		for i := range bs {
			bs[i] = generateBlock(t, 100)
		}
		err := b.addFile("space1", "file1", "object1", bs)
		require.NoError(t, err)

		timeout := time.NewTimer(10 * time.Millisecond)
		var gotRequests int
		for range wantRequests {
			select {
			case <-queue:
				gotRequests++
			case <-timeout.C:
				t.Fatal("timeout")
			}
		}
	})

	t.Run("add file with small blocks, enqueue batch in background", func(t *testing.T) {
		maxBatchSize := 100
		queue := make(chan blockPushManyRequest, 100)
		b := newRequestsBatcher(maxBatchSize, 10*time.Millisecond, queue)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go b.run(ctx)

		wantRequests := 1
		bs := make([]blocks.Block, 5)
		for i := range bs {
			bs[i] = generateBlock(t, 10)
		}
		err := b.addFile("space1", "file1", "object1", bs)
		require.NoError(t, err)

		timeout := time.NewTimer(20 * time.Millisecond)
		var gotRequests int
		for range wantRequests {
			select {
			case req := <-queue:
				gotRequests++
				_ = req
			case <-timeout.C:
				t.Fatal("timeout")
			}
		}
	})

	t.Run("add multiple files, enqueue batches in background", func(t *testing.T) {
		for i, tc := range []struct {
			files           int
			blocksPerFile   int
			bytesPerBlock   int
			wantRequestsNum int
		}{
			{
				files:           10,
				blocksPerFile:   3,
				bytesPerBlock:   21,
				wantRequestsNum: 8,
			},
			{
				files:           10,
				blocksPerFile:   3,
				bytesPerBlock:   20,
				wantRequestsNum: 6,
			},
			{
				files:           10,
				blocksPerFile:   5,
				bytesPerBlock:   100,
				wantRequestsNum: 50,
			},
		} {
			t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
				maxBatchSize := 100
				queue := make(chan blockPushManyRequest, 100)
				b := newRequestsBatcher(maxBatchSize, 1*time.Millisecond, queue)

				var wg sync.WaitGroup
				for i := range tc.files {
					wg.Add(1)
					go func() {
						defer wg.Done()

						bs := make([]blocks.Block, tc.blocksPerFile)
						for i := range bs {
							bs[i] = generateBlock(t, tc.bytesPerBlock)
						}
						err := b.addFile("space1", fmt.Sprintf("file%02d", i), "object1", bs)
						require.NoError(t, err)
					}()
				}
				wg.Wait()

				time.Sleep(5 * time.Millisecond)
				b.tick()

				gotRequests := waitRequests(queue)

				assert.Equal(t, tc.wantRequestsNum, len(gotRequests))
			})
		}
	})
}

func waitRequests(queue chan blockPushManyRequest) []*fileproto.BlockPushManyRequest {
	timeout := time.NewTimer(30 * time.Millisecond)
	var got []*fileproto.BlockPushManyRequest
	for {
		select {
		case req := <-queue:
			got = append(got, req.req)
		case <-timeout.C:
			return got
		}
	}
}

func generateBlock(t *testing.T, size int) blocks.Block {
	b := make([]byte, size)
	n, err := rand.Read(b)
	require.NoError(t, err)
	require.Equal(t, n, size)
	return blocks.NewBlock(b)
}
