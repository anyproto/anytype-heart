package recordsbatcher

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/cheggaaa/mb"
	"sync"
	"time"
)

const CName = "recordsbatcher"

var log = logging.Logger("anytype-recordsbatcher")

type recordsBatcher struct {
	batcher   *mb.MB
	packDelay time.Duration // delay for better packing of msgs
	m         sync.Mutex
}

func (r *recordsBatcher) Init(a *app.App) (err error) {
	log.Errorf("recordsBatcher %p init", r)
	r.batcher = mb.New(0)
	r.packDelay = time.Millisecond * 100
	return nil
}

func (r *recordsBatcher) Name() (name string) {
	return CName
}

func (r *recordsBatcher) Add(msgs ...core.SmartblockRecordWithThreadID) error {
	var msgsIfaces []interface{}
	for _, msg := range msgs {
		msgsIfaces = append(msgsIfaces, interface{}(msg))
	}

	return r.batcher.Add(msgsIfaces...)
}

func (r *recordsBatcher) Read(buffer []core.SmartblockRecordWithThreadID) int {
	r.m.Lock()
	defer func() {
		time.Sleep(r.packDelay)
		r.m.Unlock()
	}()

	msgs := r.batcher.WaitMax(len(buffer))
	if len(msgs) == 0 {
		return 0
	}
	var msgsCasted []core.SmartblockRecordWithThreadID
	for _, msg := range msgs {
		msgsCasted = append(msgsCasted[0:], msg.(core.SmartblockRecordWithThreadID))
	}

	return copy(buffer, msgsCasted)
}

func (r *recordsBatcher) Close() (err error) {
	log.Errorf("recordsBatcher %p close", r)

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
