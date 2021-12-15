package editor

import (
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewTemplate(
	fileSource file.BlockService,
	bCtrl bookmark.DoBookmark,
	importServices _import.Services,
	lp linkpreview.LinkPreview,
) *Template {
	page := NewPage(fileSource, bCtrl, importServices, lp)
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

// GetNewPageState returns state that can be safely used to create the new document
// it has not localDetails set
func (t *Template) GetNewPageState(name string) (st *state.State, err error) {
	st = t.NewState().Copy()
	st.SetObjectType(pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String()))
	st.RemoveDetail(bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyTemplateIsBundled.String())
	// clean-up local details from the template state
	st.SetLocalDetails(nil)

	st.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(name))
	if title := st.Get(template.TitleBlockId); title != nil {
		title.Model().GetText().Text = ""
	}
	return
}
