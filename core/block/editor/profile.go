package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

	eventSender event.Sender
}

func NewProfile(sb smartblock.SmartBlock, objectStore objectstore.ObjectStore, fileBlockService file.BlockService, picker getblock.ObjectGetter, bookmarkService bookmark.BookmarkService, tempDirProvider core.TempDirProvider, layoutConverter converter.LayoutConverter, fileService files.Service, eventSender event.Sender) *Profile {
	f := file.NewFile(sb, fileBlockService, tempDirProvider, fileService, picker)
	return &Profile{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, layoutConverter),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
			eventSender,
		),
		File: f,
		Clipboard: clipboard.NewClipboard(
			sb,
			f,
			tempDirProvider,
			objectStore,
			fileService,
		),
		Bookmark:    bookmark.NewBookmark(sb, bookmarkService, objectStore),
		TableEditor: table.NewEditor(sb),
		eventSender: eventSender,
	}
}

func (p *Profile) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	return nil
}

func (p *Profile) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 3,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithObjectTypesAndLayout([]domain.TypeKey{bundle.TypeKeyProfile}, model.ObjectType_profile),
				template.WithDetail(bundle.RelationKeyLayoutAlign, pbtypes.Float64(float64(model.Block_AlignCenter))),
				template.WithTitle,
				template.WithFeaturedRelations,
				template.WithRequiredRelations(),
				migrationWithIdentityBlock,
				migrationSetHidden,
			)
		},
	}
}

func migrationSetHidden(st *state.State) {
	st.SetDetail(bundle.RelationKeyIsHidden.String(), pbtypes.Bool(true))
}

func migrationWithIdentityBlock(st *state.State) {
	blockId := "identity"
	st.Set(simple.New(&model.Block{
		Id: blockId,
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: bundle.RelationKeyProfileOwnerIdentity.String(),
			},
		},
		Restrictions: &model.BlockRestrictions{
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
	}))

	st.InsertTo(state.TitleBlockID, model.Block_Bottom, blockId)
}

func (p *Profile) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc:    migrationWithIdentityBlock,
		},
		{
			Version: 3,
			Proc:    migrationSetHidden,
		},
	})
}

func (p *Profile) SetDetails(ctx session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	if err = p.AllOperations.SetDetails(ctx, details, showEvent); err != nil {
		return
	}

	p.eventSender.Broadcast(&pb.Event{
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
