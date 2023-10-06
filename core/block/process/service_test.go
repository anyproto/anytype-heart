package process

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
)

func TestService_Add(t *testing.T) {
	var events = make(chan *pb.Event, 20)
	s := NewTest(t, func(e *pb.Event) {
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
	assert.NoError(t, s.Close(context.Background()))
}

func TestService_Cancel(t *testing.T) {
	var events = make(chan *pb.Event, 20)
	s := NewTest(t, func(e *pb.Event) {
		events <- e
	})

	err := s.Add(newTestProcess("1"))
	require.NoError(t, err)

	assert.Error(t, s.Cancel("2"))
	assert.NoError(t, s.Cancel("1"))

	assert.NoError(t, s.Close(context.Background()))
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

func NewTest(t *testing.T, broadcast func(e *pb.Event)) Service {
	s := New()
	a := new(app.App)
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Name().Return("")
	sender.EXPECT().Init(mock.Anything).Return(nil)
	if broadcast == nil {
		broadcast = func(e *pb.Event) {
			t.Log(e)
		}
	}
	sender.EXPECT().Broadcast(mock.Anything).Run(broadcast).Maybe()
	a.Register(sender).Register(s)
	a.Start(context.Background())
	return s
}
