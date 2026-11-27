package store

import (
	"context"
	"sync"
)

// Mem is an in-memory Store for tests. Fail, when set, is returned by
// every call (ErrNoStore, a TemporaryError …).
type Mem struct {
	mu    sync.Mutex
	items map[string][]byte
	Fail  error
	Reads int
}

// NewMem returns an empty Mem.
func NewMem() *Mem { return &Mem{items: map[string][]byte{}} }

func (m *Mem) Kind() string { return "memory" }

// SetFail swaps the failure every call returns.
func (m *Mem) SetFail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Fail = err
}

// ReadCount is how many reads were made.
func (m *Mem) ReadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Reads
}

func (m *Mem) Probe(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Fail
}

func (m *Mem) Read(_ context.Context, id string, _ bool) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reads++
	if m.Fail != nil {
		return nil, m.Fail
	}
	v, ok := m.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

func (m *Mem) Write(_ context.Context, id string, value []byte, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.items[id] = append([]byte(nil), value...)
	return nil
}

func (m *Mem) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	if _, ok := m.items[id]; !ok {
		return ErrNotFound
	}
	delete(m.items, id)
	return nil
}

// Put stores an item directly, for test setup.
func (m *Mem) Put(id string, it Item) {
	raw, err := it.Encode()
	if err != nil {
		panic(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[id] = raw
}

// Remove drops an item directly, for test setup.
func (m *Mem) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
}
