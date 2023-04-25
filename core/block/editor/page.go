package editor

import (
	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/migration"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

type Page struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark
	collectionService CollectionService

	dataview.Dataview
	table.TableEditor

	objectStore objectstore.ObjectStore
}

func NewPage(
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	fileBlockService file.BlockService,
	bookmarkBlockService bookmark.BlockService,
	bookmarkService bookmark.BookmarkService,
	relationService relation2.Service,
	tempDirProvider core.TempDirProvider,
	collectionService CollectionService,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
) *Page {
	sb := smartblock.New()
	f := file.NewFile(
		sb,
		fileBlockService,
		anytype,
		tempDirProvider,
	)
	return &Page{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, relationService, layoutConverter),
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
			relationService,
		),
		Bookmark: bookmark.NewBookmark(
			sb,
			bookmarkBlockService,
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
		TableEditor:       table.NewEditor(sb),
		collectionService: collectionService,
		objectStore:       objectStore,
	}
}

func (p *Page) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.ObjectTypeUrls == nil {
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
				otypes, _ := objectstore.GetObjectTypes(p.objectStore, ctx.ObjectTypeUrls)
				for _, ot := range otypes {
					layout = ot.Layout
				}
			}

			// TODO Templates must be dumb here, no migration logic

			templates := []template.StateTransformer{
				template.WithEmpty,
				template.WithObjectTypesAndLayout(ctx.ObjectTypeUrls, layout),
				bookmarksvc.WithFixedBookmarks(p.Bookmark),
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

type CollectionService interface {
	RegisterCollection(sb smartblock.SmartBlock)
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
