package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/google/uuid"
)

func NewSet(ms meta.Service, sendEvent func(e *pb.Event)) *Set {
	sb := smartblock.New(ms)
	return &Set{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Dataview:   dataview.NewDataview(sb, sendEvent),
		sendEvent:  sendEvent,
	}
}

type Set struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	dataview.Dataview

	sendEvent func(e *pb.Event)
}

func (p *Set) Init(s source.Source, _ bool) (err error) {
	if err = p.SmartBlock.Init(s, true); err != nil {
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
	// init dataview
	relations := []*model.BlockContentDataviewRelation{{Id: "id", IsVisible: false}, {Id: "name", IsVisible: true}, {Id: "lastOpened", IsVisible: true}, {Id: "lastModified", IsVisible: true}}
	dataview := simple.New(&model.Block{
		Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				DatabaseId: "pages",
				SchemaURL:  "https://anytype.io/schemas/page",
				Views: []*model.BlockContentDataviewView{
					{
						Id:   uuid.New().String(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All pages",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationId: "name",
								Type:       model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: relations,
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
