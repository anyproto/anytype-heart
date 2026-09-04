package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

// Start/stop protocol.
//
// Why not the lock. s.lock serialises every lifecycle call, and a start holds
// it for its whole duration: minutes on a cold sync, and in local-only mode
// for as long as it takes the account to appear on the LAN, which is by
// design unbounded. AccountStop therefore never uses s.lock to reach a start:
// a stop queued on it as a writer would block for that long and, s.lock being
// an RWMutex, would hold every reader — GetApp, so every RPC and every new
// event session — behind it. What a stop can do without the lock is cancel
// the start's context, which is safe because the context is the one thing a
// start shares by design: any-sync hands it to every component's Run, and the
// space service's tech-space load, the wait that blocks in local-only mode,
// runs under it.
//
// Begin, cancel, retract. Every cancellable start — AccountSelect,
// AccountCreate and the restart in AccountChangeNetworkConfigAndRestart —
// publishes itself in s.starting with beginStart before it waits for s.lock,
// so a stop has something to aim at from the first instruction, not only
// during app.Start. There is one slot: a newer start supersedes, that is
// cancels, the one published before it, which is how a newer select or create
// wins over one still booting. AccountStop cancels the slot's start with
// cancelStart and returns at once; from then on the start owns its cleanup.
// startNewApp refuses to boot once the context is cancelled, and end — run
// under s.lock as the last thing a start does — retracts the run, unless a
// newer one has replaced it (hence the identity check), and closes the app
// the start published if the cancel came too late to prevent it. Because the
// retraction happens under the lock, a stop that takes s.lock afterwards
// never finds a start that has already finished.
//
// What is not covered. any-sync's Init loop takes no context, so a cancel
// fired during it is first seen by the Run loop; and a component that ignores
// its context keeps the start, and s.lock, for as long as it blocks — the
// stop still returns, the select does not. The deprecated legacy-export path
// (RecoverFromLegacy) boots its app without publishing itself and cannot be
// cancelled.

// startRun is one in-flight start. ctx is the context it runs under; end
// consults it to learn whether the start was told to stop.
type startRun struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// beginStart publishes a start as the in-flight one, superseding any start
// published before it, and returns the context it runs under and the end to
// defer. end must run under s.lock: defer it after s.lock.Unlock.
func (s *Service) beginStart(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	run := &startRun{ctx: ctx, cancel: cancel}
	s.startMu.Lock()
	prev := s.starting
	s.starting = run
	s.startMu.Unlock()
	if prev != nil {
		log.Info("cancelling the account start in flight: superseded by a newer start")
		prev.cancel()
	}
	end := func() {
		s.startMu.Lock()
		if s.starting == run {
			s.starting = nil
		}
		s.startMu.Unlock()
		if ctx.Err() != nil {
			// told to stop — by AccountStop, by a newer start or by the
			// caller — and the stop has already returned: nothing this start
			// published may outlive it
			if s.app != nil {
				log.Info("closing the app of a cancelled account start")
				_ = s.stop()
			}
			return
		}
		if s.app == nil {
			cancel()
		}
		// a running app keeps its context: any-sync handed it to every Run
		// and objectgc holds it for the app's lifetime, so only the caller's
		// context ends it — as it did before this protocol existed
	}
	return ctx, end
}

// cancelStart cancels the in-flight start, if there is one, and takes it, so
// a second caller finds nothing. It reports whether there was one: that is
// how AccountStop tells "I stopped a pending start" from "nothing to stop".
func (s *Service) cancelStart() bool {
	s.startMu.Lock()
	run := s.starting
	s.starting = nil
	s.startMu.Unlock()
	if run == nil {
		return false
	}
	log.Info("cancelling the account start in flight: stop requested")
	run.cancel()
	return true
}

// startNewApp is where every start boots its app, the cancellable ones and
// the legacy-export path alike. It refuses a start whose context is already
// cancelled — a stop that landed during the lock wait, the previous app's
// close or the repo bootstrap boots nothing and opens no run — and brackets
// anytype.StartNewApp with the recovery tracker's run lifecycle: Begin before
// the start, so the tracker is registered ahead of every bootstrap component
// and its Init runs before any dial or space load; Fail on error, so the
// status stream ends with an account-level verdict. The tracker lives for the
// process and is reused across starts — each Begin opens a new run — which is
// what lets AccountRecoveryState serve it without touching s.lock.
func (s *Service) startNewApp(ctx context.Context, mode pb.EventAccountRecoveryMode, comps ...app.Component) (*app.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("account start cancelled before app start: %w", err)
	}
	if s.recovery != nil {
		s.recovery.Begin(recovery.Run{Mode: mode, Sender: s.eventSender})
		comps = append(comps, s.recovery)
	}
	a, err := anytype.StartNewApp(ctx, s.clientWithVersion, comps...)
	if err != nil {
		if s.recovery != nil {
			s.recovery.Fail(startFailure(ctx, err))
		}
		return nil, err
	}
	return a, nil
}

// startFailure prepares a start error for the tracker's classification. A
// start that returns after its context was cancelled was stopped, whatever
// the failing component reported, so the context's error is joined in and the
// tracker sees the cancel. Otherwise sentinels defined above core/recovery
// that replace the any-sync error chain are joined with the tracker's own, so
// it can name the failure without importing the space tree:
// space.ErrSpaceNotExists is what createAccount reports when the network has
// no space for this account — the wrong-mnemonic / wrong-network case, which
// must not read as Unexpected.
func startFailure(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(err, ctxErr)
	}
	if errors.Is(err, space.ErrSpaceNotExists) {
		return errors.Join(recovery.ErrAccountNotFound, err)
	}
	return err
}
