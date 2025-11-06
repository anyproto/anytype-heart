package widget

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	DefaultWidgetFavorite       = "favorite"
	DefaultWidgetSet            = "set"
	DefaultWidgetRecentlyEdited = "recent"
	DefaultWidgetCollection     = "collection"

	DefaultWidgetAll            = "allObjects"
	DefaultWidgetRecentlyOpened = "recentOpen"
	widgetWrapperBlockSuffix    = "-wrapper" // in case blockId is specifically provided to avoid bad tree merges


)

var ErrWidgetAlreadyExists = fmt.Errorf("widget with specified id already exists")

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
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
	case DefaultWidgetFavorite, DefaultWidgetSet, DefaultWidgetRecentlyEdited, DefaultWidgetCollection:
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
	return w.createBlock(s, req, false)
}

func (w *widget) createBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest, isAutoAdded bool) (string, error) {
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

	var wrapperBlockId string
	if b.Model().Id != "" {
		if s.Pick(b.Model().Id) != nil {
			return "", ErrWidgetAlreadyExists
		}
		// if caller provide explicit blockId, we need to make the wrapper blockId stable as well.
		// otherwise, in case of multiple devices applied this change in parallel, we can have empty wrapper blocks
		wrapperBlockId = b.Model().Id + widgetWrapperBlockSuffix
	}

	wrapper := simple.New(&model.Block{
		Id: wrapperBlockId,
		ChildrenIds: []string{
			b.Model().Id,
		},
		Content: &model.BlockContentOfWidget{
			Widget: &model.BlockContentWidget{
				Layout:    req.WidgetLayout,
				Limit:     req.ObjectLimit,
				ViewId:    req.ViewId,
				AutoAdded: isAutoAdded,
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
