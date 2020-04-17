package editor

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewArchive(ctrl ArchiveCtrl) *Archive {
	return &Archive{
		ctrl:       ctrl,
		SmartBlock: smartblock.New(),
	}
}

type ArchiveCtrl interface {
	MarkArchived(id string, archived bool) (err error)
}

type Archive struct {
	ctrl ArchiveCtrl
	smartblock.SmartBlock
}

func (p *Archive) Init(s source.Source) (err error) {
	if err = p.SmartBlock.Init(s); err != nil {
		return
	}
	return p.init()
}

func (p *Archive) init() (err error) {
	if meta := p.SmartBlock.Meta(); meta != nil {
		if meta.Details != nil && meta.Details.Fields != nil && len(meta.Details.Fields) > 0 {
			return
		}
	}
	return p.SetDetails([]*pb.RpcBlockSetDetailsDetail{
		{Key: "name", Value: pbtypes.String("Archive")},
		{Key: "icon", Value: pbtypes.String(":package:")},
	})
}

func (p *Archive) Archive(id string) (err error) {
	s := p.NewState()
	var found bool
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			found = true
			return false
		}
		return true
	})
	if found {
		log.Infof("page %s already archived", id)
		return
	}
	if err = p.ctrl.MarkArchived(id, true); err != nil {
		return
	}
	link := simple.New(&model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: id,
				Style:         model.BlockContentLink_Page,
			},
		},
	})
	s.Add(link)
	var lastTarget string
	if chIds := s.Get(s.RootId()).Model().ChildrenIds; len(chIds) > 0 {
		lastTarget = chIds[0]
	}
	if err = s.InsertTo(lastTarget, model.Block_Top, link.Model().Id); err != nil {
		return
	}
	return p.Apply(s, smartblock.NoHistory)
}

func (p *Archive) UnArchive(id string) (err error) {
	s := p.NewState()
	var (
		found  bool
		linkId string
	)
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			found = true
			linkId = b.Model().Id
			return false
		}
		return true
	})
	if !found {
		log.Infof("page %s not archived", id)
		return
	}
	if err = p.ctrl.MarkArchived(id, false); err != nil {
		return
	}
	s.Remove(linkId)
	return p.Apply(s, smartblock.NoHistory)
}
