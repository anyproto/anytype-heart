package editor

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func NewPage(source file.FileSource) *Page {
	sb := smartblock.New()
	return &Page{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		File:       file.NewFile(sb, source),
		Clipboard:  clipboard.NewClipboard(sb),
	}
}

type Page struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
}

func (p *Page) Init(s source.Source) (err error) {
	if err = p.SmartBlock.Init(s); err != nil {
		return
	}
	return p.checkRootBlock()
}

func (p *Page) checkRootBlock() (err error) {
	s := p.NewState()
	if root := s.Get(p.RootId()); root != nil {
		return
	}
	s.Add(simple.New(&model.Block{
		Id: p.RootId(),
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		},
	}))
	return p.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}
