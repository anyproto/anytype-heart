package process

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrNotFound = errors.New("process not found")
)

type Process interface {
	Id() string
	Cancel() (err error)
	Info() pb.ModelProcess
	Done() chan struct{}
}

type Service interface {
	Add(p Process) (err error)
	Cancel(id string) (err error)
	Close() (err error)
}

func NewService(sendEvent func(e *pb.Event)) Service {
	return &service{
		processes: make(map[string]Process),
		waiters:   make(map[string]chan struct{}),
		sendEvent: sendEvent,
	}
}

type service struct {
	processes map[string]Process
	sendEvent func(e *pb.Event)
	waiters   map[string]chan struct{}
	m         sync.Mutex
}

func (s *service) monitor(p Process) {
	ticker := time.NewTicker(time.Second / 2)
	defer ticker.Stop()
	defer func() {
		s.m.Lock()
		close(s.waiters[p.Id()])
		delete(s.processes, p.Id())
		delete(s.waiters, p.Id())
		s.m.Unlock()
	}()
	info := p.Info()
	s.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfProcessNew{
					ProcessNew: &pb.EventProcessNew{
						Process: &info,
					},
				},
			},
		},
	})
	var prevInfo = info
	for {
		select {
		case <-ticker.C:
			info := p.Info()
			if ! infoEquals(info, prevInfo) {
				s.sendEvent(&pb.Event{
					Messages: []*pb.EventMessage{
						{
							Value: &pb.EventMessageValueOfProcessUpdate{
								ProcessUpdate: &pb.EventProcessUpdate{
									Process: &info,
								},
							},
						},
					},
				})
				prevInfo = info
			}
		case <-p.Done():
			info := p.Info()
			s.sendEvent(&pb.Event{
				Messages: []*pb.EventMessage{
					{
						Value: &pb.EventMessageValueOfProcessDone{
							ProcessDone: &pb.EventProcessDone{
								Process: &info,
							},
						},
					},
				},
			})
			return
		}
	}
}

func (s *service) Add(p Process) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if _, ok := s.processes[p.Id()]; ok {
		return fmt.Errorf("process with id '%s' exists", p.Id())
	}
	s.processes[p.Id()] = p
	s.waiters[p.Id()] = make(chan struct{})
	go s.monitor(p)
	return nil
}

func (s *service) Cancel(id string) error {
	s.m.Lock()
	if p, ok := s.processes[id]; ok {
		waitCh := s.waiters[id]
		if err := p.Cancel(); err != nil {
			s.m.Unlock()
			return err
		}
		s.m.Unlock()
		<-waitCh
		return nil
	}
	s.m.Unlock()
	return ErrNotFound
}

func (s *service) Close() error {
	s.m.Lock()
	var ids []string
	for id := range s.processes {
		ids = append(ids, id)
	}
	s.m.Unlock()
	var errs []error
	for _, id := range ids {
		if err := s.Cancel(id); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("process closed with errors: %v", errs)
	}
	return nil
}

func infoEquals(i1, i2 pb.ModelProcess) bool {
	return reflect.DeepEqual(i1, i2)
}
