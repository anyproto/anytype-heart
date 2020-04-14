package editor

import (
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

var log = logging.Logger("anytype-mw-editor")

func NewBreadcrumbs() *Breadcrumbs {
	return &Breadcrumbs{
		SmartBlock: smartblock.New(),
	}
}

type Breadcrumbs struct {
	smartblock.SmartBlock
}

func (b *Breadcrumbs) Init(s source.Source) (err error) {
	if err = b.SmartBlock.Init(s); err != nil {
		return
	}
	return b.checkRootBlock()
}

func (b *Breadcrumbs) checkRootBlock() (err error) {
	s := b.NewState()
	if root := s.Get(b.RootId()); root != nil {
		return
	}
	s.Add(simple.New(&model.Block{
		Id: b.RootId(),
	}))
	return b.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}

func (b *Breadcrumbs) OnSmartOpen(id string) {
	s := b.NewState()
	var exists bool
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			exists = true
			return false
		}
		return true
	})
	if exists {
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
	root := s.Get(b.RootId())
	root.Model().ChildrenIds = append(root.Model().ChildrenIds, link.Model().Id)
	if err := b.Apply(s); err != nil {
		log.Warnf("breadcrumbs page add error: %v", err)
	}
}

func (b *Breadcrumbs) ChainCut(index int) {
	if index < 0 {
		return
	}
	s := b.NewState()
	root := s.Get(b.RootId())
	rootM := root.Model()
	if len(rootM.ChildrenIds) <= index {
		return
	}

	toRemoveIds := rootM.ChildrenIds[index:]
	for _, rId := range toRemoveIds {
		s.Remove(rId)
	}
	if err := b.Apply(s); err != nil {
		log.Warnf("breadcrumbs cut error: %v", err)
	}
}
