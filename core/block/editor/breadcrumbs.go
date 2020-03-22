package editor

import (
	"sync"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/google/uuid"
	"github.com/prometheus/common/log"
)

func NewBreadcrumbs() *Breadcrumbs {
	return &Breadcrumbs{}
}

type Breadcrumbs struct {
	id        string
	sendEvent func(e *pb.Event)
	sync.Mutex
	state.Doc
}

func (b *Breadcrumbs) Init(_ source.Source) (err error) {
	b.id = uuid.New().String()
	b.Doc = state.NewDoc(b.id, map[string]simple.Block{
		b.id: simple.New(&model.Block{
			Id: b.id,
			Content: &model.BlockContentOfPage{
				Page: &model.BlockContentPage{
					Style: model.BlockContentPage_Breadcrumbs,
				},
			},
		}),
	})
	return
}

func (b *Breadcrumbs) Id() string {
	return b.id
}

func (b *Breadcrumbs) Show() (err error) {
	if b.sendEvent != nil {
		b.sendEvent(&pb.Event{
			ContextId: b.id,
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockShow{
						BlockShow: &pb.EventBlockShow{
							RootId: b.RootId(),
							Blocks: b.Blocks(),
						},
					},
				},
			},
		})
	}
	return
}

func (b *Breadcrumbs) OnSmartOpen(id string) {
	s := b.NewState()
	var exists bool
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			exists = true
			return false
		}
		return true
	})
	if exists {
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
	root := s.Get(b.RootId())
	root.Model().ChildrenIds = append(root.Model().ChildrenIds, link.Model().Id)
	if err := b.Apply(s); err != nil {
		log.Warnf("breadcrumbs page add error: %v", err)
	}
}

func (b *Breadcrumbs) ChainCut(index int) {
	if index < 0 {
		return
	}
	s := b.NewState()
	root := s.Get(b.RootId())
	rootM := root.Model()
	if len(rootM.ChildrenIds) <= index {
		return
	}

	toRemoveIds := rootM.ChildrenIds[index:]
	for _, rId := range toRemoveIds {
		s.Remove(rId)
	}
	if err := b.Apply(s); err != nil {
		log.Warnf("breadcrumbs cut error: %v", err)
	}
}

func (b *Breadcrumbs) SetEventFunc(f func(e *pb.Event)) {
	b.sendEvent = f
}

func (b *Breadcrumbs) Apply(s *state.State, flags ...smartblock.ApplyFlag) (err error) {
	msgs, _, err := state.ApplyState(s)
	if err != nil {
		return
	}
	if b.sendEvent != nil {
		b.sendEvent(&pb.Event{
			ContextId: b.id,
			Messages:  msgs,
		})
	}
	return
}

func (b *Breadcrumbs) History() history.History {
	return nil
}

func (b *Breadcrumbs) Anytype() anytype.Service {
	return nil
}

func (b *Breadcrumbs) Close() (err error) {
	return nil
}
