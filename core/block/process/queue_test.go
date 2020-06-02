package process

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
)

func TestService_NewQueue(t *testing.T) {
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 0)
	assert.NotEmpty(t, q.Id())
	assert.NotEmpty(t, q.Info())
}

func TestQueue_Start(t *testing.T) {
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 5)
	assert.NoError(t, q.Start())
	assert.Error(t, q.Start()) // error for second start
	assert.NoError(t, q.Finalize())
}

func TestQueue_Add(t *testing.T) {
	var a, b int32
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 5)
	incrA := func() {
		atomic.AddInt32(&a, 1)
	}
	incrB := func() {
		atomic.AddInt32(&b, 1)
	}
	assert.NoError(t, q.Add(incrA, incrB))
	assert.NoError(t, q.Start())
	assert.NoError(t, q.Add(incrA))
	assert.NoError(t, q.Add(incrB))
	info := q.Info()
	assert.Equal(t, int64(4), info.Progress.Total)
	assert.Equal(t, pb.ModelProcess_Running, info.State)
	assert.NoError(t, q.Finalize())
	assert.Equal(t, int32(2), a)
	assert.Equal(t, int32(2), b)
	info = q.Info()
	assert.Equal(t, int64(4), info.Progress.Done)
	assert.Equal(t, int64(4), info.Progress.Total)
	assert.Equal(t, pb.ModelProcess_Done, info.State)
	assert.Error(t, q.Add(func() {}))
}

func TestQueue_Wait(t *testing.T) {
	var a, b int32
	var aCh = make(chan struct{})
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 5)
	incrA := func() {
		atomic.AddInt32(&a, 1)
	}
	incrB := func() {
		atomic.AddInt32(&b, 1)
	}
	q.Add(incrB, incrB)
	go func() {
		assert.NoError(t, q.Wait(incrA, incrA))
		close(aCh)
	}()

	select {
	case <-aCh:
		assert.True(t, false)
	default:
	}
	assert.NoError(t, q.Start())

	select {
	case <-aCh:
	case <-time.After(time.Millisecond * 100):
		assert.True(t, false, "timeout")
	}
	assert.NoError(t, q.Finalize())
	assert.Error(t, q.Wait(func() {}))
}

func TestQueue_Cancel(t *testing.T) {
	var a int32
	var aStarts = make(chan struct{})
	var aLock = make(chan struct{})
	var bLock chan struct{}
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 1)
	assert.NoError(t, q.Start())
	fl := func() {
		close(aStarts)
		<-aLock
		a++
	}
	f := func() {
		<-bLock
		a++
	}
	assert.NoError(t, q.Add(fl, f, f))
	info := q.Info()
	assert.Equal(t, pb.ModelProcess_Running, info.State)
	assert.Equal(t, int64(0), info.Progress.Done)
	assert.Equal(t, int64(3), info.Progress.Total)
	<-aStarts
	close(aLock)
	assert.NoError(t, q.Cancel())
	assert.Error(t, q.Cancel())
	info = q.Info()
	assert.Equal(t, pb.ModelProcess_Canceled, info.State)
	assert.Equal(t, int64(1), info.Progress.Done)
	assert.Equal(t, int64(3), info.Progress.Total)
}

func TestQueue_Finalize(t *testing.T) {
	s := NewService(func(e *pb.Event) {})
	q := s.NewQueue(pb.ModelProcess{}, 1)
	assert.Error(t, q.Finalize())
	assert.NoError(t, q.Start())
	assert.NoError(t, q.Finalize())
	assert.Error(t, q.Finalize())
}
