package block

import (
	"errors"
	"fmt"
	"sync"
	"time"

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
	Move(req pb.RpcBlockListMoveRequest) error
	Replace(id string, block *model.Block) error
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

	p.normalize()

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
		p.closeWg.Add(1)
		go p.versionChangesLoop(blockChanges)
	}
	p.closeWg.Add(1)
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
	s := p.newState()
	if id, err = p.create(s, req); err != nil {
		return
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) Duplicate(req pb.RpcBlockDuplicateRequest) (id string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	copyId, err := p.copy(s, req.BlockId)
	if err != nil {
		return
	}
	if err = p.insertTo(s, s.get(copyId), req.TargetId, req.Position); err != nil {
		return
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return copyId, nil
}

func (p *commonSmart) copy(s *state, sourceId string) (id string, err error) {
	b := s.get(sourceId)
	if b == nil {
		return "", ErrBlockNotFound
	}
	copy, err := s.create(b.Copy().Model())
	if err != nil {
		return
	}
	for i, childrenId := range copy.Model().ChildrenIds {
		if copy.Model().ChildrenIds[i], err = p.copy(s, childrenId); err != nil {
			return
		}
	}
	return copy.Model().Id, nil
}

func (p *commonSmart) normalize() {
	st := time.Now()
	var usedIds = make(map[string]struct{})
	p.normalizeBlock(usedIds, p.versions[p.GetId()])
	cleanVersion := make(map[string]simple.Block)
	for id := range usedIds {
		cleanVersion[id] = p.versions[id]
	}
	before := len(p.versions)
	p.versions = cleanVersion
	after := len(p.versions)
	fmt.Printf("normalize block: ignore %d blocks; %v\n", before-after, time.Since(st))
}

func (p *commonSmart) normalizeBlock(usedIds map[string]struct{}, b simple.Block) {
	usedIds[b.Model().Id] = struct{}{}
	for _, cid := range b.Model().ChildrenIds {
		if _, ok := usedIds[cid]; ok {
			b.Model().ChildrenIds = removeFromSlice(b.Model().ChildrenIds, cid)
			p.normalizeBlock(usedIds, b)
			return
		}
		if cb, ok := p.versions[cid]; ok {
			p.normalizeBlock(usedIds, cb)
		} else {
			b.Model().ChildrenIds = removeFromSlice(b.Model().ChildrenIds, cid)
			p.normalizeBlock(usedIds, b)
			return
		}
	}
}

func (p *commonSmart) create(s *state, req pb.RpcBlockCreateRequest) (id string, err error) {
	if req.Block == nil {
		return "", fmt.Errorf("block can't be empty")
	}
	newBlock, err := s.create(req.Block)
	if err != nil {
		return
	}
	id = newBlock.Model().Id
	if err = p.insertTo(s, newBlock, req.TargetId, req.Position); err != nil {
		return
	}
	return
}

func (p *commonSmart) insertTo(s *state, b simple.Block, targetId string, reqPos model.BlockPosition) (err error) {
	parent := s.get(p.GetId()).Model()
	var target simple.Block
	if targetId != "" {
		target = s.get(targetId)
		if target == nil {
			return fmt.Errorf("target block[%s] not found", targetId)
		}
		if pv := s.findParentOf(targetId); pv != nil {
			parent = pv.Model()
		}
	}

	var pos = len(parent.ChildrenIds) + 1
	if target != nil {
		var targetPos int
		if reqPos != model.Block_Inner {
			targetPos = findPosInSlice(parent.ChildrenIds, target.Model().Id)
			if targetPos == -1 {
				return fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, parent.Id)
			}
		}
		switch reqPos {
		case model.Block_Bottom:
			pos = targetPos + 1
		case model.Block_Top:
			pos = targetPos
		case model.Block_Inner:
			parent = target.Model()
		default:
			return fmt.Errorf("unexpected position for create operation: %v", reqPos)
		}
	}
	parent.ChildrenIds = insertToSlice(parent.ChildrenIds, b.Model().Id, pos)
	return
}

