package builtintemplate

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
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
	for _, tb := range templatesBinary {
		if e := b.saveBuiltinTemplate(tb); e != nil {
			log.Errorf("can't save builtin template: %v", e)
		}
	}
	return
}

func (b *builtinTemplate) saveBuiltinTemplate(tb []byte) (err error) {
	store := b.core.ObjectStore()
	st, err := BytesToState(tb)
	if err != nil {
		return
	}
	_, total, err := store.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyBuiltinTemplateId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(st.RootId()),
			},
		},
	})
	if err != nil {
		return
	}
	if total == 0 {
		origId := st.RootId()
		st = st.Copy()
		if ot := st.ObjectType(); ot != bundle.TypeKeyTemplate.URL() {
			st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(ot))
		}
		st.SetDetail(bundle.RelationKeyBuiltinTemplateId.String(), pbtypes.String(origId))
		st.SetObjectType(bundle.TypeKeyTemplate.URL())
		id, _, err := b.blockService.CreateSmartBlockFromState(smartblock.SmartBlockTypeTemplate, nil, nil, st)
		if err != nil {
			return err
		}
		log.Infof("created template '%v from orig '%v'", id, origId)
	}
	return
}

func (b *builtinTemplate) Close() (err error) {
	return
}
