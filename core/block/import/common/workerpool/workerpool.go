package workerpool

import (
	"sync"
)

type ITask interface {
	Execute(data interface{}) interface{}
}

type WorkerPool struct {
	numWorkers int
	tasks      chan ITask
	results    chan interface{}
	quit       chan struct{}
}

func NewPool(numWorkers int) *WorkerPool {
	return &WorkerPool{
		numWorkers: numWorkers,
		tasks:      make(chan ITask),

		quit:    make(chan struct{}),
		results: make(chan interface{}),
	}
}

func (p *WorkerPool) AddWork(t ITask) bool {
	select {
	case <-p.quit:
		return true
	case p.tasks <- t:
	}
	return false
}

func (p *WorkerPool) Start(data interface{}) {
	wg := &sync.WaitGroup{}
	for i := 0; i < p.numWorkers; i++ {
		wg.Add(1)
		go func(workerNum int) {
			p.works(data, wg)
		}(i)
	}
	wg.Wait()
	p.CloseResult()
}

func (p *WorkerPool) works(data interface{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for task := range p.tasks {
		select {
		case <-p.quit:
			return
		case p.results <- task.Execute(data):
		}
	}
}

func (p *WorkerPool) Results() chan interface{} {
	return p.results
}

func (p *WorkerPool) Stop() {
	close(p.quit)
}

func (p *WorkerPool) CloseResult() {
	close(p.results)
}

func (p *WorkerPool) CloseTask() {
	close(p.tasks)
}
