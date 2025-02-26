package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

	eventSender       event.Sender
	fileObjectService fileobject.Service
}

func (f *ObjectFactory) newProfile(spaceId string, sb smartblock.SmartBlock) *Profile {
	store := f.objectStore.SpaceIndex(spaceId)
	fileComponent := file.NewFile(sb, f.fileBlockService, f.picker, f.processService, f.fileUploaderService)
	return &Profile{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			store,
			f.eventSender,
		),
		File: fileComponent,
		Clipboard: clipboard.NewClipboard(
			sb,
			fileComponent,
			f.tempDirProvider,
			store,
			f.fileService,
			f.fileObjectService,
		),
		Bookmark:          bookmark.NewBookmark(sb, f.bookmarkService),
		TableEditor:       table.NewEditor(sb),
		eventSender:       f.eventSender,
		fileObjectService: f.fileObjectService,
	}
}

func (p *Profile) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	if !ctx.IsNewObject {
		migrateFilesToObjects(p, p.fileObjectService)(ctx.State)
	}

	return nil
}

func (p *Profile) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 4,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyProfile}),
				template.WithLayout(model.ObjectType_profile),
				template.WithDetail(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter)),
				migrationSetHidden,
			)
		},
	}
}

func migrationSetHidden(st *state.State) {
	st.SetDetail(bundle.RelationKeyIsHidden, domain.Bool(true))
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
		{
			Version: 4,
			Proc: func(s *state.State) {
			},
		},
	})
}

func (p *Profile) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
	if err = p.AllOperations.SetDetails(ctx, details, showEvent); err != nil {
		return
	}

	p.eventSender.Broadcast(event.NewEventSingleMessage(p.SpaceID(), &pb.EventMessageValueOfAccountDetails{
		AccountDetails: &pb.EventAccountDetails{
			ProfileId: p.Id(),
			Details:   p.Details().ToProto(),
		},
	}))
	return
}
