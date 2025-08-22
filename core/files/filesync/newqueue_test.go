package filesync

import (
	"sync"
	"testing"
)

func TestNewQueue(t *testing.T) {
	q := newStateProcessor()

	q.files = map[string]FileInfo{
		"f1": {
			ObjectId:      "f1",
			BytesToUpload: 0,
		},
	}

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {

		}()
	}
}
