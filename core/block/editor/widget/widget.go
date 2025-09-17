package widget

import (
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	DefaultWidgetFavorite       = "favorite"
	DefaultWidgetSet            = "set"
	DefaultWidgetRecentlyEdited = "recent"
	DefaultWidgetCollection     = "collection"
	DefaultWidgetBin            = "bin"
	DefaultWidgetChat           = "chat"
	DefaultWidgetAll            = "allObjects"
	DefaultWidgetRecentlyOpened = "recentOpen"
	widgetWrapperBlockSuffix    = "-wrapper" // in case blockId is specifically provided to avoid bad tree merges

	DefaultWidgetFavoriteEventName = "Favorite"
	DefaultWidgetBinEventName      = "Bin"
)

var ErrWidgetAlreadyExists = fmt.Errorf("widget with specified id already exists")

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
	// AddAutoWidget adds a widget block. If widget with the same targetId was installed/removed before, it will not be added again.
	// blockId is optional and used to protect from multi-device conflicts.
	// if eventName is empty no event is produced
	AddAutoWidget(s *state.State, targetId, blockId, viewId string, layout model.BlockContentWidgetLayout, eventName string) error
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

func (w *widget) AddAutoWidget(st *state.State, targetId, widgetBlockId, viewId string, layout model.BlockContentWidgetLayout, eventName string) error {
	isDisabled := st.Details().Get(bundle.RelationKeyAutoWidgetDisabled).Bool()
	if isDisabled {
		return nil
	}
	targets := st.Details().Get(bundle.RelationKeyAutoWidgetTargets).StringList()
	if slices.Contains(targets, targetId) {
		return nil
	}
	targets = append(targets, targetId)
	st.SetDetail(bundle.RelationKeyAutoWidgetTargets, domain.StringList(targets))

	targetBlockId, position, err := calculateTargetAndPosition(st, targetId)
	if err != nil {
		if errors.Is(err, ErrWidgetAlreadyExists) {
			return nil
		}
		return err
	}

	_, err = w.createBlock(st, &pb.RpcBlockCreateWidgetRequest{
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
	}, true)
	if err != nil {
		return err
	}

	if eventName != "" {
		msg := event.NewMessage(w.SpaceID(), &pb.EventMessageValueOfSpaceAutoWidgetAdded{
			SpaceAutoWidgetAdded: &pb.EventSpaceAutoWidgetAdded{
				TargetId:      targetId,
				TargetName:    eventName,
				WidgetBlockId: widgetBlockId,
			},
		})
		w.SendEvent([]*pb.EventMessage{msg})
	}

	return nil
}

func calculateTargetAndPosition(st *state.State, targetId string) (string, model.BlockPosition, error) {
	if targetId == DefaultWidgetFavorite {
		rootBlock := st.Get(st.RootId())
		rootChildren := rootBlock.Model().ChildrenIds
		if len(rootChildren) == 0 {
			return "", model.Block_Bottom, nil
		}
		return rootChildren[0], model.Block_Top, nil
	}

	var (
		binBlockWrapperId      string
		binIsTheLast           bool
		typeBlockAlreadyExists bool
	)
	err := st.Iterate(func(b simple.Block) (isContinue bool) {
		link := b.Model().GetLink()
		if link == nil {
			return true
		}
		if link.TargetBlockId == targetId {
			// check by targetBlockId in case user created the same block manually
			typeBlockAlreadyExists = true
			return false
		}
		if link.TargetBlockId == DefaultWidgetBin {
			binBlockWrapperId = st.GetParentOf(b.Model().Id).Model().Id
			rootBlock := st.Get(st.RootId())
			if len(rootBlock.Model().GetChildrenIds()) == 0 {
				return true
			}
			if rootBlock.Model().GetChildrenIds()[len(rootBlock.Model().GetChildrenIds())-1] == binBlockWrapperId {
				binIsTheLast = true
				return false
			}
		}
		return true
	})

	if err != nil {
		return "", 0, err
	}
	if typeBlockAlreadyExists {
		return "", 0, ErrWidgetAlreadyExists
	}
	if binIsTheLast {
		return binBlockWrapperId, model.Block_Top, nil
	}
	return "", model.Block_Bottom, nil
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
