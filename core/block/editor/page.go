package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var pageRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyCoverId,
	bundle.RelationKeyCoverScale,
	bundle.RelationKeyCoverType,
	bundle.RelationKeyCoverX,
	bundle.RelationKeyCoverY,
	bundle.RelationKeySnippet,
	bundle.RelationKeyFeaturedRelations,
	bundle.RelationKeyLinks,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyLayoutAlign,
}

type Page struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	file.File
	stext.Text
	clipboard.Clipboard
	bookmark.Bookmark
	source.ChangeReceiver

	dataview.Dataview
	table.TableEditor

	objectStore       spaceindex.Store
	fileObjectService fileobject.Service
	objectDeleter     ObjectDeleter
}

func (f *ObjectFactory) newPage(spaceId string, sb smartblock.SmartBlock) *Page {
	store := f.objectStore.SpaceIndex(spaceId)
	fileComponent := file.NewFile(sb, f.fileBlockService, f.picker, f.processService, f.fileUploaderService)
	return &Page{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService, f.lastUsedUpdater),
		IHistory:       basic.NewHistory(sb),
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
		Dataview:          dataview.NewDataview(sb, store),
		TableEditor:       table.NewEditor(sb),
		objectStore:       store,
		fileObjectService: f.fileObjectService,
		objectDeleter:     f.objectDeleter,
	}
}

func (p *Page) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, pageRequiredRelations...)
	if ctx.ObjectTypeKeys == nil && (ctx.State == nil || len(ctx.State.ObjectTypeKeys()) == 0) && ctx.IsNewObject {
		ctx.ObjectTypeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	if !ctx.IsNewObject {
		migrateFilesToObjects(p, p.fileObjectService)(ctx.State)
	}

	if p.isRelationDeleted(ctx) {
		// todo: move this to separate component
		go func() {
			err = p.deleteRelationOptions(p.SpaceID(), p.Details().GetString(bundle.RelationKeyRelationKey))
			if err != nil {
				log.With("err", err).Error("failed to delete relation options")
			}
		}()
	}
	return nil
}

func (p *Page) isRelationDeleted(ctx *smartblock.InitContext) bool {
	return p.Type() == coresb.SmartBlockTypeRelation &&
		ctx.State.Details().GetBool(bundle.RelationKeyIsUninstalled)
}

func (p *Page) deleteRelationOptions(spaceID string, relationKey string) error {
	relationOptions, _, err := p.objectStore.QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
		},
	})
	if err != nil {
		return err
	}

	for _, id := range relationOptions {
		err := p.objectDeleter.DeleteObjectByFullID(domain.FullID{SpaceID: spaceID, ObjectID: id})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Page) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 2,
		Proc: func(s *state.State) {
			layout, ok := ctx.State.Layout()
			if !ok {
				// nolint:errcheck
				if len(ctx.ObjectTypeKeys) > 0 {
					lastTypeKey := ctx.ObjectTypeKeys[len(ctx.ObjectTypeKeys)-1]
					uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, string(lastTypeKey))
					if err != nil {
						log.Errorf("failed to create unique key: %v", err)
					} else {
						otype, err := p.objectStore.GetObjectByUniqueKey(uk)
						if err != nil {
							log.Errorf("failed to get object by unique key: %v", err)
						} else {
							layout = model.ObjectTypeLayout(int32(otype.GetInt64(bundle.RelationKeyRecommendedLayout)))
						}
					}
				}
			}
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}
			// TODO Templates must be dumb here, no migration logic

			templates := []template.StateTransformer{
				template.WithEmpty,
				template.WithObjectTypesAndLayout(ctx.State.ObjectTypeKeys(), layout),
				template.WithLayout(layout),
				template.WithDefaultFeaturedRelations,
				template.WithFeaturedRelations,
				template.WithLinkFieldsMigration,
				template.WithCreatorRemovedFromFeaturedRelations,
			}

			switch layout {
			case model.ObjectType_note:
				templates = append(templates,
					template.WithNameToFirstBlock,
					template.WithFirstTextBlock,
					template.WithNoTitle,
				)
			case model.ObjectType_todo:
				templates = append(templates,
					template.WithTitle,
					template.WithRelations([]domain.RelationKey{bundle.RelationKeyDone}),
				)
			case model.ObjectType_bookmark:
				templates = append(templates,
					template.WithTitle,
					template.WithDescription,
					template.WithAddedFeaturedRelation(bundle.RelationKeyType),
					template.WithAddedFeaturedRelation(bundle.RelationKeyBacklinks),
					template.WithBookmarkBlocks,
				)
			case model.ObjectType_relation:
				templates = append(templates,
					template.WithTitle,
					template.WithAddedFeaturedRelation(bundle.RelationKeyType),
				)
			case model.ObjectType_objectType:
				templates = append(templates,
					template.WithTitle,
					template.WithAddedFeaturedRelation(bundle.RelationKeyType),
				)
			case model.ObjectType_chat:
				templates = append(templates,
					template.WithTitle,
					template.WithBlockChat,
				)
			case model.ObjectType_chatDerived:
				templates = append(templates,
					template.WithTitle,
					template.WithBlockChat,
				)
				// TODO case for relationOption?
			case model.ObjectType_tag:
				templates = append(templates,
					template.WithTitle,
					template.WithNoDescription)
			default:
				templates = append(templates,
					template.WithTitle,
				)
			}

			template.InitTemplate(s, templates...)
		},
	}
}

func (p *Page) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc:    template.WithAddedFeaturedRelation(bundle.RelationKeyBacklinks),
		},
	})
}
