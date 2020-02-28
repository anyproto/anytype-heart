package block

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
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
	Open(b anytype.Block, active bool) error
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
	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error)
	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error)
	Replace(id string, block *model.Block) (newId string, err error)
	UpdateBlock(ids []string, hist bool, apply func(b simple.Block) error) (err error)
	UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) error
	UpdateIconBlock(id string, apply func(t base.IconBlock) error) error
	Upload(id string, localPath, url string) error
	SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Undo() error
	Redo() error
	Close() error
	Anytype() anytype.Anytype
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

	b, err := s.anytype.GetBlockWithBatcher(id)
	if err != nil {
		return
	}
	ver, err := b.GetCurrentVersion()
	if err != nil {
		return
	}

	log.Infof("block: %+v", b)
	log.Infof("version: %+v", ver)

	switch ver.Model().Content.(type) {
	case *model.BlockContentOfDashboard:
		if ver.Model().GetDashboard().Style == model.BlockContentDashboard_Archive {
			sb, err = newArchive(s)
		} else {
			sb, err = newDashboard(s)
		}
	case *model.BlockContentOfPage:
		sb, err = newPage(s)
	default:
		return nil, fmt.Errorf("%v %T", ErrUnexpectedSmartBlockType, ver.Model().Content)
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
	block    anytype.Block
	versions map[string]simple.Block
	active   bool

	history history.History

	m sync.RWMutex

	versionsChange func(vers []core.BlockVersion)

	clientEventsCancel func()
	blockChangesCancel func()
	closeWg            *sync.WaitGroup
}

func (p *commonSmart) GetId() string {
	return p.block.GetId()
}

func (p *commonSmart) Active(isActive bool) {
	p.active = isActive
}

func (p *commonSmart) hideArchiveBlock() {
	if os.Getenv("ANYTYPE_ARCHIVE") == "1" {
		return
	}
	archiveBlockId := p.Anytype().PredefinedBlockIds().Archive
	if p.block.GetId() != p.Anytype().PredefinedBlockIds().Home {
		return
	}
	var archiveBlockLinkId string
	for id, v := range p.versions {
		if link, isLink := v.Model().Content.(*model.BlockContentOfLink); isLink && link.Link.TargetBlockId == archiveBlockId {
			archiveBlockLinkId = id
			break
		}
	}
	if archiveBlockLinkId == "" {
		return
	}

	p.versions[p.block.GetId()].Model().ChildrenIds = removeFromSlice(p.versions[p.block.GetId()].Model().ChildrenIds, archiveBlockLinkId)
}

func (p *commonSmart) Open(block anytype.Block, active bool) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	p.closeWg = new(sync.WaitGroup)
	p.versions = make(map[string]simple.Block)
	p.active = active
	p.block = block
	ver, err := p.block.GetCurrentVersion()
	if err != nil {
		return
	}

	for id, v := range ver.DependentBlocks() {
		p.versions[id] = simple.New(v.Model())
	}
	p.versions[p.GetId()] = simple.New(ver.Model())
	p.hideArchiveBlock()

	p.normalize()
	if p.active {
		events := make(chan proto.Message)
		p.clientEventsCancel, err = p.block.SubscribeClientEvents(events)
		if err != nil {
			return
		}
		if p.versionsChange != nil {
			blockChanges := make(chan []core.BlockVersion)
			p.blockChangesCancel, err = block.SubscribeNewVersionsOfBlocks(ver.Model().Id, false, blockChanges)
			if err != nil {
				return
			}
			p.closeWg.Add(1)
			go p.versionChangesLoop(blockChanges)
		}
		p.closeWg.Add(1)
		go p.clientEventsLoop(events)
	}
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

func (p *commonSmart) Anytype() anytype.Anytype {
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
		if err = p.insertTo(s, s.get(copyId), targetId, pos); err != nil {
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
			if err = p.insertTo(s, s.get(copyId), targetId, pos); err != nil {
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
			if err = p.insertTo(s, s.get(copyBlockId), targetId, model.Block_Bottom); err != nil {
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
	if err = p.insertTo(s, newBlock, req.TargetId, req.Position); err != nil {
		return
	}
	return
}

func (p *commonSmart) createSmartBlock(m *model.Block) (err error) {
	nb, err := p.block.NewBlock(*m)
	if err != nil {
		return
	}
	m.Id = nb.GetId()
	if _, err = p.block.AddVersions([]*model.Block{m}); err != nil {
		return
	}
	p.block.Flush()
	return
}

func (p *commonSmart) insertTo(s *state, b simple.Block, targetId string, reqPos model.BlockPosition) (err error) {
	switch block := b.Model().Content.(type) {
	case *model.BlockContentOfText:
		if block.Text.Style == model.BlockContentText_Title {
			return nil // Just do not insert, it is not an error
		}
	}

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
		case model.Block_Bottom, model.Block_Replace:
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
	if reqPos == model.Block_Replace {
		s.remove(targetId)
		parent.ChildrenIds = removeFromSlice(parent.ChildrenIds, targetId)
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

func (p *commonSmart) Upload(id string, localPath, url string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	f, err := s.getFile(id)
	if err != nil {
		return
	}
	if err = f.Upload(p.s.anytype, p, localPath, url); err != nil {
		return
	}
	return p.applyAndSendEventHist(s, false, true)
}

func (p *commonSmart) UpdateFileBlock(id string, apply func(f file.Block)) error {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	f, err := s.getFile(id)
	if err != nil {
		return err
	}
	apply(f)
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
		Content:      b.Content,
	}
}

func (p *commonSmart) root() *model.Block {
	return p.versions[p.block.GetId()].Model()
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
	for _, b := range p.versions {
		if p.s.ls != nil {
			p.s.ls.onDelete(p, b)
		}
	}
	p.closeWg.Wait()
	if p.block != nil {
		p.block.Close()
	}
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
	if p.s.ls != nil {
		p.s.ls.onChange(p, b)
	}
}

func (p *commonSmart) onCreate(b simple.Block) {
	if p.s.ls != nil {
		p.s.ls.onCreate(p, b)
	}
}

func (p *commonSmart) onDelete(b simple.Block) {
	if p.s.ls != nil {
		p.s.ls.onDelete(p, b)
	}
}

func (p *commonSmart) rangeTextPaste(s *state, id string, from int32, to int32, newText string, newMarks []*model.BlockContentTextMark) error {
	t, err := s.getText(id)
	if err != nil {
		return err
	}
	return t.RangeTextPaste(from, to, newText, newMarks)
}
