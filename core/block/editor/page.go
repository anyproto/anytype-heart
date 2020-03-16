package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/text"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func NewPage(s source.Source) *Page {
	sb := smartblock.New()
	return &Page{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		File:       nil,
		Text:       nil,
	}
}

type Page struct {
	smartblock.SmartBlock
	basic.Basic
	file.File
	text.Text
}
