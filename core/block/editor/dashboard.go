package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func NewDashboard() *Dashboard {
	sb := smartblock.New()
	return &Dashboard{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
	}
}

type Dashboard struct {
	smartblock.SmartBlock
	basic.Basic
}

func (p *Dashboard) Init(s source.Source) (err error) {
	if err = p.SmartBlock.Init(s); err != nil {
		return
	}
	return p.checkRootBlock()
}

func (p *Dashboard) checkRootBlock() (err error) {
	s := p.NewState()
	if root := s.Get(p.RootId()); root != nil {
		return
	}
	s.Add(simple.New(&model.Block{
		Id: p.RootId(),
		Content: &model.BlockContentOfDashboard{
			Dashboard: &model.BlockContentDashboard{},
		},
	}))
	archive := simple.New(&model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: p.Anytype().PredefinedBlocks().Archive,
				Style:         model.BlockContentLink_Archive,
			},
		},
	})
	s.Add(archive)
	if err = s.InsertTo(p.RootId(), model.Block_Inner, archive.Model().Id); err != nil {
		return fmt.Errorf("can't insert archive: %v", err)
	}
	log.Infof("create default structure for dashboard: %v", s.RootId())
	return p.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}
