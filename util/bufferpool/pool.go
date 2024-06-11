package bufferpool

import (
	"bytes"
	"sync"
)

type Pool interface {
	Get() Buffer
}

func NewPool() Pool {
	return &bufferPoolWrapper{pool: &sync.Pool{
		New: func() interface{} {
			return []byte{}
		},
	}}
}

type bufferPoolWrapper struct {
	pool *sync.Pool
}

func (bp *bufferPoolWrapper) Get() Buffer {
	b := bp.pool.Get().([]byte)

	buff := &buffer{
		Buffer: bytes.NewBuffer(b[:0]),
		buf:    b,
		pool:   bp.pool,
	}

	return buff
}
