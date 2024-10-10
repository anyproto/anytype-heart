package bufferpool

import (
	"bytes"
	"io"
	"sync"
)

type Buffer interface {
	io.Writer
	io.Closer
	GetReadSeekCloser() (io.ReadSeekCloser, error)
}

type buffer struct {
	*bytes.Buffer
	buf    []byte
	pool   *sync.Pool
	m      sync.Mutex
	closed bool
}

func (b *buffer) Close() error {
	b.m.Lock()
	defer b.m.Unlock()
	if !b.closed {
		b.pool.Put(b.buf)
		b.closed = true
	}
	return nil
}

// GetReadSeekCloser returns a ReadSeekCloser that reads from the buffer.
// GetReadSeekCloser after Close will return EOF.
// It's a responsibility of the caller to Close the ReadSeekCloser to put the buffer back into the pool.
func (b *buffer) GetReadSeekCloser() (io.ReadSeekCloser, error) {
	b.m.Lock()
	defer b.m.Unlock()
	if !b.closed {
		b.closed = true
		return newPoolReadSeekCloser(b.Buffer.Bytes(), b.pool), nil
	}

	return nil, io.EOF
}

// Close puts the buffer back into the pool.
// Close after GetReadSeekCloser does nothing.
func (b *buffer) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	if b.closed {
		return 0, io.EOF
	}
	return b.Buffer.Write(p)
}

// Close puts the buffer back into the pool.
// Close after GetReadSeekCloser does nothing.
