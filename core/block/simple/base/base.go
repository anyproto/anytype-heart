package base

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func init() {
	simple.RegisterCreator(func(m *model.Block) simple.Block {
		if m.GetDiv() != nil {
			return NewDiv(m)
		}
		return nil
	})
	simple.RegisterFallback(func(m *model.Block) simple.Block {
		return NewBase(m)
	})
}

func NewBase(block *model.Block) simple.Block {
	return &Base{Block: block}
}

type Base struct {
	*model.Block
}

func (s *Base) Model() *model.Block {
	return s.Block
}

func (s *Base) ModelToSave() *model.Block {
	return s.Block
}

func (s *Base) Diff(block simple.Block) (msgs []simple.EventMessage, err error) {
	m := block.Model()
	if !stringSlicesEq(m.ChildrenIds, s.ChildrenIds) {
		m := &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetChildrenIds{BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
			Id:          s.Id,
			ChildrenIds: m.ChildrenIds,
		}}}
		msgs = append(msgs, simple.EventMessage{Msg: m})
	}

	if s.Restrictions == nil {
		s.Restrictions = &model.BlockRestrictions{}
	}
	if m.Restrictions == nil {
		m.Restrictions = &model.BlockRestrictions{}
	}
	if *s.Restrictions != *m.Restrictions {
		m := &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetRestrictions{BlockSetRestrictions: &pb.EventBlockSetRestrictions{
			Id:           s.Id,
			Restrictions: m.Restrictions,
		}}}
		msgs = append(msgs, simple.EventMessage{Msg: m})
	}
	if !fieldsEq(s.Fields, m.Fields) {
		m := &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetFields{BlockSetFields: &pb.EventBlockSetFields{
			Id:     s.Id,
			Fields: m.Fields,
		}}}
		msgs = append(msgs, simple.EventMessage{Msg: m})
	}
	if s.BackgroundColor != m.BackgroundColor {
		m := &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetBackgroundColor{BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
			Id:              s.Id,
			BackgroundColor: m.BackgroundColor,
		}}}
		msgs = append(msgs, simple.EventMessage{Msg: m})
	}
	if s.Align != m.Align {
		m := &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetAlign{BlockSetAlign: &pb.EventBlockSetAlign{
			Id:    s.Id,
			Align: m.Align,
		}}}
		msgs = append(msgs, simple.EventMessage{Msg: m})
	}

	return
}

func (b *Base) Copy() simple.Block {
	return NewBase(pbtypes.CopyBlock(b.Model()))
}

func (b *Base) String() string {
	return fmt.Sprintf("%s: %T (%d)", b.Id, b.Content, len(b.ChildrenIds))
}

func stringSlicesEq(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, v := range s1 {
		if v != s2[i] {
			return false
		}
	}
	return true
}

func fieldsEq(f1, f2 *types.Struct) bool {
	if f1 == nil {
		f1 = &types.Struct{}
	}
	if f2 == nil {
		f2 = &types.Struct{}
	}
	return f1.Compare(f2) == 0
}
