package spacev2

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
)

// smProcess is one mode's running representation (v1 mode.Process without
// CanTransition — §11.3: mode gating lives in the controller's status→mode
// mapping, and offloading is deliberately NOT terminal so CancelLeave can
// drive Offloading→Loading).
type smProcess interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

// processFactory mints processes; every call MUST return a fresh instance —
// closed any-sync child apps cannot restart (§9.5).
type processFactory interface {
	Process(md Mode) smProcess
}

var (
	ErrTransitionInProcess = errors.New("transition in process")
	ErrFailedToStart       = errors.New("failed to start")
)

type waitResult struct {
	proc smProcess
	err  error
}

type waiter chan waitResult

// stateMachine serializes per-space mode transitions on a single loop
// goroutine (v1 mode.StateMachine port). Contract (§9.6): same mode → no-op;
// a different in-flight target → ErrTransitionInProcess; same-target callers
// piggyback on the in-flight transition; the old process is closed (drained)
// strictly before the next starts (§9.4); start failure falls back to
// ModeInitial (which must always start) and reports the CAUSE to waiters
// (v1 lost it — waiters got nil and a bare ErrFailedToStart).
type stateMachine struct {
	sync.Mutex
	current Process
	mode    Mode
	next    Mode
	waiters []waiter
	factory processFactory
	ctx     context.Context
	cancel  context.CancelFunc
	doneCh  chan struct{}
	notify  chan struct{}
	log     logger.CtxLogger
}

// Process aliases smProcess for the state machine's current-process storage.
type Process = smProcess

func newStateMachine(factory processFactory, log logger.CtxLogger) (*stateMachine, error) {
	ctx, cancel := context.WithCancel(context.Background())
	machine := &stateMachine{
		mode:    ModeInitial,
		next:    ModeUnknown,
		doneCh:  make(chan struct{}),
		factory: factory,
		ctx:     ctx,
		cancel:  cancel,
		current: factory.Process(ModeInitial),
		notify:  make(chan struct{}, 1),
		log:     log,
	}
	if err := machine.current.Start(machine.ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("start initial process: %w", err)
	}
	go machine.loop()
	return machine, nil
}

func (s *stateMachine) Close() {
	s.cancel()
	<-s.doneCh
}

func (s *stateMachine) GetMode() Mode {
	s.Lock()
	defer s.Unlock()
	return s.mode
}

func (s *stateMachine) GetProcess() smProcess {
	s.Lock()
	defer s.Unlock()
	return s.current
}

// ChangeMode blocks until the requested transition completes (piggybacking on
// an identical in-flight one) and returns the now-current process.
func (s *stateMachine) ChangeMode(next Mode) (smProcess, error) {
	s.Lock()
	if s.mode == next && s.next == ModeUnknown {
		proc := s.current
		s.Unlock()
		return proc, nil
	}
	if s.next != next && s.next != ModeUnknown {
		s.Unlock()
		return nil, ErrTransitionInProcess
	}
	if s.next == ModeUnknown {
		s.next = next
		s.notifyChange()
	}
	wait := make(waiter, 1)
	s.waiters = append(s.waiters, wait)
	s.Unlock()

	res := <-wait
	return res.proc, res.err
}

func (s *stateMachine) notifyChange() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *stateMachine) loop() {
	for {
		select {
		case <-s.ctx.Done():
			s.Lock()
			cur := s.current
			ch := s.doneCh
			s.Unlock()
			if cur != nil {
				if err := cur.Close(s.ctx); err != nil {
					s.log.Warn("close current process", zap.Error(err))
				}
			}
			close(ch)
			return
		case <-s.notify:
			s.Lock()
			cur := s.current
			next := s.next
			s.Unlock()
			// Close (drain) the old process strictly before building the new
			// one — the only serialization of offload vs load on storage (§9.4).
			if err := cur.Close(s.ctx); err != nil {
				s.log.Warn("close process before transition", zap.Error(err))
			}

			cur = s.factory.Process(next)
			err := cur.Start(s.ctx)
			if err != nil {
				s.log.Error("start process", zap.Int("mode", int(next)), zap.Error(err))
				startErr := fmt.Errorf("start mode %d process: %w", next, errors.Join(err, ErrFailedToStart))
				s.Lock()
				s.next = ModeUnknown
				s.mode = ModeInitial
				s.current = s.factory.Process(ModeInitial)
				// Initial must always start.
				if initErr := s.current.Start(s.ctx); initErr != nil {
					s.log.Error("start initial fallback", zap.Error(initErr))
				}
				waiters := append([]waiter{}, s.waiters...)
				s.waiters = nil
				s.Unlock()
				for _, w := range waiters {
					w <- waitResult{err: startErr}
				}
				break
			}
			s.Lock()
			s.mode = s.next
			s.next = ModeUnknown
			s.current = cur
			waiters := append([]waiter{}, s.waiters...)
			s.waiters = nil
			s.Unlock()
			for _, w := range waiters {
				w <- waitResult{proc: cur}
			}
		}
	}
}
