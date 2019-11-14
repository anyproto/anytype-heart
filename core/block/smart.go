package block

import (
	"errors"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

var (
	ErrUnexpectedSmartBlockType = errors.New("unexpected smartBlock type")
)

type smartBlock interface {
	Open(b anytype.Block) error
	GetId() string
	Type() smartBlockType
	Close() error
}

type smartBlockType int

const (
	smartBlockTypeDashboard smartBlockType = iota
	smartBlockTypePage
)

func openSmartBlock(s *service, id string) (sb smartBlock, err error) {
	b, err := s.anytype.GetBlock(id)
	if err != nil {
		return
	}
	ver, err := b.GetCurrentVersion()
	if err != nil {
		return
	}

	switch ver.(type) {
	case *core.DashboardVersion:
		sb, err = newDashboard(s, b)
	case *core.PageVersion:
		sb, err = newPage(s, b)
	default:
		return nil, ErrUnexpectedSmartBlockType
	}
	if err = sb.Open(b); err != nil {
		return
	}
	return
}

type commonSmart struct {
	s            *service
	id           string
	eventsCancel func()
	closed       chan struct{}
}

func (p *commonSmart) GetId() string {
	return p.id
}

func (p *commonSmart) Open(block anytype.Block) (err error) {
	ver, err := block.GetCurrentVersion()
	if err != nil {
		return
	}
	p.sendOnOpenEvents(ver)
	events := make(chan proto.Message)
	p.eventsCancel = block.SubscribeClientEvents(events)
	return
}

func (p *commonSmart) sendOnOpenEvents(ver anytype.BlockVersion) {
	deps := ver.GetDependentBlocks()
	blocks := make([]*model.Block, 0, len(deps)+1)
	blocks = append(blocks, versionToModel(ver))
	for _, b := range deps {
		blocks = append(blocks, versionToModel(b))
	}
	event := &pb.Event{
		Message: &pb.EventMessageOfBlockShowFullscreen{
			BlockShowFullscreen: &pb.EventBlockShowFullscreen{
				RootId: ver.GetBlockId(),
				Blocks: blocks,
			},
		},
	}
	p.s.sendEvent(event)
}

func (p *commonSmart) eventHandler(events chan proto.Message) {
	defer close(p.closed)
	for m := range events {
		_ = m
	}
}

func (p *commonSmart) Close() error {
	p.eventsCancel()
	<-p.closed
	return nil
}
