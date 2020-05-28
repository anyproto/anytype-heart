package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewSet(source file.FileSource, bCtrl bookmark.DoBookmark, lp linkpreview.LinkPreview, sendEvent func(e *pb.Event)) *Profile {
	sb := smartblock.New()
	return &Profile{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		File:       file.NewFile(sb, source),
		Clipboard:  clipboard.NewClipboard(sb),
		Bookmark:   bookmark.NewBookmark(sb, lp, bCtrl),
		sendEvent:  sendEvent,
	}
}

type Set struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark

	sendEvent func(e *pb.Event)
}

func (p *Set) Init(s source.Source) (err error) {
	if err = p.SmartBlock.Init(s); err != nil {
		return
	}
	return p.init()
}

func (p *Set) init() (err error) {
	s := p.NewState()
	root := s.Get(p.RootId())
	setDetails := func() error {
		return p.SetDetails([]*pb.RpcBlockSetDetailsDetail{
			{Key: "name", Value: pbtypes.String("Pages")},
			{Key: "iconEmoji", Value: pbtypes.String("📒")},
		})
	}
	if len(root.Model().ChildrenIds) > 0 {
		return
	}
	// add archive link
	dataview := simple.New(&model.Block{
		Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				DatabaseId: "pages",
				Views: []*model.BlockContentDataviewView{
					{
						Type: model.BlockContentDataviewView_Table,
						Name: "Table",
						Sorts: []*model.BlockContentDataviewSort{
							{
								Column: "name",
								Type:   model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: []string{"name", "isArchived"},
						Filters:   nil,
					},
					{
						Type: model.BlockContentDataviewView_Gallery,
						Name: "Gallery",
						Sorts: []*model.BlockContentDataviewSort{
							{
								Column: "name",
								Type:   model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: []string{"name", "isArchived"},
						Filters:   nil,
					},
					{
						Type: model.BlockContentDataviewView_Kanban,
						Name: "Kanban",
						Sorts: []*model.BlockContentDataviewSort{
							{
								Column: "name",
								Type:   model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: []string{"name", "isArchived"},
						Filters:   nil,
					},
					{
						Type: model.BlockContentDataviewView_List,
						Name: "List",
						Sorts: []*model.BlockContentDataviewSort{
							{
								Column: "name",
								Type:   model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: []string{"name", "isArchived"},
						Filters:   nil,
					},
				},
			},
		},
	})

	s.Add(dataview)

	if err = s.InsertTo(p.RootId(), model.Block_Inner, dataview.Model().Id); err != nil {
		return fmt.Errorf("can't insert dataview: %v", err)
	}

	err = setDetails()
	if err != nil {
		return fmt.Errorf("can't set details: %v", err)
	}

	log.Infof("create default structure for set: %v", s.RootId())
	return p.Apply(s, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Set) SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error) {
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
