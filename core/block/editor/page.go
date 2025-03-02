package editor

import (
	"slices"

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
	"github.com/anyproto/anytype-heart/core/relationutils"
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
	bundle.RelationKeyMentions,
	bundle.RelationKeyLayoutAlign,
}

const objectTypeAllViewId = "all"
var typeAndRelationRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyUniqueKey,
	bundle.RelationKeyIsReadonly,
	bundle.RelationKeySourceObject,
	bundle.RelationKeyLastUsedDate,
	bundle.RelationKeyRevision,
	bundle.RelationKeyIsHidden,
}

var typeRequiredRelations = append(typeAndRelationRequiredRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
	bundle.RelationKeyRecommendedFileRelations,
	bundle.RelationKeyRecommendedLayout,
	bundle.RelationKeySmartblockTypes,
	bundle.RelationKeyIconOption,
	bundle.RelationKeyIconName,
)

var relationRequiredRelations = append(typeAndRelationRequiredRelations,
	bundle.RelationKeyRelationFormat,
	bundle.RelationKeyRelationFormatObjectTypes,
	bundle.RelationKeyRelationKey,
)

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
		AllOperations:  basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
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
	appendRequiredInternalRelations(ctx)
	if ctx.ObjectTypeKeys == nil && (ctx.State == nil || len(ctx.State.ObjectTypeKeys()) == 0) && ctx.IsNewObject {
		ctx.ObjectTypeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	if !ctx.IsNewObject {
		migrateFilesToObjects(p, p.fileObjectService)(ctx.State)
	}

	p.EnableLayouts()
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

func appendRequiredInternalRelations(ctx *smartblock.InitContext) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, pageRequiredRelations...)
	if len(ctx.ObjectTypeKeys) != 1 {
		return
	}
	switch ctx.ObjectTypeKeys[0] {
	case bundle.TypeKeyObjectType:
		ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, typeRequiredRelations...)
	case bundle.TypeKeyRelation:
		ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, relationRequiredRelations...)
	}
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
				RelationKey: bundle.RelationKeyResolvedLayout,
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
		Version: 4,
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
							layout = model.ObjectTypeLayout(otype.GetInt64(bundle.RelationKeyRecommendedLayout))
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
				template.WithObjectTypes(ctx.State.ObjectTypeKeys()),
				template.WithFeaturedRelationsBlock,
				template.WithLinkFieldsMigration,
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
					template.WithBookmarkBlocks,
				)
			case model.ObjectType_relation:
				templates = append(templates,
					template.WithTitle,
					template.WithLayout(layout),
				)
			case model.ObjectType_objectType:
				templates = append(templates,
					template.WithTitle,
					template.WithLayout(layout),
				)
				templates = append(templates, p.getObjectTypeTemplates()...)
			case model.ObjectType_chat:
				templates = append(templates,
					template.WithTitle,
					template.WithBlockChat,
					template.WithLayout(layout),
				)
			case model.ObjectType_chatDerived:
				templates = append(templates,
					template.WithTitle,
					template.WithBlockChat,
					template.WithLayout(layout),
				)
				// TODO case for relationOption?
			case model.ObjectType_tag:
				templates = append(templates,
					template.WithTitle,
					template.WithNoDescription,
					template.WithLayout(layout),
				)
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
	migrations := []migration.Migration{
		{
			Version: 2,
			Proc:    func(s *state.State) {},
		},
		{
			Version: 3,
			Proc:    p.featuredRelationsMigration,
		},
	}

	// migration 3 is skipped
	// migration 4 is applied only for ObjectType
	if p.ObjectTypeKey() == bundle.TypeKeyObjectType {
		migrations = append(migrations,
			migration.Migration{
				Version: 4,
				Proc: func(s *state.State) {
					template.InitTemplate(s, p.getObjectTypeTemplates()...)
				},
			})
	}

	return migration.MakeMigrations(migrations)
}

func (p *Page) getObjectTypeTemplates() []template.StateTransformer {
	details := p.Details()
	name := details.GetString(bundle.RelationKeyName)
	key := details.GetString(bundle.RelationKeyUniqueKey)

	dvContent := template.MakeDataviewContent(false, &model.ObjectType{
		Url:  p.Id(),
		Name: name,
		// todo: add RelationLinks, because they are not indexed at this moment :(
		Key: key,
	}, []*model.RelationLink{
		{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_longtext,
		},
	}, objectTypeAllViewId)

	dvContent.Dataview.TargetObjectId = p.Id()
	return []template.StateTransformer{
		template.WithDataviewID(state.DataviewBlockID, dvContent, false),
		template.WithForcedDetail(bundle.RelationKeySetOf, domain.StringList([]string{p.Id()})),
	}
}

func (p *Page) featuredRelationsMigration(s *state.State) {
	if p.Type() != coresb.SmartBlockTypeObjectType {
		return
	}

	if s.HasRelation(bundle.RelationKeyRecommendedFeaturedRelations.String()) {
		return
	}

	featuredRelationKeys := relationutils.DefaultFeaturedRelationKeys()
	featuredRelationIds := make([]string, 0, len(featuredRelationKeys))
	for _, key := range featuredRelationKeys {
		id, err := p.Space().DeriveObjectID(nil, domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
		if err != nil {
			log.Errorf("failed to derive object id: %v", err)
			continue
		}
		featuredRelationIds = append(featuredRelationIds, id)
	}

	if len(featuredRelationIds) == 0 {
		return
	}

	s.SetDetail(bundle.RelationKeyRecommendedFeaturedRelations, domain.StringList(featuredRelationIds))

	recommendedRelations := s.Details().GetStringList(bundle.RelationKeyRecommendedRelations)
	oldLen := len(recommendedRelations)
	recommendedRelations = slices.DeleteFunc(recommendedRelations, func(s string) bool {
		return slices.Contains(featuredRelationIds, s)
	})

	if oldLen == len(recommendedRelations) {
		return
	}

	s.SetDetail(bundle.RelationKeyRecommendedRelations, domain.StringList(recommendedRelations))
}
