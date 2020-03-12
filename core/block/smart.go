package block

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrUnexpectedSmartBlockType = errors.New("unexpected smartBlock type")
)

type smartBlock interface {
	Open(b anytype.SmartBlock, active bool) error
	Init()
	GetId() string
	Type() smartBlockType
	Show() error
	Active(isActive bool)
	Create(req pb.RpcBlockCreateRequest) (id string, err error)
	CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error)
	Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(id ...string) (err error)
	Split(id string, pos int32) (blockId string, err error)
	Merge(firstId, secondId string) error
	Move(req pb.RpcBlockListMoveRequest) error
	Cut(req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error)
	Copy(req pb.RpcBlockCopyRequest) (html string, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
	Replace(id string, block *model.Block) (newId string, err error)
	UpdateBlock(ids []string, hist bool, apply func(b simple.Block) error) (err error)
	UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) error
	UpdateIconBlock(id string, apply func(t base.IconBlock) error) error
	Upload(id string, localPath, url string) error
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)
	SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Undo() error
	Redo() error
	Close() error
	Anytype() anytype.Service
}

type smartBlockType int

const (
	smartBlockTypeDashboard smartBlockType = iota
	smartBlockTypePage
)

func openSmartBlock(s *service, id string, active bool) (sb smartBlock, err error) {
	if id == testPageId {
		sb = &testPage{s: s}
		sb.Open(nil, active)
		sb.Init()
		return
	}

	b, err := s.anytype.GetBlock(id)
	if err != nil {
		return
	}

	switch b.Type() {
	case core.SmartBlockTypeDashboard:
		sb, err = newDashboard(s)
	case core.SmartBlockTypePage:
		sb, err = newPage(s)
	// TODO: archive
	default:
		return nil, fmt.Errorf("%v %T", ErrUnexpectedSmartBlockType, b.Type())
	}
	if err = sb.Open(b, active); err != nil {
		sb.Close()
		return
	}
	sb.Init()
	return
}

type commonSmart struct {
	s        *service
	block    anytype.SmartBlock
	versions map[string]simple.Block
	active   bool

	history history.History

	m sync.RWMutex

	clientEventsCancel func()
	blockChangesCancel func()
	closeWg            *sync.WaitGroup
}

func (p *commonSmart) GetId() string {
	return p.block.ID()
}

func (p *commonSmart) Active(isActive bool) {
	p.active = isActive
}

func (p *commonSmart) Open(block anytype.SmartBlock, active bool) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	p.closeWg = new(sync.WaitGroup)
	p.versions = make(map[string]simple.Block)
	p.active = active
	p.block = block

	snapshot, err := block.GetLastSnapshot()
	if err != nil {
		return fmt.Errorf("GetLastSnapshot error: %v", err)
	}

	blocks, err := snapshot.Blocks()
	if err != nil {
		return fmt.Errorf("snapshot.Blocks error: %v", err)
	}
	for _, m := range blocks {
		p.versions[m.Id] = simple.New(m)
	}
	p.normalize()
	return
}

func (p *commonSmart) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	p.init()
}

func (p *commonSmart) init() {
	for _, v := range p.versions {
		p.onCreate(v)
	}
	p.show()
}

func (p *commonSmart) Show() error {
	p.m.Lock()
	defer p.m.Unlock()
	p.show()
	return nil
}

func (p *commonSmart) Anytype() anytype.Service {
	return p.s.anytype
}

func (p *commonSmart) UpdateBlock(ids []string, hist bool, apply func(b simple.Block) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	for _, id := range ids {
		var b simple.Block
		if b = s.get(id); b == nil {
			return ErrBlockNotFound
		}
		if err = apply(b); err != nil {
			return
		}
	}
	return p.applyAndSendEventHist(s, hist, true)
}

