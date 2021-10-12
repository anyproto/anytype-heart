package recordsbatcher

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/cheggaaa/mb"
)

const CName = "recordsbatcher"

var log = logging.Logger("anytype-recordsbatcher")

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

func (r *recordsBatcher) Add(msgs ...interface{}) error {
	return r.batcher.Add(msgs...)
}

func (r *recordsBatcher) Read(buffer []interface{}) int {
	defer func() {
		time.Sleep(r.packDelay)
	}()

	msgs := r.batcher.WaitMax(len(buffer))
	if len(msgs) == 0 {
		return 0
	}
	for i, msg := range msgs {
		buffer[i] = msg
	}

	return len(msgs)
}

func (r *recordsBatcher) Close() (err error) {
	return r.batcher.Close()
}

func New() RecordsBatcher {
	return &recordsBatcher{batcher: mb.New(0)}
}

type RecordsBatcher interface {
	// Read reads a batch into the buffer, returns number of records that were read. 0 means no more data will be available
	Read(buffer []interface{}) int
	Add(msgs ...interface{}) error
	app.Component
}
