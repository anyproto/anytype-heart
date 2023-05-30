package widget

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	DefaultWidgetFavorite = "favorite"
	DefaultWidgetSet      = "set"
	DefaultWidgetRecent   = "recent"

	TreeLowLimit    int32 = 6
	TreeMiddleLimit int32 = 10
	TreeHighLimit   int32 = 14
	ListLowLimit    int32 = 4
	ListMiddleLimit int32 = 6
	ListHighLimit   int32 = 8
)

var LimitOptionsByLayout = map[model.BlockContentWidgetLayout][]int32{
	model.BlockContentWidget_Tree: {
		TreeLowLimit,
		TreeMiddleLimit,
		TreeHighLimit,
	},
	model.BlockContentWidget_List: {
		ListLowLimit,
		ListMiddleLimit,
		ListHighLimit,
	},
}

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
}

type widget struct {
	smartblock.SmartBlock
}

func IsPredefinedWidgetTargetId(targetID string) bool {
	switch targetID {
	case DefaultWidgetFavorite, DefaultWidgetSet, DefaultWidgetRecent:
		return true
	default:
		return false
	}
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
				Limit:  w.computeObjectLimit(req),
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

func (w *widget) computeObjectLimit(req *pb.RpcBlockCreateWidgetRequest) int32 {
	switch req.WidgetLayout {
	case model.BlockContentWidget_Tree, model.BlockContentWidget_CompactList:
		if lo.Contains(LimitOptionsByLayout[model.BlockContentWidget_Tree], req.ObjectLimit) {
			return req.ObjectLimit
		}
		return TreeLowLimit
	case model.BlockContentWidget_List:
		if lo.Contains(LimitOptionsByLayout[model.BlockContentWidget_List], req.ObjectLimit) {
			return req.ObjectLimit
		}
		return TreeLowLimit
	default:
		return 0
	}
}
