package process

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/pb"
)

type Progress interface {
	Process
	SetTotal(total int64)
	SetTotalPreservingRatio(total int64)
	SetDone(done int64)
	AddDone(delta int64)
	SetProgressMessage(msg string)
	Canceled() chan struct{}
	Finish(err error)
	TryStep(delta int64) error
}

func NewProgress(pType pb.ModelProcessType) Progress {
	return &progress{
		id:     bson.NewObjectId().Hex(),
		done:   make(chan struct{}),
		cancel: make(chan struct{}),
		pType:  pType,
	}
}

type progress struct {
	id                    string
	done, cancel          chan struct{}
	totalCount, doneCount int64

	pType    pb.ModelProcessType
	pMessage string
	m        sync.Mutex

	isCancelled         bool
	isDone              bool
	isFinishedWithError bool
}

func (p *progress) SetTotal(total int64) {
	atomic.StoreInt64(&p.totalCount, total)
}

// SetTotalPreservingRatio sets total in case current done is 0. Otherwise, it increases total and done the way
// 1. Their ratio is kept the same.   2. newTotal - newDone = total (function argument)
func (p *progress) SetTotalPreservingRatio(total int64) {
	done := atomic.LoadInt64(&p.doneCount)
	currentTotal := atomic.LoadInt64(&p.totalCount)
	if done != 0 && done < currentTotal {
		left := currentTotal - done
		atomic.StoreInt64(&p.doneCount, done*total/left)
		total = currentTotal * total / left
	}
	atomic.StoreInt64(&p.totalCount, total)
}

func (p *progress) SetDone(done int64) {
	atomic.StoreInt64(&p.doneCount, done)
}

func (p *progress) AddDone(delta int64) {
	atomic.AddInt64(&p.doneCount, delta)
}

func (p *progress) SetProgressMessage(msg string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.pMessage = msg
}

func (p *progress) Canceled() chan struct{} {
	return p.cancel
}

func (p *progress) Finish(err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.isDone {
		return
	}
	if err != nil {
		p.isFinishedWithError = true
	}
	close(p.done)
	p.isDone = true
}

// nolint:revive
func (p *progress) Id() string {
	return p.id
}

func (p *progress) Cancel() (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.isCancelled {
		return
	}
	close(p.cancel)
	p.isCancelled = true
	return
}

func (p *progress) Info() pb.ModelProcess {
	state := pb.ModelProcess_Running
	select {
	case <-p.done:
		state = pb.ModelProcess_Done
		if p.isFinishedWithError {
			state = pb.ModelProcess_Error
		}
	default:
	}
	select {
	case <-p.cancel:
		state = pb.ModelProcess_Canceled
	default:
	}
	p.m.Lock()
	defer p.m.Unlock()
	return pb.ModelProcess{
		Id:    p.id,
		Type:  p.pType,
		State: state,
		Progress: &pb.ModelProcessProgress{
			Total:   atomic.LoadInt64(&p.totalCount),
			Done:    atomic.LoadInt64(&p.doneCount),
			Message: p.pMessage,
		},
	}
}

func (p *progress) Done() chan struct{} {
	return p.done
}

func (p *progress) TryStep(delta int64) error {
	select {
	case <-p.Canceled():
		return fmt.Errorf("cancelled import")
	default:
	}

	p.AddDone(delta)

	return nil
}
