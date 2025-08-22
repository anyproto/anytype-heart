package temp

import (
	"fmt"
	"sync"
	"testing"
)

func TestQueue(t *testing.T) {
	q := newInmemoryQueue()

	for _, fi := range []FileInfo{
		{
			ObjectId: "f1",
			Value:    1,
		},
		{
			ObjectId: "f2",
			Value:    2,
		},
		{
			ObjectId: "f3",
			Value:    3,
		},
	} {
		q.process(fi.Key(), func(exists bool, info FileInfo) (FileInfo, error) {
			return fi, nil
		})
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, key := range []string{
				"f1", "f2", "f3",
			} {
				q.process(key, func(exists bool, info FileInfo) (FileInfo, error) {
					info.Value++
					return info, nil
				})
			}
		}()
	}

	wg.Wait()
	fmt.Println("fsdfs")
}
