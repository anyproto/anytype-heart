package editor

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type Page struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark
	_import.Import
	dataview.Dataview
	table.Editor

	objectStore objectstore.ObjectStore
}

func NewPage() *Page {
	sb := smartblock.New()
	return &Page{SmartBlock: sb}
}

func (p *Page) Init(ctx *smartblock.InitContext) (err error) {
	p.AllOperations = basic.NewBasic(p.SmartBlock)
	p.IHistory = basic.NewHistory(p.SmartBlock)
	p.Text = stext.NewText(
		p.SmartBlock,
		app.MustComponent[objectstore.ObjectStore](ctx.App),
	)
	p.File = file.NewFile(
		p.SmartBlock,
		app.MustComponent[file.BlockService](ctx.App),
		app.MustComponent[core.Service](ctx.App),
	)
	p.Clipboard = clipboard.NewClipboard(
		p.SmartBlock,
		p.File,
		app.MustComponent[core.Service](ctx.App),
	)
	p.Bookmark = bookmark.NewBookmark(
		p.SmartBlock,
		app.MustComponent[bookmark.BlockService](ctx.App),
		app.MustComponent[bookmark.BookmarkService](ctx.App),
		app.MustComponent[objectstore.ObjectStore](ctx.App),
	)
	p.Import = _import.NewImport(
		p.SmartBlock,
		app.MustComponent[_import.Services](ctx.App),
		app.MustComponent[_import.ObjectCreator](ctx.App),
		app.MustComponent[core.Service](ctx.App),
	)
	p.Dataview = dataview.NewDataview(
		p.SmartBlock,
		app.MustComponent[core.Service](ctx.App),
		app.MustComponent[objectstore.ObjectStore](ctx.App),
		app.MustComponent[relation2.Service](ctx.App),
	)
	p.Editor = table.NewEditor(p.SmartBlock)
	p.objectStore = app.MustComponent[objectstore.ObjectStore](ctx.App)

	if ctx.ObjectTypeUrls == nil {
		ctx.ObjectTypeUrls = []string{bundle.TypeKeyPage.URL()}
	}
	newDoc := ctx.State != nil
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	layout, ok := ctx.State.Layout()
	if !ok {
		otypes, _ := objectstore.GetObjectTypes(p.objectStore, ctx.ObjectTypeUrls)
		for _, ot := range otypes {
			layout = ot.Layout
		}
	}

	tmpls := []template.StateTransformer{
		template.WithObjectTypesAndLayout(ctx.ObjectTypeUrls, layout),
		bookmarksvc.WithFixedBookmarks(p.Bookmark),
	}

	// replace title to text block for note
	if newDoc && layout == model.ObjectType_note {
		if name := pbtypes.GetString(ctx.State.Details(), bundle.RelationKeyName.String()); name != "" {
			ctx.State.RemoveDetail(bundle.RelationKeyName.String())
			tmpls = append(tmpls, template.WithFirstTextBlockContent(name))
		}
	}

	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.ByLayout(
			layout,
			tmpls...,
		)...,
	)
}
