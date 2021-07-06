package editor

import (
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewTemplate(
	m meta.Service,
	fileSource file.BlockService,
	bCtrl bookmark.DoBookmark,
	importServices _import.Services,
	lp linkpreview.LinkPreview,
) *Template {
	page := NewPage(m, fileSource, bCtrl, importServices, lp)
	return &Template{Page: page}
}

type Template struct {
	*Page
}

func (t *Template) Init(ctx *smartblock.InitContext) (err error) {
	if err = t.Page.Init(ctx); err != nil {
		return
	}
	var fixOt bool
	for _, ot := range t.ObjectTypes() {
		if strings.HasPrefix(ot, "&") {
			fixOt = true
			break
		}
	}
	if t.Type() == model.SmartBlockType_Template && (len(t.ObjectTypes()) != 2 || fixOt) {
		s := t.NewState()
		if targetType := pbtypes.Get(s.Details(), bundle.RelationKeyTargetObjectType.String()).GetStringValue(); targetType != "" {
			s.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), targetType})
			return t.Apply(s, smartblock.NoHistory, smartblock.NoEvent)
		}
	}
	return
}

func (t *Template) GetNewPageState(name string) (st *state.State, err error) {
	st = t.NewState().Copy()
	det := st.Details()
	st.SetObjectType(pbtypes.GetString(det, bundle.RelationKeyTargetObjectType.String()))
	pbtypes.Delete(det, bundle.RelationKeyTargetObjectType.String())
	pbtypes.Delete(det, bundle.RelationKeyTemplateIsBundled.String())
	st.SetDetails(det)
	st.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(name))
	return
}
