package process

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Add(t *testing.T) {
	var events = make(chan *pb.Event, 20)
	s := NewTest(func(e *pb.Event) {
		events <- e
	})

	err := s.Add(newTestProcess("1"))
	require.NoError(t, err)
	err = s.Add(newTestProcess("2"))
	require.NoError(t, err)

	assert.Error(t, s.Add(newTestProcess("1")))

	for i := 0; i < 2; i++ {
		select {
		case <-events:
		case <-time.After(time.Second):
			t.Error("timeout")
		}
	}
	assert.NoError(t, s.Close())
}

func TestService_Cancel(t *testing.T) {
	var events = make(chan *pb.Event, 20)
	s := NewTest(func(e *pb.Event) {
		events <- e
	})

	err := s.Add(newTestProcess("1"))
	require.NoError(t, err)

	assert.Error(t, s.Cancel("2"))
	assert.NoError(t, s.Cancel("1"))

	assert.NoError(t, s.Close())
	for i := 0; i < 2; i++ {
		select {
		case <-events:
		case <-time.After(time.Second):
			t.Error("timeout")
		}
	}
}

func newTestProcess(id string) *testProcess {
	return &testProcess{
		id:   id,
		done: make(chan struct{}),
	}
}

type testProcess struct {
	id   string
	done chan struct{}
}

func (t *testProcess) Id() string {
	return t.id
}

func (t *testProcess) Cancel() (err error) {
	close(t.done)
	return
}

func (t testProcess) Info() pb.ModelProcess {
	return pb.ModelProcess{
		Id: t.id,
	}
}

func (t *testProcess) Done() chan struct{} {
	return t.done
}

func NewTest(se func(e *pb.Event)) Service {
	s := New()
	a := new(app.App)
	a.Register(&testapp.EventSender{
		F: se,
	}).Register(s)
	a.Start()
	return s
}
