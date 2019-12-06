package block

import (
	"errors"
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
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
	Duplicate(req pb.RpcBlockDuplicateRequest) (id string, err error)
	Unlink(id ...string) (err error)
	Split(id string, pos int32) (blockId string, err error)
	Merge(firstId, secondId string) error
	UpdateTextBlock(id string, apply func(t text.Block) error) error
	UpdateIconBlock(id string, apply func(t base.IconBlock) error) error
	SetFields(id string, fields *types.Struct) (err error)
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

	fmt.Printf("block: %+v\n", b)
	fmt.Printf("version: %+v\n", ver)

	switch ver.Model().Content.(type) {
	case *model.BlockContentOfDashboard:
		sb, err = newDashboard(s, b)
	case *model.BlockContentOfPage:
		sb, err = newPage(s, b)
	default:
		return nil, fmt.Errorf("%v %T", ErrUnexpectedSmartBlockType, ver.Model().Content)
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
	versions map[string]simple.Block

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
	p.versions = make(map[string]simple.Block)

	p.block = block
	ver, err := p.block.GetCurrentVersion()
	if err != nil {
		return
	}

	for id, v := range ver.DependentBlocks() {
		p.versions[id] = simple.New(v.Model())
	}
	p.versions[p.GetId()] = simple.New(ver.Model())

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
	p.m.Lock()
	defer p.m.Unlock()
	fmt.Println("middle: create block request in:", p.GetId())
	return p.create(req)
}

func (p *commonSmart) Duplicate(req pb.RpcBlockDuplicateRequest) (id string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	block, ok := p.versions[req.BlockId]
	if ! ok {
		return "", fmt.Errorf("block %s not found", req.BlockId)
	}
	return p.create(pb.RpcBlockCreateRequest{
		ContextId: req.ContextId,
		TargetId:  req.TargetId,
		Block:     block.Copy().Model(),
		Position:  req.Position,
	})
}

func (p *commonSmart) create(req pb.RpcBlockCreateRequest) (id string, err error) {
	if req.Block == nil {
		return "", fmt.Errorf("block can't be empty")
	}

	parent := p.versions[p.GetId()].Model()
	var target simple.Block
	if req.TargetId != "" {
		var ok bool
		target, ok = p.versions[req.TargetId]
		if !ok {
			return "", fmt.Errorf("target block[%s] not found", req.TargetId)
		}
		if pv := p.findParentOf(req.TargetId); pv != nil {
			parent = pv.Model()
		}
	}

	var pos = len(parent.ChildrenIds) + 1
	if target != nil {
		targetPos := findPosInSlice(parent.ChildrenIds, target.Model().Id)
		if targetPos == -1 {
			return "", fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, parent.Id)
		}
		switch req.Position {
		case model.Block_After:
			pos = targetPos + 1
		case model.Block_Before:
			pos = targetPos
		default:
			return "", fmt.Errorf("unexpected position for create operation: %v", req.Position)
		}
	}

	newBlock, err := p.block.NewBlock(*req.Block)
	fmt.Println("middle: creating new block in lib:", err)
	if err != nil {
		return
	}
	req.Block.Id = newBlock.GetId()

	parent.ChildrenIds = insertToSlice(parent.ChildrenIds, newBlock.GetId(), pos)

	p.versions[newBlock.GetId()] = simple.New(req.Block)
	_, err = p.block.AddVersions([]*model.Block{p.toSave(req.Block), p.toSave(parent)})
	fmt.Println("middle: save updates in lib:", err)
	if err != nil {
		delete(p.versions, newBlock.GetId())
		return
	}
	id = req.Block.Id
	p.sendCreateEvents(parent, req.Block)
	return
}

func (p *commonSmart) Unlink(ids ...string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	return p.unlink(ids...)
}

func (p *commonSmart) unlink(ids ...string) (err error) {
	var toUpdateBlockIds = make(uniqueIds)
	for _, id := range ids {
		_, ok := p.versions[id]
		if ! ok {
			return ErrBlockNotFound
		}
		parent := p.findParentOf(id)
		if parent != nil {
			parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, id)
			toUpdateBlockIds.Add(parent.Model().Id)
		}
		delete(p.versions, id)
	}
	var msgs []*pb.EventMessage
	var parentBlocks []*model.Block
	for id := range toUpdateBlockIds {
		parent := p.versions[id].Model()
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetChildrenIds{
			BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
				Id:          id,
				ChildrenIds: parent.ChildrenIds,
			},
		}})
		parentBlocks = append(parentBlocks, p.toSave(parent))
	}
	for _, id := range ids {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDelete{
			BlockDelete: &pb.EventBlockDelete{
				BlockId: id,
			},
		}})
	}
	if len(msgs) > 0 {
		p.s.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: p.GetId(),
		})
	}
	if len(parentBlocks) > 0 {
		if _, err := p.block.AddVersions(parentBlocks); err != nil {
			return err
		}
	}
	return
}

