package process

import (
	"sync"
	"sync/atomic"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/globalsign/mgo/bson"
)

func NewProgress(pType pb.ModelProcessType) *Progress {
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
	atomic.StoreInt64(&p.totalCount, total)
}

func (p *Progress) SetDone(done int64) {
	atomic.StoreInt64(&p.doneCount, done)
}

func (p *Progress) AddDone(delta int64) {
	atomic.AddInt64(&p.doneCount, delta)
}

func (p *Progress) SetProgressMessage(msg string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.pMessage = msg
}

func (p *Progress) Canceled() chan struct{} {
	return p.cancel
}

func (p *Progress) Finish() {
	p.m.Lock()
	defer p.m.Unlock()
	if p.isDone {
		return
	}
	close(p.done)
	p.isDone = true
}

func (p *Progress) Id() string {
	return p.id
}

func (p *Progress) Cancel() (err error) {
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
	return p.done
}
