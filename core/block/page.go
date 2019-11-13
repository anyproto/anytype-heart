package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/gogo/protobuf/proto"
)

func newPage(s *service, block anytype.Block) (smartBlock, error) {
	p := &page{s: s}
	return p, nil
}

type page struct {
	s            *service
	id           string
	eventsCancel func()
	closed       chan struct{}
}

func (p *page) GetId() string {
	return p.id
}

func (p *page) Type() smartBlockType {
	return smartBlockTypePage
}

func (p *page) Open(block anytype.Block) (err error) {
	ver, err := block.GetCurrentVersion()
	if err != nil {
		return
	}
	p.sendOnOpenEvents(ver)
	events := make(chan proto.Message)
	p.eventsCancel = block.SubscribeClientEvents(events)
	return
}

func (p *page) sendOnOpenEvents(block anytype.BlockVersion) {

}

func (p *page) eventHandler(events chan proto.Message) {
	defer close(p.closed)
	for m := range events {
		_ = m
	}
}

func (p *page) Close() error {
	p.eventsCancel()
	<-p.closed
	return nil
}
