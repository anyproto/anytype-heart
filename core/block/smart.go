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
	Init()
	GetId() string
	Type() smartBlockType
	Create(req pb.RpcBlockCreateRequest) (id string, err error)
	Update(req pb.RpcBlockUpdateRequest) (err error)
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
		sb.Init()
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

	switch ver.Model().Content.Content.(type) {
	case *model.BlockCoreContentOfDashboard:
		sb, err = newDashboard(s, b)
	case *model.BlockCoreContentOfPage:
		sb, err = newPage(s, b)
	default:
		return nil, ErrUnexpectedSmartBlockType
	}
	if err = sb.Open(b); err != nil {
		sb.Close()
		return
	}
	sb.Init()
	return
}

type commonSmart struct {
	s        *service
	block    anytype.Block
	versions map[string]simple

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
	p.versions = make(map[string]simple)

	p.block = block
	ver, err := p.block.GetCurrentVersion()
	if err != nil {
		return
	}

	for id, v := range ver.DependentBlocks() {
		p.versions[id] = &simpleBlock{v.Model()}
	}
	p.versions[p.GetId()] = &simpleBlock{ver.Model()}

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
	return
}

func (p *commonSmart) Init() {
	p.m.RLock()
	defer p.m.RUnlock()
	p.show()
}

func (p *commonSmart) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	p.m.RLock()
	defer p.m.RUnlock()
	fmt.Println("middle: create block request in:", p.GetId())
	if req.Block == nil {
		return "", fmt.Errorf("block can't be empty")
	}

	parentVer, ok := p.versions[req.ParentId]
	if !ok {
		return "", fmt.Errorf("parent block[%s] not found", req.ParentId)
	}
	parent := parentVer.Model()
	var target simple
	if req.TargetId != "" {
		target, ok = p.versions[req.TargetId]
		if !ok {
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
	fmt.Println("middle: creating new block in lib:", err)
	if err != nil {
		return
	}
	req.Block.Id = newBlock.GetId()

	parent.ChildrenIds = insertToSlice(parent.ChildrenIds, newBlock.GetId(), pos)

	p.versions[newBlock.GetId()] = &simpleBlock{req.Block}
	vers, err := p.block.AddVersions([]*model.Block{p.toSave(req.Block), p.toSave(parent)})
	fmt.Println("middle: save updates in lib:", err)
	if err != nil {
		delete(p.versions, newBlock.GetId())
		return
	}
	id = req.Block.Id
	fmt.Println("middle block created:", req.Block.Id, vers[0].Model().Id)
	p.sendCreateEvents(parent, req.Block)
	return
}

func (p *commonSmart) Update(req pb.RpcBlockUpdateRequest) (err error) {
	if req.Changes == nil || req.Changes.Changes == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	var (
		oldBlocks = make([]simple, len(req.Changes.Changes))
		updateCtx = make(uniqueIds)
	)

	var rollback = func() {
		for _, ob := range oldBlocks {
			if ob != nil {
				p.versions[ob.Model().Id] = ob
			}
		}
	}
	for i, changes := range req.Changes.Changes {
		if oldBlocks[i], err = p.applyChanges(updateCtx, changes); err != nil {
			rollback()
			return
		}
	}

	var updatedBlocks = make([]*model.Block, 0, len(updateCtx))
	for id := range updateCtx {
		updatedBlocks = append(updatedBlocks, p.toSave(p.versions[id].Model()))
	}

	if _, err = p.block.AddVersions(updatedBlocks); err != nil {
		rollback()
		return
	}
	return
}

func (p *commonSmart) sendCreateEvents(parent, new *model.Block) {
	p.s.sendEvent(&pb.Event{Message: &pb.EventMessageOfBlockAdd{BlockAdd: &pb.EventBlockAdd{
		Blocks:    []*model.Block{new},
		ContextId: p.GetId(),
	}}})
	p.s.sendEvent(&pb.Event{
		Message: &pb.EventMessageOfBlockUpdate{
			BlockUpdate: &pb.EventBlockUpdate{
				Changes: &pb.Changes{
					Changes: []*pb.ChangesBlock{
						{
							Id:          parent.Id,
							ChildrenIds: &pb.ChangesBlockChildrenIds{ChildrenIds: parent.ChildrenIds},
						},
					},
					Author: &model.Account{}, // TODO: How to get an Account?
				},
			},
		},
	})
	return
}

func (p *commonSmart) show() {
	blocks := make([]*model.Block, 0, len(p.versions))
	for _, b := range p.versions {
		blocks = append(blocks, b.Model())
	}

	event := &pb.Event{
		Message: &pb.EventMessageOfBlockShow{
			BlockShow: &pb.EventBlockShow{
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

func (p *commonSmart) excludeVirtualIds(ids []string) ([]string) {
	res := make([]string, 0, len(ids))
	for _, id := range ids {
		if v, ok := p.versions[id]; ok && !v.Virtual() {
			res = append(res, id)
		}
	}
	return res
}

func (p *commonSmart) toSave(b *model.Block) *model.Block {
	return &model.Block{
		Id:          b.Id,
		Fields:      b.Fields,
		Permissions: b.Permissions,
		ChildrenIds: p.excludeVirtualIds(b.ChildrenIds),
		IsArchived:  b.IsArchived,
		Content:     b.Content,
	}
}

func (p *commonSmart) root() *model.Block {
	return p.versions[p.block.GetId()].Model()
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
