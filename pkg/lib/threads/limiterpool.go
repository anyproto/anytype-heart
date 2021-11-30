package threads

import (
	"context"
	"fmt"
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

func (p *limiterPool) IsPending(id string) bool {
	p.mx.Lock()
	defer p.mx.Unlock()
	_, exists := p.m[id]
	return exists
}

func (p *limiterPool) PendingOperations() int {
	p.mx.Lock()
	defer p.mx.Unlock()
	return len(p.m)
}

func (p *limiterPool) AddOperations(ops []Operation, priority int) {
	p.mx.Lock()
	defer p.mx.Unlock()
	for _, op := range ops {
		it, exists := p.m[op.Id()]
		if exists {
			// we don't deal with running operations
			if it.index != -1 && it.value.(Operation).Type() != op.Type() {
				it.value = op
				p.tasks.UpdatePriority(it, priority)
			}
			continue
		}
		it = &Item{
			value:    op,
			priority: priority,
			index:    0,
		}
		p.addItem(it)
	}
}

func (p *limiterPool) AddOperation(op Operation, priority int) {
	it := &Item{
		value:    op,
		priority: priority,
		index:    0,
	}
	p.mx.Lock()
	defer p.mx.Unlock()
	p.addItem(it)
}

func (p *limiterPool) UpdatePriorities(ids []string, priority int) {
	p.mx.Lock()
	defer p.mx.Unlock()

	for _, id := range ids {
		queueLog.
			With("object id", id).
			With("priority", priority).
			Debug("trying to update priority for object")
		p.updatePriority(id, priority)
	}
}

func (p *limiterPool) UpdatePriority(id string, priority int) {
	p.mx.Lock()
	defer p.mx.Unlock()
	p.UpdatePriority(id, priority)
}

func (p *limiterPool) updatePriority(id string, priority int) {
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
	if p.isDone() {
		return
	}

	p.m[it.value.(Operation).Id()] = it
	p.tasks.Push(it)

	select {
	case p.limiter <- struct{}{}:
	default:
	}
}

func (p *limiterPool) runTask(task *Item) {
	op := task.value.(Operation)
	err := op.Run()

	p.mx.Lock()
	p.runningTasks--
	task.attempt++
	attempt := task.attempt
	priority := task.priority
	p.mx.Unlock()

	select {
	case p.limiter <- struct{}{}:
	default:
	}

	if err != nil {
		err = fmt.Errorf("operation failed with attempt: %d, %w", attempt, err)
	}

	op.OnFinish(err)
	if err == nil {
		p.mx.Lock()
		defer p.mx.Unlock()
		delete(p.m, op.Id())
		return
	}

	if !op.IsRetriable() {
		p.mx.Lock()
		defer p.mx.Unlock()
		delete(p.m, op.Id())
		return
	}

	// we don't remove retriable operations from pending, so we won't be able to add them from outside
	<-time.After(5 * time.Second * time.Duration(attempt) / time.Duration(priority + 1))
	p.mx.Lock()
	defer p.mx.Unlock()
	p.addItem(task)
}

func (p *limiterPool) run() {
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
			queueLog.
				With("priority", newTask.priority).
				With("object id", newTask.value.(Operation).Id()).
				With("operation type", newTask.value.(Operation).Type()).
				Debug("start operation")

			p.runningTasks++
			p.mx.Unlock()

			go p.runTask(newTask)
		}
	}
}

func (p *limiterPool) isDone() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}
