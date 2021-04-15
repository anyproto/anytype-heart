package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
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

func (t *Template) GetNewPageState() (st *state.State, err error) {
	st = t.NewState().Copy()
	det := st.Details()
	st.SetObjectType(pbtypes.GetString(det, bundle.RelationKeyTargetObjectType.String()))
	pbtypes.Delete(det, bundle.RelationKeyTargetObjectType.String())
	st.SetDetails(det)
	return
}
