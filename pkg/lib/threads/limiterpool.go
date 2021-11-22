package threads

import (
	"context"
	"sync"
	"time"
)

type limiterPool struct {
	limiter      chan struct{}
	tasks        *operationPriorityQueue
	m            map[string]*Item
	ctx          context.Context
	mx           sync.Mutex
	runningTasks int
	started      bool
	limit        int
}

func newLimiterPool(ctx context.Context, limit int) *limiterPool {
	return &limiterPool{
		limiter: make(chan struct{}, limit),
		tasks:   newOperationPriorityQueue(),
		m:       map[string]*Item{},
		ctx:     ctx,
		mx:      sync.Mutex{},
		started: false,
		limit:   limit,
	}
}

func (p *limiterPool) AddOperation(op Operation, priority int) {
	it := &Item{
		value:    op,
		priority: priority,
		index:    0,
	}
	p.addItem(it)
}

func (p *limiterPool) UpdatePriority(id string, priority int) {
	p.mx.Lock()
	defer p.mx.Unlock()
	it, exists := p.m[id]
	if !exists {
		return
	}
	// if the item is running we can just update its priority in map, because it is not in priority queue
	if it.index == -1 {
		it.priority = priority
		return
	}
	p.tasks.UpdatePriority(it, priority)
}

func (p *limiterPool) addItem(it *Item) {
	p.mx.Lock()
	defer p.mx.Unlock()
	if p.isDone() {
		return
	}

	p.m[it.value.(Operation).Id()] = it
	p.tasks.Push(it)

	if !p.started {
		go p.startProcessing()
	}

	select {
	case p.limiter <- struct{}{}:
	default:
	}
	p.started = true
}

func (p *limiterPool) runTask(task *Item) {
	op := task.value.(Operation)
	err := op.Run()

	p.mx.Lock()
	p.runningTasks--
	p.mx.Unlock()

	if err == nil {
		op.OnFinish(nil)
		p.mx.Lock()
		defer p.mx.Unlock()
		delete(p.m, op.Id())
		return
	}

	log.With("id", op.Id()).Errorf("failed to run operation")

	if !op.IsRetriable() {
		p.mx.Lock()
		defer p.mx.Unlock()
		delete(p.m, op.Id())
		return
	}
	task.attempt++
	<-time.After(5 * time.Second * time.Duration(task.attempt) / time.Duration(task.priority))
	p.addItem(task)
}

func (p *limiterPool) startProcessing() {
Loop:
	for {
		select {
		case <-p.ctx.Done():
			break Loop
		case _ = <-p.limiter:
			p.mx.Lock()
			if p.tasks.Size() == 0 || p.runningTasks >= p.limit {
				p.mx.Unlock()
				break
			}
			newTask := p.tasks.Pop()
			p.runningTasks++
			p.mx.Unlock()

			go p.runTask(newTask)
		}
	}
	p.mx.Lock()
	p.started = false
	p.mx.Unlock()
}

func (p *limiterPool) isDone() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}
