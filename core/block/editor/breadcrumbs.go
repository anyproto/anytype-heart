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
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{
				Style: model.BlockContentPage_Breadcrumbs,
			},
		},
	}))
	return b.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}

func (b *Breadcrumbs) SetCrumbs(ids []string) (err error) {
	s := b.NewState()
	var existingLinks = make(map[string]string)
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil {
			existingLinks[link.TargetBlockId] = b.Model().Id
		}
		return true
	})
	root := s.Get(s.RootId()).Model()
	root.ChildrenIds = make([]string, 0, len(ids))
	for _, id := range ids {
		linkId, ok := existingLinks[id]
		if !ok {
			link := simple.New(&model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: id,
						Style:         model.BlockContentLink_Page,
					},
				},
			})
			s.Add(link)
			linkId = link.Model().Id
		}
		root.ChildrenIds = append(root.ChildrenIds, linkId)
	}
	return b.Apply(s)
}
