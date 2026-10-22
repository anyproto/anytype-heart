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

	// The chat and bin widgets are minted by the clients (anytype-ts spells
	// them through its widgetId table); the ids reach the heart only inside
	// stored widget objects, which is why no heart-side code creates them.
	DefaultWidgetChat = "chat"
	DefaultWidgetBin  = "bin"

	widgetWrapperBlockSuffix = "-wrapper" // in case blockId is specifically provided to avoid bad tree merges

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

// IsPredefinedWidgetTargetId reports whether targetID names a built-in
// listing rather than an object — the whole inventory a widget link can
// carry, not just the four this function used to know.
//
// The gap was not cosmetic. common.handleLinkBlock leaves a link target
// alone only when this function knows it; anything else it cannot resolve
// becomes addr.MissingObject, and WidgetObject.Init then strips the link and
// its now-empty wrapper. allObjects is created by WidgetObject's own
// migration 3, chat and bin by the clients — so importing an app export
// silently lost exactly those widgets, with a log line as the only trace.
// Measured over a 77-space account: 33 of 218 widget links name a listing,
// and 29 of those named one this function did not know.
func IsPredefinedWidgetTargetId(targetID string) bool {
	switch targetID {
	case DefaultWidgetFavorite, DefaultWidgetSet, DefaultWidgetRecentlyEdited, DefaultWidgetCollection,
		DefaultWidgetAll, DefaultWidgetRecentlyOpened, DefaultWidgetChat, DefaultWidgetBin:
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

// UnlinkWithWrapper unlinks the given block ids together with their parent
// widget wrapper. Removing only the inner link leaves an empty wrapper.
func UnlinkWithWrapper(st *state.State, ids ...string) {
	for _, id := range ids {
		if p := st.PickParentOf(id); p != nil && p.Model().GetWidget() != nil {
			st.Unlink(p.Model().Id)
		}
		st.Unlink(id)
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
