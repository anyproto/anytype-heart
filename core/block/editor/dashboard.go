package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewDashboard(m meta.Service, importServices _import.Services) *Dashboard {
	sb := smartblock.New(m, objects.BundledObjectTypeURLPrefix+"dashboard")
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

func (p *Dashboard) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	if err = p.SmartBlock.Init(s, true, []string{p.SmartBlock.DefaultObjectTypeUrl()}); err != nil {
		return
	}
	p.DisableLayouts()
	return p.init()
}

func (p *Dashboard) init() (err error) {
	s := p.NewState()
	state.CleanupLayouts(s)
	if err = template.ApplyTemplate(p, s,
		template.WithEmpty,
		template.WithDetailName("Home"),
		template.WithDetailIconEmoji("🏠"),
		template.WithRootLink(p.Anytype().PredefinedBlocks().Archive, model.BlockContentLink_Archive),
		template.WithRootLink(p.Anytype().PredefinedBlocks().SetPages, model.BlockContentLink_Archive),
	); err != nil {
		return
	}

	log.Infof("create default structure for dashboard: %v", s.RootId())
	return
}
