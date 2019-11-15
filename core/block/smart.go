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
	s        *service
	block    anytype.Block
	versions map[string]core.BlockVersion

	m sync.RWMutex

	versionsChange func(vers []core.BlockVersion)

	clientEventsCancel func()
	blockChangesCancel func()
	closeWg            sync.WaitGroup
}

func (p *commonSmart) GetId() string {
	return p.block.GetId()
}

func (p *commonSmart) Open(block anytype.Block) (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	p.block = block
	ver, err := p.block.GetCurrentVersion()
	if err != nil {
		return
	}
	p.versions = ver.DependentBlocks()
	p.versions[p.GetId()] = ver

	p.showFullscreen()

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
		go p.clientEventsLoop(events)
		go p.versionChangesLoop(blockChanges)
	}
	return
}

func (p *commonSmart) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	p.m.RLock()
	defer p.m.RUnlock()

	parent, ok := p.versions[req.ParentId]
	if ! ok {
		return "", fmt.Errorf("parent block[%s] not found", req.ParentId)
	}
	var target core.BlockVersion
	if req.TargetId != "" {
		target, ok = p.versions[req.TargetId]
		if ! ok {
			return "", fmt.Errorf("parent block[%s] not found", req.ParentId)
		}
	}

	childrenIds := parent.Model().ChildrenIds
	var pos = len(childrenIds) + 1
	if target != nil {
		targetPos := findPosInSlice(childrenIds, target.Model().Id)
		if targetPos == -1 {
			return "", fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, parent.Model().Id)
		}
		if req.Position == model.Block_AFTER {
			pos = targetPos + 1
		} else {
			pos = targetPos
		}
	}

	var newBlockId string
	childrenIds = insertToSlice(childrenIds, newBlockId, pos)

	return
}

func (p *commonSmart) addBlock(b *model.Block) (id string, err error) {
	// todo:
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
