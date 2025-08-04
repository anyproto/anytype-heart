package mode

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
)

type Mode int

const (
	ModeUnknown Mode = iota
	ModeInitial
	ModeLoading
	ModeOffloading
	ModeJoining
)

type WaitResult struct {
	Result Process
	Error  error
}

type Process interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	CanTransition(next Mode) bool
}

var (
	ErrInvalidTransition   = errors.New("invalid transition")
	ErrTransitionInProcess = errors.New("transition in process")
	ErrFailedToStart       = errors.New("failed to start")
)

type ProcessFactory interface {
	Process(mode Mode) Process
}

type waiter chan Process

type StateMachine struct {
	sync.Mutex
	current Process
	mode    Mode
	next    Mode
	waiters []waiter
	factory ProcessFactory
	ctx     context.Context
	cancel  context.CancelFunc
	doneCh  chan struct{}
	notify  chan struct{}
	log     logger.CtxLogger
}

func NewStateMachine(factory ProcessFactory, log logger.CtxLogger) (*StateMachine, error) {
	ctx, cancel := context.WithCancel(context.Background())
	machine := &StateMachine{
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
	err := machine.current.Start(machine.ctx)
	if err != nil {
		return nil, err
	}
	machine.Run()
	return machine, err
}

func (s *StateMachine) Run() {
	go s.loop()
}

func (s *StateMachine) Close() {
	s.cancel()
	<-s.doneCh
}

func (s *StateMachine) GetMode() Mode {
	s.Lock()
	defer s.Unlock()
	return s.mode
}

func (s *StateMachine) GetProcess() Process {
	s.Lock()
	defer s.Unlock()
	return s.current
}

func (s *StateMachine) ChangeMode(next Mode) (proc Process, err error) {
	s.log.Debug("changing", zap.Int("next", int(next)))
	s.Lock()
	if s.mode == next {
		proc = s.current
		s.Unlock()
		return
	}
	if s.next != next && s.next != ModeUnknown {
		s.Unlock()
		return nil, ErrTransitionInProcess
	}
	if !s.current.CanTransition(next) {
		s.Unlock()
		return nil, ErrInvalidTransition
	}
	if s.next == ModeUnknown {
		s.notifyChange()
	}
	s.next = next
	wait := make(waiter)
	s.waiters = append(s.waiters, wait)
	s.Unlock()
	s.log.Debug("notify next", zap.Int("next", int(next)))
	// TODO: [MR] send error to waiter
	proc = <-wait
	if proc == nil {
		return nil, ErrFailedToStart
	}
	return
}

func (s *StateMachine) notifyChange() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *StateMachine) loop() {
	for {
		select {
		case <-s.ctx.Done():
			s.Lock()
			cur := s.current
			ch := s.doneCh
			mode := s.mode
			s.Unlock()
			if cur != nil {
				cur.Close(s.ctx)
			}
			s.log.Debug("closed", zap.Int("mode", int(mode)))
			close(ch)
			return
		case <-s.notify:
			s.Lock()
			cur := s.current
			mode := s.mode
			next := s.next
			s.Unlock()
			s.log.Debug("closing", zap.Int("mode", int(mode)))
			cur.Close(s.ctx)

			cur = s.factory.Process(next)
			s.log.Debug("starting", zap.Int("mode", int(next)))
			err := cur.Start(s.ctx)
			if err != nil {
				s.log.Error("failed to start", zap.Error(err))
				s.Lock()
				s.next = ModeUnknown
				s.mode = ModeInitial
				s.current = s.factory.Process(ModeInitial)
				// Initial should always start
				err := s.current.Start(s.ctx)
				if err != nil {
					s.log.Error("failed to start initial", zap.Error(err))
				}
				waiters := append([]waiter{}, s.waiters...)
				s.waiters = nil
				s.Unlock()
				for _, w := range waiters {
					w <- nil
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
				w <- cur
			}
		}
	}
}
