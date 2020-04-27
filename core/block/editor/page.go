package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

func NewPage(source file.FileSource, bCtrl bookmark.DoBookmark, lp linkpreview.LinkPreview) *Page {
	sb := smartblock.New()
	return &Page{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		File:       file.NewFile(sb, source),
		Clipboard:  clipboard.NewClipboard(sb),
		Bookmark:   bookmark.NewBookmark(sb, lp, bCtrl),
		Import:     _import.NewImport(sb),
	}
}

type Page struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark
	_import.Import
}
