package builtintemplate

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "builtintemplate"

var templatesBinary [][]byte

var log = logging.Logger("anytype-mw-builtintemplate")

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
	source       source.Service
}

func (b *builtinTemplate) Init(a *app.App) (err error) {
	b.blockService = a.MustComponent(block.CName).(block.Service)
	b.core = a.MustComponent(core.CName).(core.Service)
	b.source = a.MustComponent(source.CName).(source.Service)
	return
}

func (b *builtinTemplate) Name() (name string) {
	return CName
}

func (b *builtinTemplate) Run() (err error) {
	for _, tb := range templatesBinary {
		if e := b.registerBuiltin(tb); e != nil {
			log.Errorf("can't save builtin template: %v", e)
		}
	}
	return
}

func (b *builtinTemplate) registerBuiltin(tb []byte) (err error) {
	st, err := BytesToState(tb)
	if err != nil {
		return
	}

	id := st.RootId()
	st = st.Copy()
	if ot := st.ObjectType(); ot != bundle.TypeKeyTemplate.URL() {
		st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(ot))
	}
	st.SetObjectType(bundle.TypeKeyTemplate.URL())
	b.source.RegisterStaticSource(id, func() source.Source {
		return source.NewStaticSource(b.core, id, model.SmartBlockType_BundledTemplate, st.Copy())
	})
	return
}

func (b *builtinTemplate) Close() (err error) {
	return
}
