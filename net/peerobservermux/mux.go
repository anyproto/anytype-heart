// Package peerobservermux fills the single peerobserver.CName slot with a
// fan-out, so several consumers (the recovery status tracker today,
// diagnostics later) can observe the peer connection lifecycle without
// competing for the slot. Dispatch is synchronous and holds no lock while an
// observer runs, so the dial-path contract of peerobserver.Observer applies to
// every observer unchanged: never call the pool for the peer named, never
// block. Each observer is called with peerobserver.Notify's panic containment,
// so one panicking observer cannot starve the others.
package peerobservermux

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerobserver"
)

const CName = peerobserver.CName

type Mux struct {
	mu        sync.RWMutex
	observers []peerobserver.Observer
}

func New() *Mux {
	return &Mux{}
}

func (m *Mux) Init(_ *app.App) error { return nil }

func (m *Mux) Name() string { return CName }

// Add registers an observer for every subsequent event. An observer added
// while an event is being dispatched does not see that event.
func (m *Mux) Add(o peerobserver.Observer) {
	if o == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, o)
}

func (m *Mux) ObservePeerEvent(ev peerobserver.Event) {
	m.mu.RLock()
	observers := make([]peerobserver.Observer, len(m.observers))
	copy(observers, m.observers)
	m.mu.RUnlock()
	for _, o := range observers {
		peerobserver.Notify(o, ev)
	}
}