func (p *commonSmart) Unlink(ids ...string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	if err = p.unlink(s, ids...); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) Replace(id string, block *model.Block) error {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()

	if _, err := p.create(s, pb.RpcBlockCreateRequest{
		TargetId: id,
		Block:    block,
		Position: model.Block_Bottom,
	}); err != nil {
		return err
	}

	if old := s.get(id); old == nil {
		return ErrBlockNotFound
	}
	s.removeFromChilds(id)
	s.remove(id)
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) unlink(s *state, ids ...string) (err error) {
	for _, id := range ids {
		if _, ok := p.versions[id]; !ok {
			return ErrBlockNotFound
		}
		parent := s.findParentOf(id)
		if parent != nil {
			parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, id)
		}
		s.remove(id)
	}
	return
}

func (p *commonSmart) findParentOf(id string, sources ...map[string]simple.Block) simple.Block {
	if len(sources) == 0 {
		sources = []map[string]simple.Block{p.versions}
	}
	for _, d := range sources {
		for _, v := range d {
			for _, cid := range v.Model().ChildrenIds {
				if cid == id {
					return v
				}
			}
		}
	}
	return nil
}

func (p *commonSmart) find(id string, sources ...map[string]simple.Block) simple.Block {
	if len(sources) == 0 {
		sources = []map[string]simple.Block{p.versions}
	}
	for _, d := range sources {
		if b, ok := d[id]; ok {
			return b
		}
	}
	return nil
}

func (p *commonSmart) Split(id string, pos int32) (blockId string, err error) {
	s := p.newState()
	t, err := s.getText(id)
	if err != nil {
		return
	}

	newBlock, err := t.Split(pos)
	if err != nil {
		return
	}

	if blockId, err = p.create(s, pb.RpcBlockCreateRequest{
		TargetId: id,
		Block:    newBlock.Model(),
		Position: model.Block_Bottom,
	}); err != nil {
		return "", err
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) Merge(firstId, secondId string) (err error) {
	s := p.newState()
	first, err := s.getText(firstId)
	if err != nil {
		return
	}
	second, err := s.getText(secondId)
	if err != nil {
		return
	}
	if err = first.Merge(second); err != nil {
		return
	}

	if err = p.unlink(s, second.Model().Id); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) UpdateIconBlock(id string, apply func(t base.IconBlock) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	icon, err := s.getIcon(id)
	if err != nil {
		return
	}
	if err = apply(icon); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) UpdateTextBlock(id string, apply func(t text.Block) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	tb, err := s.getText(id)
	if err != nil {
		return
	}
	if err = apply(tb); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) SetFields(id string, fields *types.Struct) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	if err = p.setFields(s, id, fields); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) setFields(s *state, id string, fields *types.Struct) (err error) {
	b := s.get(id)
	if b == nil {
		return ErrBlockNotFound
	}
	b.Model().Fields = fields
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
	defer p.closeWg.Done()
	for m := range events {
		_ = m // TODO: handle client events
	}
}

func (p *commonSmart) versionChangesLoop(blockChanges chan []core.BlockVersion) {
	defer p.closeWg.Done()
	for versions := range blockChanges {
		p.versionsChange(versions)
	}
}

func (p *commonSmart) excludeVirtualIds(ids []string, sources ...map[string]simple.Block) []string {
	res := make([]string, 0, len(ids))
	for _, id := range ids {
		if v := p.find(id, sources...); v != nil && !v.Virtual() {
			res = append(res, id)
		}
	}
	return res
}

func (p *commonSmart) toSave(b *model.Block, sources ...map[string]simple.Block) *model.Block {
	return &model.Block{
		Id:           b.Id,
		Fields:       b.Fields,
		Restrictions: b.Restrictions,
		ChildrenIds:  p.excludeVirtualIds(b.ChildrenIds, sources...),
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
	if !ok {
		return nil, ErrBlockNotFound
	}
	if b.Virtual() {
		return nil, ErrUnexpectedBlockType
	}
	return b, nil
}

func (p *commonSmart) applyAndSendEvent(s *state) (err error) {
	msgs, err := s.apply()
	if err != nil {
		return
	}
	if len(msgs) > 0 {
		p.s.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: p.GetId(),
		})
	}
	return
}
