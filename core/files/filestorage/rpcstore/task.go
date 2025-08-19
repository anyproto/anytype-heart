package rpcstore

import (
	"context"
	"sync"
)

var taskPool = &sync.Pool{
	New: func() any {
		return new(task)
	},
}

func getTask() *task {
	return taskPool.Get().(*task)
}

type result struct {
	err error
}

type task struct {
	ctx         context.Context
	spaceId     string
	denyPeerIds []string
	write       bool
	exec        func(c *client) error
	onFinished  func(t *task, c *client, err error)
	ready       chan result
}

func (t *task) execWithClient(c *client) {
	t.onFinished(t, c, t.exec(c))
}

func (t *task) release() {
	t.ctx = nil
	t.spaceId = ""
	t.denyPeerIds = t.denyPeerIds[:0]
	t.write = false
	t.exec = nil
	taskPool.Put(t)
}
