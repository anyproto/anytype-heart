package spacev2

import (
	"errors"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var (
	// ErrClosed is returned once the controller or registry has been closed.
	ErrClosed = errors.New("spacev2: closed")
	// ErrSpaceDeleted is returned to waiters when the space converges toward
	// offload (deleted, removed, or join rejected).
	ErrSpaceDeleted = errors.New("spacev2: space is deleted")
)

// State is what the controller currently holds for its space. Only the
// reconcile goroutine moves between states; transient states (Loading,
// Joining, Unloading, Offloading) exist strictly for the duration of the
// corresponding backend call.
type State int

const (
	// StateIdle: nothing resident in memory; on-disk storage may exist.
	// This is also the paused/unloaded state.
	StateIdle State = iota
	StateLoading
	// StateLoaded: a clientspace.Space is live.
	StateLoaded
	StateJoining
	StateUnloading
	StateOffloading
	// StateOffloaded: local data (storage, files, indexes) has been deleted.
	StateOffloaded
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateLoading:
		return "Loading"
	case StateLoaded:
		return "Loaded"
	case StateJoining:
		return "Joining"
	case StateUnloading:
		return "Unloading"
	case StateOffloading:
		return "Offloading"
	case StateOffloaded:
		return "Offloaded"
	case StateClosed:
		return "Closed"
	}
	return "Invalid"
}

// Target is where the controller should converge, derived purely from the
// space's synced account status and the local demand flag.
type Target int

const (
	// TargetIdle: keep the space unloaded (not demanded / paused).
	TargetIdle Target = iota
	TargetLoaded
	// TargetJoining is a target activity rather than an end state: run the
	// join waiter until it resolves the account status to Active or Deleted.
	TargetJoining
	TargetOffloaded
)

func (t Target) String() string {
	switch t {
	case TargetIdle:
		return "Idle"
	case TargetLoaded:
		return "Loaded"
	case TargetJoining:
		return "Joining"
	case TargetOffloaded:
		return "Offloaded"
	}
	return "Invalid"
}

// decide derives the target from the live inputs. Deletion wins over demand;
// AccountStatusRemoving is legacy and treated as Deleted.
func decide(status spaceinfo.AccountStatus, wanted bool) Target {
	switch status {
	case spaceinfo.AccountStatusDeleted, spaceinfo.AccountStatusRemoving:
		return TargetOffloaded
	case spaceinfo.AccountStatusJoining:
		return TargetJoining
	default: // Unknown, Active
		if wanted {
			return TargetLoaded
		}
		return TargetIdle
	}
}

// converged reports whether state already satisfies target.
func converged(s State, t Target) bool {
	switch t {
	case TargetIdle:
		// Offloaded also satisfies "not resident": absence of demand never
		// recreates local data.
		return s == StateIdle || s == StateOffloaded
	case TargetLoaded:
		return s == StateLoaded
	case TargetJoining:
		// Joining never converges statically: the join step runs until the
		// backend resolves the account status to Active or Deleted.
		return false
	case TargetOffloaded:
		return s == StateOffloaded
	}
	return false
}

type fatalError struct{ err error }

func (e fatalError) Error() string { return e.err.Error() }
func (e fatalError) Unwrap() error { return e.err }

// Fatal marks err as non-retryable: the controller stops timer-driven retries,
// surfaces err to waiters, and re-attempts only on the next poke (i.e. when
// some input actually changed).
func Fatal(err error) error {
	if err == nil {
		return nil
	}
	return fatalError{err: err}
}

func isFatal(err error) bool {
	var fe fatalError
	return errors.As(err, &fe)
}
