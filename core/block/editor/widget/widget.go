package widget

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
}

type widget struct {
	smartblock.SmartBlock
}

func NewWidget(sb smartblock.SmartBlock) Widget {
	return &widget{
		SmartBlock: sb,
	}
}

func (w *widget) CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error) {
	if req.Block.Content == nil {
		return "", fmt.Errorf("block has no content")
	}

	switch req.Block.Content.(type) {
	case *model.BlockContentOfLink:
		// Add block<->widget layout validation when new cases are added
	default:
		return "", fmt.Errorf("unsupported widget content: %T", req.Block.Content)
	}

	req.Block.Id = ""
	req.Block.ChildrenIds = nil
	b := simple.New(req.Block)
	if err := b.Validate(); err != nil {
		return "", fmt.Errorf("validate block: %w", err)
	}

	wrapper := simple.New(&model.Block{
		ChildrenIds: []string{
			b.Model().Id,
		},
		Content: &model.BlockContentOfWidget{
			Widget: &model.BlockContentWidget{
				Layout: req.WidgetLayout,
			},
		},
	})

	if !s.Add(b) {
		return "", fmt.Errorf("can't add block")
	}
	if !s.Add(wrapper) {
		return "", fmt.Errorf("can't add widget wrapper block")
	}
	if err := s.InsertTo(req.TargetId, req.Position, wrapper.Model().Id); err != nil {
		return "", fmt.Errorf("insert widget wrapper block: %w", err)
	}

	return wrapper.Model().Id, nil
}
