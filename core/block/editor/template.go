package editor

import (
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type Template struct {
	*Page
}

func NewTemplate(
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	fileBlockService file.BlockService,
	bookmarkBlockService bookmark.BlockService,
	bookmarkService bookmark.BookmarkService,
	relationService relation2.Service,
	tempDirProvider core.TempDirProvider,
) *Template {
	return &Template{Page: NewPage(
		objectStore,
		anytype,
		fileBlockService,
		bookmarkBlockService,
		bookmarkService,
		relationService,
		tempDirProvider,
	)}
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
