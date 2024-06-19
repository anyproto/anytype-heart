package bufferpool

import (
	"bytes"
	"io"
	"sync"
)

// poolReadSeekCloser is a custom type that wraps a byte slice and a sync.Pool.
type poolReadSeekCloser struct {
	*bytes.Reader
	buf    []byte
	pool   *sync.Pool
	m      sync.RWMutex
	closed bool
}

// NewPoolReadSeekCloser creates a new poolReadSeekCloser.
func newPoolReadSeekCloser(buf []byte, pool *sync.Pool) io.ReadSeekCloser {
	return &poolReadSeekCloser{
		Reader: bytes.NewReader(buf),
		buf:    buf,
		pool:   pool,
	}
}

// Close puts the buffer back into the pool.
func (prsc *poolReadSeekCloser) Close() error {
	prsc.m.Lock()
	defer prsc.m.Unlock()
	if prsc.closed {
		return nil
	}

	prsc.closed = true
	prsc.pool.Put(prsc.buf)
	return nil
}

func (prsc *poolReadSeekCloser) Read(p []byte) (n int, err error) {
	prsc.m.RLock()
	defer prsc.m.RUnlock()
	if prsc.closed {
		return 0, io.EOF
	}
	return prsc.Reader.Read(p)
}
