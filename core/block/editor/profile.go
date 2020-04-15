package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

func NewProfile(source file.FileSource, bCtrl bookmark.DoBookmark, lp linkpreview.LinkPreview) *Profile {
	sb := smartblock.New()
	return &Profile{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		File:       file.NewFile(sb, source),
		Clipboard:  clipboard.NewClipboard(sb),
		Bookmark:   bookmark.NewBookmark(sb, lp, bCtrl),
	}
}

type Profile struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark
}
