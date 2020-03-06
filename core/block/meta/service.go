package meta

import (
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/gogo/protobuf/types"
)

type Meta struct {
	BlockId string
	Details *types.Struct
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
	s.ps.setMeta(m)
}

func (s *service) Close() (err error) {
	return s.ps.Close()
}
