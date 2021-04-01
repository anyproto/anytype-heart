package builtintemplate

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const CName = "builtintemplate"

var templatesBinary [][]byte

func New() BuiltinTemplate {
	return new(builtinTemplate)
}

type BuiltinTemplate interface {
	GenerateTemplates() (n int, err error)
	app.ComponentRunnable
}

type builtinTemplate struct {
	core         core.Service
	blockService block.Service
}

func (b *builtinTemplate) Init(a *app.App) (err error) {
	b.blockService = a.MustComponent(block.CName).(block.Service)
	b.core = a.MustComponent(core.CName).(core.Service)
	return
}

func (b *builtinTemplate) Name() (name string) {
	return CName
}

func (b *builtinTemplate) Run() (err error) {
	return
}

func (b *builtinTemplate) Close() (err error) {
	return
}
