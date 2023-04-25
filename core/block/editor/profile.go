package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
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
	table.TableEditor

	sendEvent func(e *pb.Event)
}

func NewProfile(
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	fileBlockService file.BlockService,
	bookmarkBlockService bookmark.BlockService,
	bookmarkService bookmark.BookmarkService,
	sendEvent func(e *pb.Event),
	tempDirProvider core.TempDirProvider,
) *Profile {
	sb := smartblock.New()
	f := file.NewFile(
		sb,
		fileBlockService,
		anytype,
		tempDirProvider,
	)
	return &Profile{
		SmartBlock:    sb,
		sendEvent:     sendEvent,
		AllOperations: basic.NewBasic(sb),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
		),
		File: f,
		Clipboard: clipboard.NewClipboard(
			sb,
			f,
			anytype,
			tempDirProvider,
		),
		Bookmark: bookmark.NewBookmark(
			sb,
			bookmarkBlockService,
			bookmarkService,
			objectStore,
		),
		TableEditor: table.NewEditor(sb),
	}
}

func (p *Profile) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyProfile.URL()}, model.ObjectType_profile),
		template.WithDetail(bundle.RelationKeyLayoutAlign, pbtypes.Float64(float64(model.Block_AlignCenter))),
		template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true)),
		template.WithTitle,
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
