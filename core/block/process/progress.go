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

func NewProgress(processMessage pb.IsModelProcessMessage) Progress {
	return &progress{
		id:             bson.NewObjectId().Hex(),
		done:           make(chan struct{}),
		cancel:         make(chan struct{}),
		processMessage: processMessage,
	}
}

type progress struct {
	id                    string
	done, cancel          chan struct{}
	totalCount, doneCount int64

	processMessage pb.IsModelProcessMessage
	pMessage       string
	m              sync.Mutex

	isCancelled         bool
	isDone              bool
	isFinishedWithError bool

	err error
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
		p.err = err
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
	var errDescription string
	select {
	case <-p.done:
		state = pb.ModelProcess_Done
		if p.isFinishedWithError {
			errDescription = p.err.Error()
			state = pb.ModelProcess_Error
		} else {
			p.SetDone(atomic.LoadInt64(&p.totalCount))
		}
		return p.makeInfo(state, errDescription)
	default:
	}
	select {
	case <-p.cancel:
		state = pb.ModelProcess_Canceled
	default:
	}
	return p.makeInfo(state, errDescription)
}

func (p *progress) makeInfo(state pb.ModelProcessState, errDescription string) pb.ModelProcess {
	p.m.Lock()
	defer p.m.Unlock()
	return pb.ModelProcess{
		Id:    p.id,
		State: state,
		Progress: &pb.ModelProcessProgress{
			Total:   atomic.LoadInt64(&p.totalCount),
			Done:    atomic.LoadInt64(&p.doneCount),
			Message: p.pMessage,
		},
		Error:   errDescription,
		Message: p.processMessage,
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
