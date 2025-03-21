package widget

import (
	"fmt"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	DefaultWidgetFavorite   = "favorite"
	DefaultWidgetSet        = "set"
	DefaultWidgetRecent     = "recent"
	DefaultWidgetCollection = "collection"
	DefaultWidgetBin        = "bin"
	DefaultWidgetRecentOpen = "recentOpen"
)

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
	AddAutoWidget(s *state.State, targetId, blockId, viewId string, layout model.BlockContentWidgetLayout) error
}

type widget struct {
	smartblock.SmartBlock
}

type ImportWidgetFlags struct {
	ImportSet        bool
	ImportCollection bool
}

func (w *ImportWidgetFlags) IsEmpty() bool {
	return !w.ImportCollection && !w.ImportSet
}

func FillImportFlags(link *model.BlockContentLink, widgetFlags *ImportWidgetFlags) bool {
	var builtinWidget bool
	if link.TargetBlockId == DefaultWidgetSet {
		widgetFlags.ImportSet = true
		builtinWidget = true
	}
	if link.TargetBlockId == DefaultWidgetCollection {
		widgetFlags.ImportCollection = true
		builtinWidget = true
	}
	return builtinWidget
}

func IsPredefinedWidgetTargetId(targetID string) bool {
	switch targetID {
	case DefaultWidgetFavorite, DefaultWidgetSet, DefaultWidgetRecent, DefaultWidgetCollection:
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

func (w *widget) AddAutoWidget(st *state.State, targetId, widgetBlockId, viewId string, layout model.BlockContentWidgetLayout) error {
	targets := st.Details().Get(bundle.RelationKeyAutoWidgetTargets).StringList()
	if slices.Contains(targets, targetId) {
		return nil
	}
	targets = append(targets, targetId)
	st.SetDetail(bundle.RelationKeyAutoWidgetTargets, domain.StringList(targets))
	var typeBlockAlreadyExists bool

	var (
		binBlockWrapperId string
		binIsTheLast      bool
	)
	err := st.Iterate(func(b simple.Block) (isContinue bool) {
		link := b.Model().GetLink()
		if link == nil {
			return true
		}
		if link.TargetBlockId == targetId {
			// check by targetBlockId in case user created the same block manually
			typeBlockAlreadyExists = true
		}
		if link.TargetBlockId == DefaultWidgetBin {
			binBlockWrapperId = st.GetParentOf(b.Model().Id).Model().Id
			rootBlock := st.Get(st.RootId())
			if len(rootBlock.Model().GetChildrenIds()) == 0 {
				return true
			}
			if rootBlock.Model().GetChildrenIds()[len(rootBlock.Model().GetChildrenIds())-1] == binBlockWrapperId {
				binIsTheLast = true
			}
		}
		return true
	})

	if err != nil {
		return err
	}
	if typeBlockAlreadyExists {
		return nil
	}

	var (
		targetBlockId string
		position      model.BlockPosition
	)
	if binIsTheLast {
		targetBlockId = binBlockWrapperId
		position = model.Block_Top
	} else {
		targetBlockId = ""
		position = model.Block_Bottom
	}

	_, err = w.CreateBlock(st, &pb.RpcBlockCreateWidgetRequest{
		ContextId:    st.RootId(),
		ObjectLimit:  6,
		WidgetLayout: layout,
		Position:     position,
		TargetId:     targetBlockId,
		ViewId:       viewId,
		Block: &model.Block{
			Id: widgetBlockId, // hardcode id to avoid duplicates
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: targetId,
			}},
		},
	})
	return err
}

func (w *widget) CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error) {
	if req.Block.Content == nil {
		return "", fmt.Errorf("block has no content")
	}

	if req.Block.GetLink() == nil {
		return "", fmt.Errorf("unsupported widget content: %T", req.Block.Content)
	}

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
				Limit:  req.ObjectLimit,
				ViewId: req.ViewId,
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
