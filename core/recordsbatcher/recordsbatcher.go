package recordsbatcher

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/cheggaaa/mb"
	"sync"
	"time"
)

const CName = "recordsbatcher"

type recordsBatcher struct {
	batcher   *mb.MB
	packDelay time.Duration // delay for better packing of msgs
	m         sync.Mutex
}

func (r *recordsBatcher) Init(a *app.App) (err error) {
	r.batcher = mb.New(0)
	r.packDelay = time.Millisecond * 100
	return nil
}

func (r *recordsBatcher) Name() (name string) {
	return CName
}

func (r *recordsBatcher) Add(msgs ...core.SmartblockRecordWithThreadID) error {
	return r.batcher.Add(msgs)
}

func (r *recordsBatcher) Read(buffer []core.SmartblockRecordWithThreadID) int {
	r.m.Lock()
	defer func() {
		time.Sleep(r.packDelay)
		r.m.Unlock()
	}()
	msgs := r.batcher.Wait()
	if len(msgs) == 0 {
		return 0
	}
	var total int
	for _, msg := range msgs {
		buffer = append(buffer, msg.(core.SmartblockRecordWithThreadID))
		total++
	}
	return total
}

func (r *recordsBatcher) Close() (err error) {
	return r.batcher.Close()
}

func New() RecordsBatcher {
	return &recordsBatcher{batcher: mb.New(0)}
}

type RecordsBatcher interface {
	// Read reads a batch into the buffer, returns number of records that were read. 0 means no more data will be available
	Read(buffer []core.SmartblockRecordWithThreadID) int
	Add(msgs ...core.SmartblockRecordWithThreadID) error
	app.Component
}