func (p *commonSmart) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	log.Debugf("create block request in: %v", p.GetId())
	s := p.newState()
	if id, err = p.create(s, req); err != nil {
		return
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error) {
	p.m.Lock()
	defer p.m.Unlock()

	if req.Block.GetPage() == nil {
		err = fmt.Errorf("only page blocks can be created")
		return
	}

	s := p.newState()
	if id, err = p.create(s, pb.RpcBlockCreateRequest{
		ContextId: req.ContextId,
		TargetId:  req.TargetId,
		Block:     req.Block,
		Position:  req.Position,
	}); err != nil {
		return
	}
	targetId = s.get(id).Model().GetLink().TargetBlockId
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	pos := req.Position
	targetId := req.TargetId
	for _, id := range req.BlockIds {
		copyId, e := p.copy(s, id)
		if e != nil {
			return nil, e
		}
		if err = p.insertTo(s, targetId, pos, copyId); err != nil {
			return
		}
		pos = model.Block_Bottom
		targetId = copyId
		newIds = append(newIds, copyId)
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
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

func (p *commonSmart) duplicate(s *state, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	pos := req.Position
	targetId := req.TargetId
	for _, id := range req.BlockIds {

		restricted := false

		switch block := s.get(id).Model().Content.(type) {
		case *model.BlockContentOfText:
			if block.Text.Style == model.BlockContentText_Title {
				restricted = true
			}
		}

		if !restricted {
			copyId, e := p.copy(s, id)
			if e != nil {
				return nil, e
			}
			if err = p.insertTo(s, targetId, pos, copyId); err != nil {
				return
			}
			pos = model.Block_Bottom
			targetId = copyId
			newIds = append(newIds, copyId)
		}
	}

	return newIds, nil
}

func (p *commonSmart) pasteBlocks(s *state, req pb.RpcBlockPasteRequest, targetId string) (blockIds []string, err error) {
	parent := s.get(p.GetId()).Model()
	emptyPage := false

	blockIds = []string{}

	if len(parent.ChildrenIds) == 0 {
		emptyPage = true
	}

	for i := 0; i < len(req.AnySlot); i++ {
		copyBlock, err := s.create(req.AnySlot[i])
		if err != nil {
			return blockIds, err
		}

		copyBlockId := copyBlock.Model().Id

		blockIds = append(blockIds, copyBlockId)

		if f, ok := copyBlock.(file.Block); ok {
			file := copyBlock.Model().GetFile()
			url := file.Name
			f.Upload(p.s.anytype, p, "", url)
		}

		if err != nil {
			return blockIds, err
		}
		for i, childrenId := range copyBlock.Model().ChildrenIds {
			if copyBlock.Model().ChildrenIds[i], err = p.copy(s, childrenId); err != nil {
				return blockIds, err
			}
		}

		if emptyPage {
			parent.ChildrenIds = append(parent.ChildrenIds, copyBlockId)
		} else {
			if err = p.insertTo(s, targetId, model.Block_Bottom, copyBlockId); err != nil {
				return blockIds, err
			}
			targetId = copyBlockId
		}
	}

	return blockIds, nil
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
	log.Infof("normalize block: ignore %d blocks; %v", before-after, time.Since(st))
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
	if err = p.insertTo(s, req.TargetId, req.Position, id); err != nil {
		return
	}
	return
}

func (p *commonSmart) createSmartBlock(m *model.Block) (err error) {
	sbType := core.SmartBlockTypePage
	if m.GetDashboard() != nil {
		sbType = core.SmartBlockTypeDashboard
	}
	nb, err := p.s.anytype.CreateBlock(sbType)
	if err != nil {
		return
	}
	m.Id = nb.ID()
	return
}

func (p *commonSmart) insertTo(s *state, targetId string, reqPos model.BlockPosition, ids ...string) (err error) {
	var (
		target        simple.Block
		targetParentM *model.Block
		targetPos     int
	)
	if targetId == "" {
		reqPos = model.Block_Inner
		target = s.get(p.GetId())
	} else {
		target = s.get(targetId)
		if target == nil {
			return fmt.Errorf("target block[%s] not found", targetId)
		}
		if reqPos != model.Block_Inner {
			if pv := s.findParentOf(targetId); pv != nil {
				targetParentM = pv.Model()
			} else {
				return fmt.Errorf("target without parent")
			}
			targetPos = findPosInSlice(targetParentM.ChildrenIds, target.Model().Id)
		}
	}

	if targetId != "" && findPosInSlice(ids, targetId) != -1 {
		return fmt.Errorf("blockIds contains target")
	}
	if targetParentM != nil && findPosInSlice(ids, targetParentM.Id) != -1 {
		return fmt.Errorf("blockIds contains parent")
	}

	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, targetParentM.Id)
	}

	var pos int
	insertPos := func() {
		for _, id := range ids {
			targetParentM.ChildrenIds = insertToSlice(targetParentM.ChildrenIds, id, pos)
			pos++
		}
	}

	switch reqPos {
	case model.Block_Bottom:
		pos = targetPos + 1
		insertPos()
	case model.Block_Top:
		pos = targetPos
		insertPos()
	case model.Block_Left, model.Block_Right:
		if err = p.moveFromSide(s, target, reqPos, ids...); err != nil {
			return
		}
	case model.Block_Inner:
		target.Model().ChildrenIds = append(target.Model().ChildrenIds, ids...)
	case model.Block_Replace:
		pos = targetPos + 1
		insertPos()
		s.remove(target.Model().Id)
		s.removeFromChilds(target.Model().Id)
	default:
		return fmt.Errorf("unexpected position")
	}
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

func (p *commonSmart) Replace(id string, block *model.Block) (newId string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	if newId, err = p.replace(s, id, block); err != nil {
		return
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) replace(s *state, id string, block *model.Block) (newId string, err error) {
	if newId, err = p.create(s, pb.RpcBlockCreateRequest{
		TargetId: id,
		Block:    block,
		Position: model.Block_Bottom,
	}); err != nil {
		return
	}

	if old := s.get(id); old == nil {
		return "", ErrBlockNotFound
	}
	s.removeFromChilds(id)
	s.remove(id)
	return
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

func (p *commonSmart) rangeSplit(s *state, id string, from int32, to int32) (blockId string, err error) {
	t, err := s.getText(id)
	if err != nil {
		return "", err
	}

	newBlocks, text, err := t.RangeSplit(from, to)
	if err != nil {
		return "", err
	}

	if len(text) == 0 {
		p.unlink(s, id)
	}

	if len(newBlocks) == 0 {
		return "", nil
	}

	if blockId, err = p.create(s, pb.RpcBlockCreateRequest{
		TargetId: id,
		Block:    newBlocks[0].Model(),
		Position: model.Block_Bottom,
	}); err != nil {
		return "", err
	}

	return
}

func (p *commonSmart) split(s *state, id string, pos int32) (blockId string, err error) {
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
	return
}

func (p *commonSmart) Split(id string, pos int32) (blockId string, err error) {
	p.m.Lock()
	defer p.m.Unlock()

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
	p.m.Lock()
	defer p.m.Unlock()

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

func (p *commonSmart) UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	var tb text.Block
	for _, id := range ids {
		if tb, err = s.getText(id); err != nil {
			return
		}
		if err = apply(tb); err != nil {
			return
		}
	}
	return p.applyAndSendEventHist(s, true, showEvent)
}

func (p *commonSmart) SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	for _, fr := range fields {
		if fr != nil {
			if err = p.setFields(s, fr.BlockId, fr.Fields); err != nil {
				return
			}
		}
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
	if !p.active {
		return
	}
	blocks := make([]*model.Block, 0, len(p.versions))
	for _, b := range p.versions {
		blocks = append(blocks, b.Model())
	}

	event := &pb.Event{
		ContextId: p.GetId(),
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

func (p *commonSmart) root() *model.Block {
	return p.versions[p.GetId()].Model()
}

func (p *commonSmart) Close() error {
	p.m.Lock()
	defer p.m.Unlock()
	if p.clientEventsCancel != nil {
		p.clientEventsCancel()
	}
	if p.blockChangesCancel != nil {
		p.blockChangesCancel()
	}
	p.closeWg.Wait()
	return nil
}

func (p *commonSmart) applyAndSendEvent(s *state) (err error) {
	return p.applyAndSendEventHist(s, true, true)
}

func (p *commonSmart) applyAndSendEventHist(s *state, hist, event bool) (err error) {
	var action *history.Action
	if hist {
		action = &history.Action{}
	}
	msgs, err := s.apply(action)
	if err != nil {
		return
	}
	if p.active && event && len(msgs) > 0 {
		p.s.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: p.GetId(),
		})
	}
	if hist && p.history != nil && !action.IsEmpty() {
		p.history.Add(*action)
	}
	return
}

func (p *commonSmart) setBlock(b simple.Block) {
	id := b.Model().Id
	_, exists := p.versions[id]
	p.versions[id] = b
	if exists {
		p.onChange(b)
	} else {
		p.onCreate(b)
	}
}

func (p *commonSmart) deleteBlock(id string) (deleted simple.Block) {
	if b, ok := p.versions[id]; ok {
		delete(p.versions, id)
		p.onDelete(b)
		return b
	}
	return nil
}

func (p *commonSmart) onChange(b simple.Block) {

}

func (p *commonSmart) onCreate(b simple.Block) {

}

func (p *commonSmart) onDelete(b simple.Block) {

}

func (p *commonSmart) rangeTextPaste(s *state, id string, from int32, to int32, newText string, newMarks []*model.BlockContentTextMark) error {
	t, err := s.getText(id)
	if err != nil {
		return err
	}
	return t.RangeTextPaste(from, to, newText, newMarks)
}
