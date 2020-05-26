package meta

import (
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
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

func NewService(a anytype.Service, metaFetcher func(id string) (m Meta, err error)) Service {
	return &service{
		ps:          newPubSub(a, metaFetcher),

	}
}

type service struct {
	ps          *pubSub
	m           sync.Mutex
}

func (s *service) PubSub() PubSub {
	return s.ps
}

func (s *service) ReportChange(m Meta) {
	m = copyMeta(m)
	s.ps.setMeta(m)
}

func (s *service) Close() (err error) {
	return s.ps.Close()
}
