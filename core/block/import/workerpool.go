package importer

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/gogo/protobuf/types"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type ITask interface {
	Execute(ctx *session.Context, oldIDtoNew map[string]string, progress *process.Progress) (*types.Struct, string, error)
}

type WorkerPool struct {
	numWorkers int
	tasks      chan ITask
	mu         sync.Mutex

	quit    chan struct{}
	ce      converter.ConvertError
	details map[string]*types.Struct
}

func (p *WorkerPool) Details() map[string]*types.Struct {
	return p.details
}

func NewPool(numWorkers int, channelSize int) *WorkerPool {
	tasks := make(chan ITask, channelSize)

	return &WorkerPool{
		numWorkers: numWorkers,
		tasks:      tasks,

		ce:      converter.NewError(),
		quit:    make(chan struct{}),
		details: make(map[string]*types.Struct, 0),
	}
}

func (p *WorkerPool) AddWork(t ITask) {
	select {
	case p.tasks <- t:
	case <-p.quit:
	}
}

func (p *WorkerPool) Start(ctx *session.Context, oldIDtoNew map[string]string, mode pb.RpcObjectImportRequestMode, progress *process.Progress) {
	for i := 0; i < p.numWorkers; i++ {
		go func(workerNum int) {
			p.works(ctx, oldIDtoNew, mode, progress)
		}(i)
	}
}

func (p *WorkerPool) works(ctx *session.Context, oldIDtoNew map[string]string, mode pb.RpcObjectImportRequestMode, progress *process.Progress) {
	for {
		select {
		case <-p.quit:
			return
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			var (
				err    error
				detail *types.Struct
				id     string
			)
			if detail, id, err = task.Execute(ctx, oldIDtoNew, progress); err != nil {
				p.ce.Add(id, err)
				if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
					p.stop()
					return
				}
			}
			p.addDetails(id, detail)

		}
	}
}

func (p *WorkerPool) addDetails(id string, detail *types.Struct) {
	defer p.mu.Unlock()
	p.mu.Lock()
	p.details[id] = detail
}

func (p *WorkerPool) stop() {
	close(p.quit)
}
