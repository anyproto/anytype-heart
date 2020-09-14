package editor

import (
	"fmt"

	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewDashboard(m meta.Service, importServices _import.Services) *Dashboard {
	sb := smartblock.New(m)
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
	var anythingChanged bool

	setDetails := func() error {
		return p.SetDetails([]*pb.RpcBlockSetDetailsDetail{
			{Key: "name", Value: pbtypes.String("Home")},
			{Key: "iconEmoji", Value: pbtypes.String("🏠")},
		})
	}

	addLink := func(targetBlockId string, style model.BlockContentLinkStyle) error {
		linkBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: targetBlockId,
					Style:         style,
				},
			},
		})
		s.Add(linkBlock)
		if err = s.InsertTo(p.RootId(), model.Block_Inner, linkBlock.Model().Id); err != nil {
			return fmt.Errorf("can't insert link: %v", err)
		}
		return nil
	}

	type link struct {
		TargetBlockId string
		Style         model.BlockContentLinkStyle
	}

	var linksToHave = []link{
		{
			TargetBlockId: p.Anytype().PredefinedBlocks().Archive,
			Style:         model.BlockContentLink_Archive,
		},
		{
			TargetBlockId: p.Anytype().PredefinedBlocks().SetPages,
			Style:         model.BlockContentLink_Dataview,
		},
	}

	if p.Meta().Details == nil || p.Meta().Details.Fields == nil || p.Meta().Details.Fields["name"] == nil {
		anythingChanged = true
		err = setDetails()
		if err != nil {
			return err
		}
	}

	var foundLinks = map[string]struct{}{}
	for _, block := range p.Blocks() {
		if link := block.GetLink(); link != nil {
			for _, linkToHave := range linksToHave {
				if linkToHave.TargetBlockId == link.TargetBlockId {
					foundLinks[link.TargetBlockId] = struct{}{}
				}
			}
		}
	}

	for _, linkToHave := range linksToHave {
		if _, found := foundLinks[linkToHave.TargetBlockId]; !found {
			anythingChanged = true
			err = addLink(linkToHave.TargetBlockId, linkToHave.Style)
			if err != nil {
				return err
			}
		}
	}

	if !anythingChanged {
		return nil
	}

	log.Infof("create default structure for dashboard: %v", s.RootId())
	return p.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}
