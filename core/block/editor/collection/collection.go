package collection

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var ErrObjectNotFound = fmt.Errorf("object not found")

func NewCollection(sb smartblock.SmartBlock) Collection {
	return &objectLinksCollection{SmartBlock: sb}
}

type Collection interface {
	AddObject(id string) (err error)
	HasObject(id string) (exists bool, linkId string)
	RemoveObject(id string) (err error)
	GetIds() (ids []string, err error)
}

type objectLinksCollection struct {
	smartblock.SmartBlock
}

func (p *objectLinksCollection) AddObject(id string) (err error) {
	s := p.NewState()
	var found bool
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			found = true
			return false
		}
		return true
	})
	if found {
		return
	}

	link := simple.New(&model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: id,
				Style:         model.BlockContentLink_Page,
			},
		},
	})
	s.Add(link)
	var lastTarget string
	if chIds := s.Get(s.RootId()).Model().ChildrenIds; len(chIds) > 0 {
		lastTarget = chIds[0]
	}
	if err = s.InsertTo(lastTarget, model.Block_Top, link.Model().Id); err != nil {
		return
	}
	return p.Apply(s, smartblock.NoHistory)
}

func (p *objectLinksCollection) HasObject(id string) (exists bool, linkId string) {
	s := p.NewState()
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			exists = true
			linkId = b.Model().Id
			return false
		}
		return true
	})

	return
}

func (p *objectLinksCollection) RemoveObject(id string) (err error) {
	s := p.NewState()
	exists, linkId := p.HasObject(id)
	if !exists {
		return ErrObjectNotFound
	}

	s.Unlink(linkId)
	return p.Apply(s, smartblock.NoHistory)
}

func (p *objectLinksCollection) GetIds() (ids []string, err error) {
	err = p.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil {
			ids = append(ids, link.TargetBlockId)
		}
		return true
	})
	return
}
