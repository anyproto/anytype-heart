package threads

import "github.com/textileio/go-threads/core/thread"

type ThreadQueue interface {

	PullThread(threadId thread.ID) error
	IsAdded(threadId thread.ID) bool
}

type pullThreadWorker struct{}

func NewPullThreadWorker() ThreadQueue {
	return &pullThreadWorker{}
}

func (p *pullThreadWorker) PullThread(threadId thread.ID) error {
	panic("implement me")
}

func (p *pullThreadWorker) IsAdded(threadId thread.ID) bool {
	panic("implement me")
}
