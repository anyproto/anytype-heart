package recordsbatcher

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
)

const CName = "recordsbatcher"

type recordsBatcher struct {
	batcher   *mb.MB[any]
	packDelay time.Duration // delay for better packing of msgs
}

func (r *recordsBatcher) Init(a *app.App) (err error) {
	r.batcher = mb.New[any](0)
	r.packDelay = time.Millisecond * 100
	return nil
}

func (r *recordsBatcher) Name() (name string) {
	return CName
}

func (r *recordsBatcher) Add(msgs ...any) error {
	return r.batcher.Add(context.Background(), msgs...)
}

func (r *recordsBatcher) Read(buffer []interface{}) int {
	defer func() {
		time.Sleep(r.packDelay)
	}()

	msgs, err := r.batcher.NewCond().WithMax(len(buffer)).Wait(context.Background())
	if err != nil {
		return 0
	}
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
	return &recordsBatcher{batcher: mb.New[any](0)}
}

type RecordsBatcher interface {
	// Read reads a batch into the buffer, returns number of records that were read. 0 means no more data will be available
	Read(buffer []interface{}) int
	Add(msgs ...interface{}) error
	app.Component
}
