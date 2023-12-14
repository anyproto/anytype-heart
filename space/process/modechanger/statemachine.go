package modechanger

import (
	"context"
	"errors"
	"sync"
)

type Mode int

const (
	ModeUnknown Mode = iota
	ModeInitial
	ModeLoading
	ModeOffloading
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
}

func NewStateMachine(factory ProcessFactory) (*StateMachine, error) {
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
	}
	err := machine.current.Start(machine.ctx)
	return machine, err
}

func (s *StateMachine) Run() {
	s.loop()
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
	proc = <-wait
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
			s.Unlock()
			if cur != nil {
				cur.Close(s.ctx)
			}
			close(ch)
			return
		case <-s.notify:
			s.Lock()
			cur := s.current
			s.Unlock()

			cur.Close(s.ctx)

			s.Lock()
			s.mode = s.next
			s.next = ModeUnknown
			s.current = s.factory.Process(s.mode)
			cur = s.current
			waiters := append([]waiter{}, s.waiters...)
			s.waiters = nil
			s.Unlock()
			for _, w := range waiters {
				w <- cur
			}
			err := cur.Start(s.ctx)
			if err != nil {
				s.Lock()
				if s.next != ModeUnknown {
					s.Unlock()
					continue
				}
				s.next = ModeInitial
				s.Unlock()
				s.notifyChange()
			}
		}
	}
}
