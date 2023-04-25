package process

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

type IProgress interface {
	Process
	SetTotal(total int64)
	SetDone(done int64)
	AddDone(delta int64)
	SetProgressMessage(msg string)
	Canceled() chan struct{}
	Finish()
	TryStep(delta int64) error
}

func NewProgress(pType pb.ModelProcessType) IProgress {
	return &Progress{
		id:     bson.NewObjectId().Hex(),
		done:   make(chan struct{}),
		cancel: make(chan struct{}),
		pType:  pType,
	}
}

type Progress struct {
	id                    string
	done, cancel          chan struct{}
	totalCount, doneCount int64

	pType    pb.ModelProcessType
	pMessage string
	m        sync.Mutex

	isCancelled bool
	isDone      bool
}

func (p *Progress) SetTotal(total int64) {
	if p == nil {
		return
	}
	atomic.StoreInt64(&p.totalCount, total)
}

func (p *Progress) SetDone(done int64) {
	if p == nil {
		return
	}
	atomic.StoreInt64(&p.doneCount, done)
}

func (p *Progress) AddDone(delta int64) {
	if p == nil {
		return
	}
	atomic.AddInt64(&p.doneCount, delta)
}

func (p *Progress) SetProgressMessage(msg string) {
	if p == nil {
		return
	}
	p.m.Lock()
	defer p.m.Unlock()
	p.pMessage = msg
}

func (p *Progress) Canceled() chan struct{} {
	if p == nil {
		return nil
	}
	return p.cancel
}

func (p *Progress) Finish() {
	if p == nil {
		return
	}
	p.m.Lock()
	defer p.m.Unlock()
	if p.isDone {
		return
	}
	close(p.done)
	p.isDone = true
}

func (p *Progress) Id() string {
	if p == nil {
		return ""
	}
	return p.id
}

func (p *Progress) Cancel() (err error) {
	if p == nil {
		return nil
	}
	p.m.Lock()
	defer p.m.Unlock()
	if p.isCancelled {
		return
	}
	close(p.cancel)
	p.isCancelled = true
	return
}

func (p *Progress) Info() pb.ModelProcess {
	if p == nil {
		return pb.ModelProcess{}
	}
	state := pb.ModelProcess_Running
	select {
	case <-p.done:
		state = pb.ModelProcess_Done
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

func (p *Progress) Done() chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

func (p *Progress) TryStep(delta int64) error {
	if p == nil {
		return nil
	}
	select {
	case <-p.Canceled():
		return fmt.Errorf("cancelled import")
	default:
	}

	p.AddDone(delta)

	return nil
}
