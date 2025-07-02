package textservice

import (
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/components"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.block.textservice"

type Service interface {
	app.Component

	SetText(parentCtx session.Context, req pb.RpcBlockTextSetTextRequest) error
	SetTextStyle(ctx session.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx session.Context, req pb.RpcBlockTextSetCheckedRequest) error
	SetTextColor(ctx session.Context, contextId string, color string, blockIds ...string) error
	ClearTextStyle(ctx session.Context, contextId string, blockIds ...string) error
	ClearTextContent(ctx session.Context, contextId string, blockIds ...string) error
	SetTextMark(ctx session.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string) error
	SetTextIcon(ctx session.Context, contextId, image, emoji string, blockIds ...string) error
}

type service struct {
	objectGetter         cache.ObjectGetter
	setTextApplyInterval time.Duration
}

func New(setTextApplyInterval time.Duration) Service {
	return &service{
		setTextApplyInterval: setTextApplyInterval,
	}
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) SetText(parentCtx session.Context, req pb.RpcBlockTextSetTextRequest) error {
	return cache.DoComponent(s.objectGetter, req.ContextId, func(sb smartblock.SmartBlock, b components.Text) error {
		f, err := components.GetComponent[components.TextFlusher](sb)
		if err != nil {
			return err
		}

		ctx := session.NewChildContext(parentCtx)
		s := f.NewSetTextState(ctx, req.BlockId, req.SelectedTextRange, s.setTextApplyInterval)

		detailsBlockChanged, mentionsChanged, err := b.SetText(s, req)
		if err != nil {
			f.CancelSetTextState()
			return err
		}

		if detailsBlockChanged {
			f.FlushSetTextState(smartblock.ApplyInfo{})
		}

		if mentionsChanged {
			f.RemoveInternalFlags(s)
			f.FlushSetTextState(smartblock.ApplyInfo{})
		}

		return nil
	})
}

func (s *service) SetTextStyle(
	ctx session.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string,
) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *service) SetTextChecked(ctx session.Context, req pb.RpcBlockTextSetCheckedRequest) error {
	return cache.DoComponent(s.objectGetter, req.ContextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.UpdateTextBlocks(ctx, []string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *service) SetTextColor(ctx session.Context, contextId string, color string, blockIds ...string) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *service) ClearTextStyle(ctx session.Context, contextId string, blockIds ...string) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.Model().BackgroundColor = ""
			t.Model().Align = model.Block_AlignLeft
			t.Model().VerticalAlign = model.Block_VerticalAlignTop
			t.SetTextColor("")
			t.SetStyle(model.BlockContentText_Paragraph)

			marks := t.Model().GetText().Marks.Marks[:0]
			for _, m := range t.Model().GetText().Marks.Marks {
				switch m.Type {
				case model.BlockContentTextMark_Strikethrough,
					model.BlockContentTextMark_Keyboard,
					model.BlockContentTextMark_Italic,
					model.BlockContentTextMark_Bold,
					model.BlockContentTextMark_Underscored,
					model.BlockContentTextMark_TextColor,
					model.BlockContentTextMark_BackgroundColor:
				default:
					marks = append(marks, m)
				}
			}
			t.Model().GetText().Marks.Marks = marks

			return nil
		})
	})
}

func (s *service) ClearTextContent(ctx session.Context, contextId string, blockIds ...string) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetText("", nil)
			return nil
		})
	})
}

func (s *service) SetTextMark(
	ctx session.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string,
) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.SetMark(ctx, mark, blockIds...)
	})
}

func (s *service) SetTextIcon(ctx session.Context, contextId, image, emoji string, blockIds ...string) error {
	return cache.DoComponent(s.objectGetter, contextId, func(sb smartblock.SmartBlock, b components.Text) error {
		return b.SetIcon(ctx, image, emoji, blockIds...)
	})
}
