package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

func NewProfile(m meta.Service, fileSource file.BlockService, bCtrl bookmark.DoBookmark, lp linkpreview.LinkPreview, sendEvent func(e *pb.Event)) *Profile {
	sb := smartblock.New(m)
	f := file.NewFile(sb, fileSource)
	return &Profile{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		File:       f,
		Clipboard:  clipboard.NewClipboard(sb, f),
		Bookmark:   bookmark.NewBookmark(sb, lp, bCtrl),
		sendEvent:  sendEvent,
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

	sendEvent func(e *pb.Event)
}

func (p *Profile) Init(s source.Source, _ bool) (err error) {
	return p.SmartBlock.Init(s, true)
}

func (p *Profile) SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error) {
	if err = p.SmartBlock.SetDetails(details); err != nil {
		return
	}
	meta := p.SmartBlock.Meta()
	if meta == nil {
		return
	}
	p.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfAccountDetails{
					AccountDetails: &pb.EventAccountDetails{
						ProfileId: p.Id(),
						Details:   meta.Details,
					},
				},
			},
		},
	})
	return
}
