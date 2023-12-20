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
	DefaultWidgetFavorite   = "favorite"
	DefaultWidgetSet        = "set"
	DefaultWidgetRecent     = "recent"
	DefaultWidgetCollection = "collection"
)

type Widget interface {
	CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error)
}

type accountService interface {
	PersonalSpaceID() string
	IdentityObjectId() string
}

type widget struct {
	smartblock.SmartBlock
	accountService accountService
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

func NewWidget(sb smartblock.SmartBlock, accountService accountService) Widget {
	return &widget{
		SmartBlock:     sb,
		accountService: accountService,
	}
}

func (w *widget) CreateBlock(s *state.State, req *pb.RpcBlockCreateWidgetRequest) (string, error) {
	if req.Block.Content == nil {
		return "", fmt.Errorf("block has no content")
	}

	if l, ok := req.Block.GetContent().(*model.BlockContentOfLink); ok {
		// substitute identity object with profile object as links are treated differently in personal and private spaces
		if l.Link.TargetBlockId == w.accountService.IdentityObjectId() && w.Space().Id() == w.accountService.PersonalSpaceID() {
			l.Link.TargetBlockId = w.Space().DerivedIDs().Profile
		}
	} else {
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
