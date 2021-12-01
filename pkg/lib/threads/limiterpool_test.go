package threads

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

type testOperation struct {
	doneChannel chan struct{}
	sharedVar   *int
	m           *sync.Mutex
	id          string
	increase    int
}

func (e testOperation) Type() string {
	return "1"
}

func (e testOperation) Id() string {
	return e.id
}

func (e testOperation) IsRetriable() bool {
	return false
}

func (e testOperation) Run() error {
	e.m.Lock()
	*e.sharedVar += e.increase
	e.m.Unlock()
	<-e.doneChannel
	return nil
}

func (e testOperation) OnFinish(err error) {}

func TestLimiterPool_NotExceedMaxSimultaneousOperations(t *testing.T) {
	doneChannel := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	limit := 10
	l := newLimiterPool(ctx, limit)
	sharedVar := 0
	m := sync.Mutex{}
	go l.run()
	for i := 0; i < limit*2; i++ {
		l.AddOperation(testOperation{
			doneChannel: doneChannel,
			id:          strconv.Itoa(i),
			m:           &m,
			sharedVar:   &sharedVar,
			increase:    1,
		}, 1)
	}
	<-time.After(1 * time.Second)
	if sharedVar != limit {
		t.Fatalf("expected %d, but got %d", limit, sharedVar)
	}
	close(doneChannel)
	<-time.After(1 * time.Second)
	if sharedVar != limit*2 {
		t.Fatalf("expected %d, but got %d", limit*2, sharedVar)
	}
	cancel()
}

func TestLimiterPool_FirstCallsWithPriority(t *testing.T) {
	doneChannel := make(chan struct{})
	ctx := context.Background()
	limit := 10
	l := newLimiterPool(ctx, limit)
	sharedVar := 0
	m := sync.Mutex{}
	for i := 0; i < limit*2; i++ {
		increase := 0
		// last limit operations have the most priority
		if i >= limit {
			increase = 1
		}
		l.AddOperation(testOperation{
			doneChannel: doneChannel,
			id:          strconv.Itoa(i),
			m:           &m,
			sharedVar:   &sharedVar,
			increase:    increase,
		}, i)
	}
	go l.run()
	<-time.After(1 * time.Second)
	if sharedVar != limit {
		t.Fatalf("expected %d, but got %d", limit, sharedVar)
	}
	close(doneChannel)
}
