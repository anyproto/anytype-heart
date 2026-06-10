package accountspace

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var (
	ErrCtrlClosed = errors.New("space controller is closed")
	// ErrModeUnreachable is returned by waits whose wanted mode is not the
	// current target (e.g. WaitLoad on a space whose status dictates
	// offloading).
	ErrModeUnreachable = errors.New("mode unreachable")
)

type processFactory interface {
	Process(md mode.Mode) mode.Process
}

// computeTarget is the single place mapping desired inputs to a lifecycle
// state, for all space kinds. Deletion outranks demand; without demand the
// space stays dormant (ModeInitial).
func computeTarget(status spaceinfo.AccountStatus, demand bool) mode.Mode {
	switch status {
	case spaceinfo.AccountStatusDeleted, spaceinfo.AccountStatusRemoving:
		return mode.ModeOffloading
	case spaceinfo.AccountStatusJoining:
		return mode.ModeJoining
	default:
		if demand {
			return mode.ModeLoading
		}
		return mode.ModeInitial
	}
}

// reconciler owns the actual state of one space: it is the only goroutine
// that starts and stops mode processes. Inputs (status, demand) are written
// latest-wins by any goroutine; the loop converges the running process to
// computeTarget(inputs) and re-reads inputs after every transition, so input
// changes are never lost and nothing ever blocks on a transition.
type reconciler struct {
	factory processFactory
	log     logger.CtxLogger

	mu      sync.Mutex
	status  spaceinfo.AccountStatus // desired: from the space view
	demand  bool                    // desired: someone wants the space loaded
	current mode.Process            // actual: running process
	mode    mode.Mode               // actual: its mode
	// failedErr holds the last transition error. While set, the loop does not
	// retry; any input change clears it (no error outlives an input change).
	failedErr error
	// changed is closed and replaced on every state or input change;
	// waiters re-evaluate on it (broadcast).
	changed  chan struct{}
	closeCtx context.Context

	wake   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func newReconciler(factory processFactory, log logger.CtxLogger) *reconciler {
	ctx, cancel := context.WithCancel(context.Background())
	r := &reconciler{
		factory: factory,
		log:     log,
		current: factory.Process(mode.ModeInitial),
		mode:    mode.ModeInitial,
		changed: make(chan struct{}),
		wake:    make(chan struct{}, 1),
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	go r.run()
	return r
}

// setInputs updates the desired state; no-op if nothing changed.
func (r *reconciler) setInputs(status spaceinfo.AccountStatus, demand bool) {
	r.mu.Lock()
	if r.status == status && r.demand == demand {
		r.mu.Unlock()
		return
	}
	r.status = status
	r.demand = demand
	r.failedErr = nil
	r.broadcastLocked()
	r.mu.Unlock()
	r.wakeUp()
}

func (r *reconciler) setStatus(status spaceinfo.AccountStatus) {
	r.mu.Lock()
	if r.status == status {
		r.mu.Unlock()
		return
	}
	r.status = status
	r.failedErr = nil
	r.broadcastLocked()
	r.mu.Unlock()
	r.wakeUp()
}

func (r *reconciler) setDemand() {
	r.mu.Lock()
	if r.demand {
		r.mu.Unlock()
		return
	}
	r.demand = true
	r.failedErr = nil
	r.broadcastLocked()
	r.mu.Unlock()
	r.wakeUp()
}

func (r *reconciler) getMode() mode.Mode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mode
}

// waitConverged blocks until the actual state equals the current target and
// returns the running process. It fails with the real transition error if the
// reconciler is stuck in a failed state, or when the caller ctx is done or
// the reconciler is closed. The target is re-read on every state change, so a
// waiter never hangs on a target that moved away.
func (r *reconciler) waitConverged(ctx context.Context) (mode.Process, error) {
	for {
		r.mu.Lock()
		if r.failedErr != nil {
			err := r.failedErr
			r.mu.Unlock()
			return nil, err
		}
		if target := computeTarget(r.status, r.demand); r.mode == target {
			p := r.current
			r.mu.Unlock()
			return p, nil
		}
		ch := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.ctx.Done():
			return nil, ErrCtrlClosed
		case <-ch:
		}
	}
}

// waitMode blocks until the actual state equals want and returns the running
// process. It fails fast with ErrModeUnreachable when want is not the current
// target, with the real transition error when the reconciler is parked in a
// failed state, and with ErrCtrlClosed / ctx.Err on shutdown or cancellation.
func (r *reconciler) waitMode(ctx context.Context, want mode.Mode) (mode.Process, error) {
	for {
		r.mu.Lock()
		if r.failedErr != nil {
			err := r.failedErr
			r.mu.Unlock()
			return nil, err
		}
		if target := computeTarget(r.status, r.demand); target != want {
			cur := r.mode
			r.mu.Unlock()
			return nil, fmt.Errorf("space is %s, target %s, want %s: %w", cur, target, want, ErrModeUnreachable)
		}
		if r.mode == want {
			p := r.current
			r.mu.Unlock()
			return p, nil
		}
		ch := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.ctx.Done():
			return nil, ErrCtrlClosed
		case <-ch:
		}
	}
}

func (r *reconciler) close(ctx context.Context) {
	r.mu.Lock()
	r.closeCtx = ctx
	r.mu.Unlock()
	r.cancel()
	<-r.done
}

func (r *reconciler) wakeUp() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// broadcastLocked must be called with r.mu held.
func (r *reconciler) broadcastLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *reconciler) run() {
	defer close(r.done)
	for {
		select {
		case <-r.ctx.Done():
			r.teardown()
			return
		case <-r.wake:
			r.reconcile()
		}
	}
}

func (r *reconciler) teardown() {
	r.mu.Lock()
	cur := r.current
	closeCtx := r.closeCtx
	r.mu.Unlock()
	if closeCtx == nil {
		closeCtx = context.Background()
	}
	if cur != nil {
		if err := cur.Close(closeCtx); err != nil {
			r.log.Warn("close process on teardown", zap.Error(err))
		}
	}
	r.log.Debug("closed")
}

// reconcile transitions toward the target until converged. Inputs are
// re-read after every transition (latest-wins); a failed transition parks the
// loop in failedErr until the next input change.
func (r *reconciler) reconcile() {
	for {
		if r.ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		target := computeTarget(r.status, r.demand)
		if target == r.mode || r.failedErr != nil {
			r.mu.Unlock()
			return
		}
		cur := r.current
		curMode := r.mode
		r.mu.Unlock()

		r.log.Debug("transition", zap.Stringer("from", curMode), zap.Stringer("to", target))
		if err := cur.Close(r.ctx); err != nil {
			r.log.Warn("close process", zap.Stringer("mode", curMode), zap.Error(err))
		}
		next := r.factory.Process(target)
		err := next.Start(r.ctx)

		r.mu.Lock()
		if err != nil {
			r.log.Error("failed to start process", zap.Stringer("mode", target), zap.Error(err))
			r.failedErr = err
			r.current = r.factory.Process(mode.ModeInitial)
			r.mode = mode.ModeInitial
		} else {
			r.current = next
			r.mode = target
		}
		r.broadcastLocked()
		r.mu.Unlock()
		if err != nil {
			return
		}
	}
}
