package editor

import (
	"fmt"

	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewDashboard(importServices _import.Services) *Dashboard {
	sb := smartblock.New()
	return &Dashboard{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		Import:     _import.NewImport(sb, importServices),
	}
}

type Dashboard struct {
	smartblock.SmartBlock
	basic.Basic
	_import.Import
}

func (p *Dashboard) Init(s source.Source, _ bool) (err error) {
	if err = p.SmartBlock.Init(s, true); err != nil {
		return
	}
	return p.init()
}

func (p *Dashboard) init() (err error) {
	s := p.NewState()
	root := s.Get(p.RootId())
	setDetails := func() error {
		return p.SetDetails([]*pb.RpcBlockSetDetailsDetail{
			{Key: "name", Value: pbtypes.String("Home")},
			{Key: "iconEmoji", Value: pbtypes.String("🏠")},
		})
	}
	if len(root.Model().ChildrenIds) > 0 {
		if p.Meta().Details == nil || p.Meta().Details.Fields == nil || p.Meta().Details.Fields["name"] == nil {
			return setDetails()
		}
		return
	}
	// add archive link
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

	err = setDetails()
	if err != nil {
		return fmt.Errorf("can't set details: %v", err)
	}

	log.Infof("create default structure for dashboard: %v", s.RootId())
	return p.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}