func (p *commonSmart) findParentOf(id string) simple.Block {
	for _, v := range p.versions {
		for _, cid := range v.Model().ChildrenIds {
			if cid == id {
				return v
			}
		}
	}
	return nil
}

func (p *commonSmart) Split(id string, pos int32) (blockId string, err error) {
	err = p.UpdateTextBlock(id, func(t text.Block) error {
		newBlock, err := t.Split(pos)
		if err != nil {
			return err
		}
		parent := p.findParentOf(id)
		if parent == nil {
			return fmt.Errorf("block %s has not parent", id)
		}
		if blockId, err = p.create(pb.RpcBlockCreateRequest{
			TargetId: id,
			Block:    newBlock.Model(),
			Position: model.Block_After,
		}); err != nil {
			return err
		}
		return nil
	})
	return
}

func (p *commonSmart) Merge(firstId, secondId string) error {
	return p.UpdateTextBlock(firstId, func(t text.Block) error {
		second, ok := p.versions[secondId]
		if ! ok {
			return ErrBlockNotFound
		}
		if err := t.Merge(second); err != nil {
			return err
		}

		return p.unlink(secondId)
	})
}

func (p *commonSmart) UpdateIconBlock(id string, apply func(t base.IconBlock) error) error {
	p.m.Lock()
	defer p.m.Unlock()
	return p.updateBlock(id, func(b simple.Block) error {
		if iconBlock, ok := b.(base.IconBlock); ok {
			return apply(iconBlock)
		}
		return ErrUnexpectedBlockType
	})
}

func (p *commonSmart) UpdateTextBlock(id string, apply func(t text.Block) error) error {
	p.m.Lock()
	defer p.m.Unlock()
	return p.updateBlock(id, func(b simple.Block) error {
		if textBlock, ok := b.(text.Block); ok {
			return apply(textBlock)
		}
		return ErrUnexpectedBlockType
	})
}

func (p *commonSmart) updateBlock(id string, apply func(b simple.Block) error) error {
	block, ok := p.versions[id]
	if !ok {
		return ErrBlockNotFound
	}
	blockCopy := block.Copy()
	if err := apply(blockCopy); err != nil {
		return err
	}
	diff, err := block.Diff(blockCopy)
	if err != nil {
		return err
	}
	if len(diff) == 0 {
		// no changes
		return nil
	}
	if ! blockCopy.Virtual() {
		if _, err := p.block.AddVersions([]*model.Block{p.toSave(blockCopy.Model())}); err != nil {
			return err
		}
	}
	p.versions[id] = blockCopy
	p.s.sendEvent(&pb.Event{
		Messages:  diff,
		ContextId: p.GetId(),
	})
	return nil
}

func (p *commonSmart) SetFields(id string, fields *types.Struct) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	return p.setFields(id, fields)
}

func (p *commonSmart) setFields(id string, fields *types.Struct) (err error) {
	b, err := p.getNonVirtualBlock(id)
	if err != nil {
		return
	}
	copy := b.Copy()
	copy.Model().Fields = fields
	diff, err := b.Diff(copy)
	if err != nil {
		return
	}
	if len(diff) == 0 {
		// no changes
		return nil
	}
	if _, err = p.block.AddVersions([]*model.Block{p.toSave(copy.Model())}); err != nil {
		return
	}
	p.versions[id] = copy
	p.s.sendEvent(&pb.Event{
		Messages:  diff,
		ContextId: p.GetId(),
	})
	return
}

func (p *commonSmart) sendCreateEvents(parent, new *model.Block) {
	p.s.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				&pb.EventMessageValueOfBlockAdd{
					BlockAdd: &pb.EventBlockAdd{
						Blocks: []*model.Block{new},
					},
				},
			},
			{
				&pb.EventMessageValueOfBlockSetChildrenIds{
					BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
						Id:          parent.Id,
						ChildrenIds: parent.ChildrenIds,
					},
				},
			},
		},
		ContextId: p.GetId(),
	})

	return
}

func (p *commonSmart) show() {
	blocks := make([]*model.Block, 0, len(p.versions))
	for _, b := range p.versions {
		blocks = append(blocks, b.Model())
	}

	event := &pb.Event{
		Messages: []*pb.EventMessage{
			{
				&pb.EventMessageValueOfBlockShow{
					BlockShow: &pb.EventBlockShow{
						RootId: p.GetId(),
						Blocks: blocks,
					},
				},
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

func (p *commonSmart) excludeVirtualIds(ids []string) []string {
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
		Id:           b.Id,
		Fields:       b.Fields,
		Restrictions: b.Restrictions,
		ChildrenIds:  p.excludeVirtualIds(b.ChildrenIds),
		IsArchived:   b.IsArchived,
		Content:      b.Content,
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
	if p.block != nil {
		p.block.Close()
	}
	return nil
}

func (p *commonSmart) getNonVirtualBlock(id string) (simple.Block, error) {
	b, ok := p.versions[id]
	if ! ok {
		return nil, ErrBlockNotFound
	}
	if b.Virtual() {
		return nil, ErrUnexpectedBlockType
	}
	return b, nil
}
