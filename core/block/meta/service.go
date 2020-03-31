package meta

import (
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/mohae/deepcopy"
)

type Meta struct {
	BlockId string
	core.SmartBlockMeta
}

type Service interface {
	PubSub() PubSub
	ReportChange(m Meta)
	Close() (err error)
}

func NewService(a anytype.Service) Service {
	return &service{
		ps: newPubSub(a),
	}
}

type service struct {
	ps *pubSub

	m sync.Mutex
}

func (s *service) PubSub() PubSub {
	return s.ps
}

func (s *service) ReportChange(m Meta) {
	s.ps.setMeta(deepcopy.Copy(m).(Meta))
}

func (s *service) Close() (err error) {
	return s.ps.Close()
}
