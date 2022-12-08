package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type Profile struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark

	sendEvent func(e *pb.Event)
}

func NewProfile(sendEvent func(e *pb.Event)) *Profile {
	sb := smartblock.New()
	return &Profile{
		SmartBlock: sb,
		sendEvent:  sendEvent,
	}
}

func (p *Profile) Init(ctx *smartblock.InitContext) (err error) {
	p.AllOperations = basic.NewBasic(p.SmartBlock)
	p.IHistory = basic.NewHistory(p.SmartBlock)
	p.Text = stext.NewText(ctx.App, p.SmartBlock)
	p.File = file.NewFile(ctx.App, p.SmartBlock)
	p.Clipboard = clipboard.NewClipboard(ctx.App, p.SmartBlock)
	p.Bookmark = bookmark.NewBookmark(ctx.App, p.SmartBlock)

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyProfile.URL()}, model.ObjectType_profile),
		template.WithDetail(bundle.RelationKeyLayoutAlign, pbtypes.Float64(float64(model.Block_AlignCenter))),
		template.WithTitle,
		// template.WithAlignedDescription(model.Block_AlignCenter, true),
		template.WithFeaturedRelations,
		template.WithRequiredRelations(),
	)
}

func (p *Profile) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	if err = p.SmartBlock.SetDetails(ctx, details, showEvent); err != nil {
		return
	}
	p.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfAccountDetails{
					AccountDetails: &pb.EventAccountDetails{
						ProfileId: p.Id(),
						Details:   p.Details(),
					},
				},
			},
		},
	})
	return
}
