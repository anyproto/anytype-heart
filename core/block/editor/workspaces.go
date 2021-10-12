package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewWorkspaces(m meta.Service) *Workspaces {
	return &Workspaces{
		SmartBlock: smartblock.New(m),
	}
}

type Workspaces struct {
	smartblock.SmartBlock
}

func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_Workspace {
		return fmt.Errorf("source type should be a file")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	templates := []template.StateTransformer{
		template.WithTitle,
	}

	return template.ApplyTemplate(p, ctx.State, templates...)
}
