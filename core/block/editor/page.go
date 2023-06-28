package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

type Page struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark

	dataview.Dataview
	table.TableEditor

	objectStore objectstore.ObjectStore
}

func NewPage(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	fileBlockService file.BlockService,
	picker getblock.Picker,
	bookmarkService bookmark.BookmarkService,
	relationService relation.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.Service,
	eventSender event.Sender,
) *Page {
	f := file.NewFile(
		sb,
		fileBlockService,
		tempDirProvider,
		fileService,
		picker,
	)
	return &Page{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, relationService, layoutConverter),
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
			relationService,
			fileService,
		),
		Bookmark: bookmark.NewBookmark(
			sb,
			picker,
			bookmarkService,
			objectStore,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
			sbtProvider,
		),
		TableEditor: table.NewEditor(sb),
		objectStore: objectStore,
	}
}

func (p *Page) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.ObjectTypeUrls == nil && (ctx.State == nil || len(ctx.State.ObjectTypes()) == 0) {
		// todo: revise this logic
		// we can have other default type on client side
		ctx.ObjectTypeUrls = []string{bundle.TypeKeyPage.URL()}
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}

func (p *Page) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			layout, ok := ctx.State.Layout()
			if !ok {
				// nolint:errcheck
				otypes, _ := p.objectStore.GetObjectTypes(ctx.ObjectTypeUrls)
				for _, ot := range otypes {
					layout = ot.Layout
				}
			}
			if len(ctx.ObjectTypeUrls) > 0 && len(ctx.State.ObjectTypes()) == 0 {
				ctx.State.SetObjectTypes(ctx.ObjectTypeUrls)
			}
			// TODO Templates must be dumb here, no migration logic

			templates := []template.StateTransformer{
				template.WithEmpty,
				template.WithObjectTypesAndLayout(ctx.State.ObjectTypes(), layout),
				template.WithLayout(layout),
				template.WithDefaultFeaturedRelations,
				template.WithFeaturedRelations,
				template.WithRequiredRelations(),
				template.WithLinkFieldsMigration,
				template.WithCreatorRemovedFromFeaturedRelations,
			}

			switch layout {
			case model.ObjectType_note:
				templates = append(templates,
					template.WithNameToFirstBlock,
					template.WithNoTitle,
					template.WithNoDescription,
				)
			case model.ObjectType_todo:
				templates = append(templates,
					template.WithTitle,
					template.WithDescription,
					template.WithRelations([]bundle.RelationKey{bundle.RelationKeyDone}),
				)
			case model.ObjectType_bookmark:
				templates = append(templates,
					template.WithTitle,
					template.WithDescription,
					template.WithAddedFeaturedRelation(bundle.RelationKeyType),
					template.WithBookmarkBlocks,
				)
			default:
				templates = append(templates,
					template.WithTitle,
					template.WithDescription,
				)
			}

			template.InitTemplate(s, templates...)
		},
	}
}

func (p *Page) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func GetDefaultViewRelations(rels []*model.Relation) []*model.BlockContentDataviewRelation {
	var viewRels = make([]*model.BlockContentDataviewRelation, 0, len(rels))
	for _, rel := range rels {
		if rel.Hidden && rel.Key != bundle.RelationKeyName.String() {
			continue
		}
		var visible bool
		if rel.Key == bundle.RelationKeyName.String() {
			visible = true
		}
		viewRels = append(viewRels, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: visible})
	}
	return viewRels
}
