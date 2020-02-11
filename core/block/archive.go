package block

import (
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

var (
	errNotPossibleForArchive = errors.New("not possible for archive")
)

type canArchived interface {
	SetArchived(isArchived bool) (err error)
	Fields() *types.Struct
}

func newArchive(s *service) (smartBlock, error) {
	p := &archive{commonSmart: &commonSmart{s: s}}
	return p, nil
}

type archive struct {
	*commonSmart

	// pageId -> linkId
	pageIds map[string]string
}

func (p *archive) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	p.pageIds = make(map[string]string)
	p.init()
	var toRemove []string
	for _, id := range p.versions[p.GetId()].Model().ChildrenIds {
		if b, ok := p.versions[id]; ok {
			if link := b.Model().GetLink(); link != nil {
				if _, ok := p.pageIds[link.TargetBlockId]; ok {
					toRemove = append(toRemove, b.Model().Id)
				} else {
					p.pageIds[link.TargetBlockId] = b.Model().Id
				}
			}
		}
	}
	if len(toRemove) > 0 {
		s := p.newState()
		for _, rid := range toRemove {
			s.remove(rid)
			s.removeFromChilds(rid)
		}
		p.applyAndSendEventHist(s, false, false)
	}
}

func (p *archive) archivePage(id string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	page, releaseSb, err := p.s.pickBlock(id)
	if err != nil {
		return
	}
	defer releaseSb()

	var a canArchived
	var ok bool
	if a, ok = page.(canArchived); ok {
		if err = a.SetArchived(true); err != nil {
			return
		}
	} else {
		return fmt.Errorf("can't be archived")
	}

	if _, ok := p.pageIds[id]; ok {
		return
	}

	s := p.newState()
	link := s.createLink(&model.Block{
		Id:     id,
		Fields: a.Fields(),
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		},
	})
	l, err := s.create(link)
	if err != nil {
		return
	}
	root := s.get(p.GetId()).Model()
	root.ChildrenIds = append([]string{l.Model().Id}, root.ChildrenIds...)
	p.pageIds[id] = link.Id
	return p.applyAndSendEvent(s)
}

func (p *archive) unArchivePage(id string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	page, releaseSb, err := p.s.pickBlock(id)
	if err != nil {
		return
	}
	defer releaseSb()

	var a canArchived
	var ok bool
	if a, ok = page.(canArchived); ok {
		if err = a.SetArchived(false); err != nil {
			return
		}
	} else {
		return fmt.Errorf("can't be archived")
	}
	var linkId string
	if linkId, ok = p.pageIds[id]; !ok {
		return
	}

	s := p.newState()
	s.remove(linkId)
	s.removeFromChilds(linkId)
	delete(p.pageIds, id)
	return p.applyAndSendEvent(s)
}

func (p *archive) Type() smartBlockType {
	return smartBlockTypeDashboard
}

func (p *archive) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return "", errNotPossibleForArchive
}
func (p *archive) CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Unlink(id ...string) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Split(id string, pos int32) (blockId string, err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Merge(firstId, secondId string) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Move(req pb.RpcBlockListMoveRequest) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Paste(req pb.RpcBlockPasteRequest) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Replace(id string, block *model.Block) (newId string, err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) UpdateBlock(ids []string, hist bool, apply func(b simple.Block) error) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) UpdateIconBlock(id string, apply func(t base.IconBlock) error) (err error) {
	err = errNotPossibleForArchive
	return
}
func (p *archive) Upload(id string, localPath, url string) (err error) {
	err = errNotPossibleForArchive
	return
}
