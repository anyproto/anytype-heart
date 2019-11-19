package block

import (
	"errors"
	"fmt"
	"sync"

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
	Create(req pb.RpcBlockCreateRequest) (id string, err error)
	Close() error
}

type smartBlockType int

const (
	smartBlockTypeDashboard smartBlockType = iota
	smartBlockTypePage
)

func openSmartBlock(s *service, id string) (sb smartBlock, err error) {
	if id == testPageId {
		sb = &testPage{s: s}
		sb.Open(nil)
		return
	}

	b, err := s.anytype.GetBlock(id)
	if err != nil {
		return
	}
	ver, err := b.GetCurrentVersion()
	if err != nil {
		return
	}

	switch ver.Model().Content.(type) {
	case *model.BlockContentOfDashboard:
		sb, err = newDashboard(s, b)
	case *model.BlockContentOfPage:
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
	s        *service
	block    anytype.Block
	versions map[string]core.BlockVersion

	m sync.RWMutex

	versionsChange func(vers []core.BlockVersion)

	clientEventsCancel func()
	blockChangesCancel func()
	closeWg            *sync.WaitGroup
}

func (p *commonSmart) GetId() string {
	return p.block.GetId()
}

func (p *commonSmart) Open(block anytype.Block) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	p.closeWg = new(sync.WaitGroup)
	p.block = block
	ver, err := p.block.GetCurrentVersion()
	if err != nil {
		return
	}
	p.versions = ver.DependentBlocks()
	p.versions[p.GetId()] = ver

	events := make(chan proto.Message)
	p.clientEventsCancel, err = p.block.SubscribeClientEvents(events)
	if err != nil {
		return
	}
	if p.versionsChange != nil {
		blockChanges := make(chan []core.BlockVersion)
		p.blockChangesCancel, err = block.SubscribeNewVersionsOfBlocks(ver.Model().Id, blockChanges)
		if err != nil {
			return
		}
		go p.versionChangesLoop(blockChanges)
	}
	go p.clientEventsLoop(events)
	p.showFullscreen()
	return
}

func (p *commonSmart) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	p.m.RLock()
	defer p.m.RUnlock()

	if req.Block == nil {
		return "", fmt.Errorf("block can't be empty")
	}

	parentVer, ok := p.versions[req.ParentId]
	if ! ok {
		return "", fmt.Errorf("parent block[%s] not found", req.ParentId)
	}
	parent := parentVer.Model()
	var target core.BlockVersion
	if req.TargetId != "" {
		target, ok = p.versions[req.TargetId]
		if ! ok {
			return "", fmt.Errorf("parent block[%s] not found", req.ParentId)
		}
	}

	var pos = len(parent.ChildrenIds) + 1
	if target != nil {
		targetPos := findPosInSlice(parent.ChildrenIds, target.Model().Id)
		if targetPos == -1 {
			return "", fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, parent.Id)
		}
		if req.Position == model.Block_After {
			pos = targetPos + 1
		} else {
			pos = targetPos
		}
	}

	newBlock, err := p.block.NewBlock(*req.Block)
	if err != nil {
		return
	}
	newBlockVer, err := newBlock.GetCurrentVersion()
	if err != nil {
		return
	}
	parent.ChildrenIds = insertToSlice(parent.ChildrenIds, newBlock.GetId(), pos)

	vers, err := p.block.AddVersions([]*model.Block{newBlockVer.Model(), parent})
	if err != nil {
		return
	}
	id = vers[0].Model().Id
	p.sendCreateEvents(parent, newBlockVer.Model())
	return
}

func (p *commonSmart) sendCreateEvents(parent, new *model.Block) {
	p.s.sendEvent(&pb.Event{Message: &pb.EventMessageOfBlockAdd{BlockAdd: &pb.EventBlockAdd{
		Blocks:    []*model.Block{new},
		ContextId: p.GetId(),
	}}})
	p.s.sendEvent(&pb.Event{Message: &pb.EventMessageOfBlockUpdate{BlockUpdate: &pb.EventBlockUpdate{
		Changes: &pb.ChangeMultipleBlocksList{
			Changes: []*pb.ChangeSingleBlocksList{
				{
					Id: []string{parent.Id},
					Change: &pb.ChangeSingleBlocksListChangeOfChildrenIds{
						ChildrenIds: &pb.ChangeBlockChildrenIds{
							ChildrenIds: parent.ChildrenIds,
						},
					},
				},
			},
		},
		ContextId: p.GetId(),
	}}})
	return
}

func (p *commonSmart) showFullscreen() {
	blocks := make([]*model.Block, 0, len(p.versions))
	for _, b := range p.versions {
		blocks = append(blocks, b.Model())
	}
	event := &pb.Event{
		Message: &pb.EventMessageOfBlockShowFullscreen{
			BlockShowFullscreen: &pb.EventBlockShowFullscreen{
				RootId: p.GetId(),
				Blocks: blocks,
			},
		},
	}
	p.s.sendEvent(event)
}

func (p *commonSmart) clientEventsLoop(events chan proto.Message) {
	p.closeWg.Add(1)
	defer p.closeWg.Done()
	for m := range events {
		_ = m // TODO: handle client events
	}
}

func (p *commonSmart) versionChangesLoop(blockChanges chan []core.BlockVersion) {
	p.closeWg.Add(1)
	defer p.closeWg.Done()
	for versions := range blockChanges {
		p.versionsChange(versions)
	}
}

func (p *commonSmart) Close() error {
	if p.clientEventsCancel != nil {
		p.clientEventsCancel()
	}
	if p.blockChangesCancel != nil {
		p.blockChangesCancel()
	}
	p.closeWg.Wait()
	return nil
}
