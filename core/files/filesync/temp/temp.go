package temp

import (
	"fmt"
	"sync"
)

type FileInfo struct {
	ObjectId string
	State    string
	Value    int
}

func (i FileInfo) Key() string {
	return i.ObjectId
}

type inmemoryQueue struct {
	lock  sync.Mutex
	files map[string]FileInfo

	processing map[string]*sync.Mutex
}

func newInmemoryQueue() *inmemoryQueue {
	return &inmemoryQueue{
		files:      make(map[string]FileInfo),
		processing: make(map[string]*sync.Mutex),
	}
}

func (q *inmemoryQueue) isProcessing(key string) bool {
	_, ok := q.processing[key]
	return ok
}

func (q *inmemoryQueue) process(key string, proc func(exists bool, info FileInfo) (FileInfo, error)) {
	q.lock.Lock()
	procLock, ok := q.processing[key]
	if !ok {
		procLock = &sync.Mutex{}
		q.processing[key] = procLock
		procLock.Lock()
	}
	q.lock.Unlock()

	if ok {
		procLock.Lock()
	}
	defer procLock.Unlock()

	q.lock.Lock()
	fi, exists := q.files[key]
	q.lock.Unlock()

	// Critical section

	next, err := proc(exists, fi)
	if err != nil {
		fmt.Println("ERROR", err)
	}

	q.lock.Lock()
	q.files[key] = next
	delete(q.processing, key)
	q.lock.Unlock()
}
