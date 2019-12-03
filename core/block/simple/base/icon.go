package base

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/mohae/deepcopy"
)

func NewIcon(m *model.Block) simple.Block {
	return &Icon{
		Base: NewBase(m).(*Base),
	}
}

type IconBlock interface {
	simple.Block
	SetIconName(name string) error
}

type Icon struct {
	*Base
}

func (i *Icon) SetIconName(name string) error {
	i.Model().GetIcon().Name = name
	return nil
}

func (i *Icon) Diff(block simple.Block) (msgs []*pb.EventMessage, err error) {
	if block.Model().GetIcon() == nil {
		return nil, fmt.Errorf("can't diff icon with %T", block.Model().Content)
	}
	if msgs, err = i.Base.Diff(block); err != nil {
		return
	}
	if newName := block.Model().GetIcon().Name; newName != i.GetIcon().Name {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockSetIcon{
				BlockSetIcon: &pb.EventBlockSetIcon{
					Id:   i.Model().Id,
					Name: &pb.EventBlockSetIconName{Value: newName},
				},
			},
		})
	}
	return
}

func (i *Icon) Copy() simple.Block {
	return NewIcon(deepcopy.Copy(i.Model()).(*model.Block))
}
