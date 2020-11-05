package meta

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

type Meta struct {
	BlockId string
	core.SmartBlockMeta
}

type Service interface {
	PubSub() PubSub
	ReportChange(m Meta)
	Close() (err error)
	FetchDetails(ids []string) (details []Meta)
}

func NewService(a anytype.Service, ss status.Service) Service {
	s := &service{
		ps: newPubSub(a, ss),
	}
	var newSmartblockCh = make(chan string)
	if err := a.InitNewSmartblocksChan(newSmartblockCh); err != nil {
		log.Errorf("can't init new smartblock chan: %v", err)
	} else {
		go s.newSmartblockListener(newSmartblockCh)
	}
	return s
}

type service struct {
	ps *pubSub
	m  sync.Mutex
}

func (s *service) PubSub() PubSub {
	return s.ps
}

func (s *service) ReportChange(m Meta) {
	m = copyMeta(m)
	s.ps.setMeta(m)
}

func (s *service) FetchDetails(ids []string) (details []Meta) {
	if len(ids) == 0 {
		return
	}
	var (
		filled = make(chan struct{})
		done   bool
		m      sync.Mutex
	)
	sub := s.PubSub().NewSubscriber().Callback(func(d Meta) {
		m.Lock()
		defer m.Unlock()
		if done {
			return
		}
		details = append(details, d)
		if len(details) == len(ids) {
			close(filled)
			done = true
		}
	}).Subscribe(ids...)
	defer sub.Close()
	select {
	case <-time.After(time.Second):
	case <-filled:
	}
	return
}

func (s *service) newSmartblockListener(ch chan string) {
	for newId := range ch {
		s.ps.onNewThread(newId)
	}
}

func (s *service) Close() (err error) {
	return s.ps.Close()
}
