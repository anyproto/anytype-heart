package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/google/uuid"
)

func NewSet(ms meta.Service, dbCtrl database.Ctrl) *Set {
	sb := &Set{
		SmartBlock: smartblock.New(ms),
	}

	sb.Basic = basic.NewBasic(sb)
	sb.IHistory = basic.NewHistory(sb)
	sb.Dataview = dataview.NewDataview(sb, dbCtrl)
	sb.Router = database.New(dbCtrl)
	return sb
}

type Set struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	dataview.Dataview
	database.Router
}

func (p *Set) Init(s source.Source, _ bool) (err error) {
	err = p.SmartBlock.Init(s, true)
	if err != nil {
		return err
	}

	if p.Id() == p.Anytype().PredefinedBlocks().SetPages {
		return p.initPagesSet()
	}
	return
}

func (p *Set) initPagesSet() (err error) {
	if p.Id() != p.Anytype().PredefinedBlocks().SetPages {
		return nil
	}
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
	relations := []*model.BlockContentDataviewRelation{{Key: "id", IsVisible: false}, {Key: "name", IsVisible: true}, {Key: "lastOpened", IsVisible: true}, {Key: "lastModified", IsVisible: true}}
	dataview := simple.New(&model.Block{
		Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Source:    "https://anytype.io/schemas/object/bundled/page",
				SchemaURL: "https://anytype.io/schemas/page",
				Views: []*model.BlockContentDataviewView{
					{
						Id:   uuid.New().String(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All pages",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: "name",
								Type:        model.BlockContentDataviewSort_Asc,
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
