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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

	objectStore       objectstore.ObjectStore
	fileObjectService fileobject.Service
	objectDeleter     ObjectDeleter
}

func (f *ObjectFactory) newPage(sb smartblock.SmartBlock) *Page {
	fileComponent := file.NewFile(sb, f.fileBlockService, f.picker, f.processService, f.fileUploaderService)
	return &Page{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basic.NewBasic(sb, f.objectStore, f.layoutConverter, f.fileObjectService, f.lastUsedUpdater),
		IHistory:       basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			f.objectStore,
			f.eventSender,
		),
		File: fileComponent,
		Clipboard: clipboard.NewClipboard(
			sb,
			fileComponent,
			f.tempDirProvider,
			f.objectStore,
			f.fileService,
			f.fileObjectService,
		),
		Bookmark:          bookmark.NewBookmark(sb, f.bookmarkService),
		Dataview:          dataview.NewDataview(sb, f.objectStore),
		TableEditor:       table.NewEditor(sb),
		objectStore:       f.objectStore,
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
			err = p.deleteRelationOptions(p.SpaceID(), pbtypes.GetString(p.Details(), bundle.RelationKeyRelationKey.String()))
			if err != nil {
				log.With("err", err).Error("failed to delete relation options")
			}
		}()
	}
	return nil
}

func (p *Page) isRelationDeleted(ctx *smartblock.InitContext) bool {
	return p.Type() == coresb.SmartBlockTypeRelation &&
		pbtypes.GetBool(ctx.State.Details(), bundle.RelationKeyIsUninstalled.String())
}

func (p *Page) deleteRelationOptions(spaceID string, relationKey string) error {
	relationOptions, _, err := p.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
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
						otype, err := p.objectStore.GetObjectByUniqueKey(p.SpaceID(), uk)
						if err != nil {
							log.Errorf("failed to get object by unique key: %v", err)
						} else {
							layout = model.ObjectTypeLayout(pbtypes.GetInt64(otype.Details, bundle.RelationKeyRecommendedLayout.String()))
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
				// TODO case for relationOption?
			case model.ObjectType_tag:
				templates = append(templates,
					template.WithTitle,
					template.WithNoDescription,
					template.WithRelations([]domain.RelationKey{bundle.RelationKeyRelationOptionColor}))
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
