package full

import "sync"

// Lazy derives a table once, on first use, and keeps the result — the
// table or the error. The inputs are constants of the binary, so a failed
// derivation would fail the same way again; retrying it per request would
// only repeat the cost.
type Lazy struct {
	once  sync.Once
	build func() (*Table, error)
	table *Table
	err   error
}

// NewLazy wraps a builder.
func NewLazy(build func() (*Table, error)) *Lazy {
	return &Lazy{build: build}
}

// Get returns the table, deriving it on the first call.
func (l *Lazy) Get() (*Table, error) {
	l.once.Do(func() {
		l.table, l.err = l.build()
	})
	return l.table, l.err
}
