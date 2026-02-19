package dedupqueue

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/cheggaaa/mb/v3"
)

type DedupQueue struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cnt     atomic.Uint64
	entries map[string]entry
	batch   *mb.MB[string]
	mx      sync.Mutex
}

type entry struct {
	call func()
	cnt  uint64
}

func New(maxSize int) *DedupQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &DedupQueue{
		ctx:     ctx,
		cancel:  cancel,
		batch:   mb.New[string](maxSize),
		entries: map[string]entry{},
	}
}

func (dq *DedupQueue) Replace(id string, call func()) {
	curCnt := dq.cnt.Load()
	dq.cnt.Add(1)

	dq.mx.Lock()
	if _, ok := dq.entries[id]; ok {
		dq.entries[id] = entry{call: call, cnt: curCnt}
		dq.mx.Unlock()
		return
	}
	ent := entry{call: call, cnt: curCnt}
	dq.entries[id] = ent
	dq.mx.Unlock()

	if err := dq.batch.TryAdd(id); err != nil {
		dq.mx.Lock()
		if cur, ok := dq.entries[id]; ok && cur.cnt == curCnt {
			delete(dq.entries, id)
		}
		dq.mx.Unlock()
	}
}

func (dq *DedupQueue) Run() {
	go dq.callLoop()
}

func (dq *DedupQueue) callLoop() {
	for {
		id, err := dq.batch.WaitOne(dq.ctx)
		if err != nil {
			return
		}

		dq.mx.Lock()
		curEntry := dq.entries[id]
		delete(dq.entries, id)
		dq.mx.Unlock()

		if curEntry.call != nil {
			curEntry.call()
		}
	}
}

func (dq *DedupQueue) Close() error {
	dq.cancel()
	return dq.batch.Close()
}
