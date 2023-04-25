package page

import (
	"context"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type ITask interface {
	Execute(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, request *block.MapRequest) ([]*converter.Snapshot, []*converter.Relation, converter.ConvertError)
	ID() string
}

type WorkerPool struct {
	numWorkers int
	tasks      chan ITask
	mu         sync.Mutex

	quit              chan struct{}
	relationsToPageID map[string][]*converter.Relation
	allSnapshots      []*converter.Snapshot
	ce                converter.ConvertError
}

func NewPool(numWorkers int, channelSize int) *WorkerPool {
	tasks := make(chan ITask, channelSize)

	return &WorkerPool{
		numWorkers: numWorkers,
		tasks:      tasks,

		quit:              make(chan struct{}),
		relationsToPageID: make(map[string][]*converter.Relation, 0),
		allSnapshots:      make([]*converter.Snapshot, 0),
		ce:                converter.NewError(),
	}
}

func (p *WorkerPool) AddWork(t ITask) {
	select {
	case p.tasks <- t:
	case <-p.quit:
	}
}

func (p *WorkerPool) ConvertError() converter.ConvertError {
	return p.ce
}

func (p *WorkerPool) RelationsToPageID() map[string][]*converter.Relation {
	return p.relationsToPageID
}

func (p *WorkerPool) AllSnapshots() []*converter.Snapshot {
	return p.allSnapshots
}

func (p *WorkerPool) Start(ctx context.Context,
	apiKey string,
	mode pb.RpcObjectImportRequestMode,
	request *block.MapRequest,
	progress *process.Progress) {
	for i := 0; i < p.numWorkers; i++ {
		go func(workerNum int) {
			p.work(ctx, apiKey, mode, request, progress)
		}(i)
	}
}

func (p *WorkerPool) work(ctx context.Context, apiKey string, mode pb.RpcObjectImportRequestMode, request *block.MapRequest, progress *process.Progress) {
	for {
		select {
		case <-p.quit:
			return
		case task, ok := <-p.tasks:
			if err := progress.TryStep(1); err != nil {
				p.ce = converter.NewFromError("", err)
				p.stop()
				return
			}
			if !ok {
				return
			}
			var (
				err converter.ConvertError
				sn  []*converter.Snapshot
				rel []*converter.Relation
			)
			if sn, rel, err = task.Execute(ctx, apiKey, mode, request); !err.IsEmpty() {
				p.ce.Merge(err)
				if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
					p.stop()
					return
				}
			}

			p.addSnapshot(sn)
			p.addRelation(request.NotionPageIdsToAnytype[task.ID()], rel)

			progress.AddDone(1)
		}
	}
}

func (p *WorkerPool) addRelation(id string, rel []*converter.Relation) {
	defer p.mu.Unlock()
	p.mu.Lock()
	p.relationsToPageID[id] = rel
}

func (p *WorkerPool) addSnapshot(sn []*converter.Snapshot) {
	defer p.mu.Unlock()
	p.mu.Lock()
	p.allSnapshots = append(p.allSnapshots, sn...)
}

func (p *WorkerPool) stop() {
	close(p.quit)
}
