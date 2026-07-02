package crossspacesub

import "time"

// clock is the timing seam so the broadcast run loop is deterministic in tests.
// Production uses realClock; tests inject a fake whose after() channel they fire
// manually. It is a struct field (never a package var) to stay -race clean.
type clock interface {
	now() time.Time
	after(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) now() time.Time                         { return time.Now() }
func (realClock) after(d time.Duration) <-chan time.Time { return time.After(d) }
