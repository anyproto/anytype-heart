package spacev2

import (
	"context"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// Mode is the per-space lifecycle state. Compared to v1 (space/internal/spaceprocess/mode)
// this adds an explicit ModePaused, making load<->unload a first-class, reversible
// transition instead of the v1 defer-only lazy mode (docs/SpaceController.md §11
// forward-looking goals: pause/unload + bounded resident memory). Refine as the
// design firms up — but see §9 for why fresh process instances and
// drain-before-build are load-bearing regardless of the state set.
type Mode int

const (
	ModeUnknown Mode = iota
	ModeInitial
	ModeLoading
	ModePaused // loaded space released to reclaim memory; re-promoted on demand
	ModeOffloading
	ModeJoining
)

// SpaceController is the per-space unit the Service tracks. Unlike Service, this is
// the INTERNAL contract you are free to redesign. Two deliberate changes vs v1
// (docs/SpaceController.md §11 candidates):
//
//   - WaitLoad is typed, replacing v1's untyped Current().(loader.LoadWaiter) assertion.
//   - ModePaused is a real state (see Mode above).
//
// Whatever shape you land on, it must uphold the §9 invariants: status writes are
// Equal()-guarded; lifecycle changes from inside the load pipeline are published
// THROUGH the SpaceView (never by a component driving its own controller); the old
// process is closed/drained before the next is built; each process instance is fresh.
type SpaceController interface {
	SpaceId() string
	Start(ctx context.Context) error
	Mode() Mode

	// WaitLoad blocks until the space is loaded (or the load fails). Replaces the v1
	// Current().(loader.LoadWaiter).WaitLoad(...) pattern.
	WaitLoad(ctx context.Context) (clientspace.Space, error)

	// Update re-derives the target mode from the current persistent status and drives
	// the transition. Must be idempotent and safe under concurrent SpaceView updates.
	Update() error

	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error

	GetStatus() spaceinfo.AccountStatus
	GetLocalStatus() spaceinfo.LocalStatus

	Close(ctx context.Context) error
}
