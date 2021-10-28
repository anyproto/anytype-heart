package threads

import "github.com/textileio/go-threads/core/thread"

type PullThreadWorker interface {
	AddThread(threadId thread.ID) error
	IsAdded(threadId thread.ID) bool
}

type pullThreadWorker struct{}

func NewPullThreadWorker() PullThreadWorker {
	return &pullThreadWorker{}
}

func (p *pullThreadWorker) AddThread(threadId thread.ID) error {
	panic("implement me")
}

func (p *pullThreadWorker) IsAdded(threadId thread.ID) bool {
	panic("implement me")
}
