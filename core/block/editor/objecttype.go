package editor

import (
	"context"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/layout"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var typeRequiredRelations = append(typeAndRelationRequiredRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
	bundle.RelationKeyRecommendedFileRelations,
	bundle.RelationKeyRecommendedLayout,
	bundle.RelationKeySmartblockTypes,
	bundle.RelationKeyIconOption,
	bundle.RelationKeyIconName,
	bundle.RelationKeyPluralName,
	bundle.RelationKeyHeaderRelationsLayout,
)

type ObjectType struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	stext.Text
	clipboard.Clipboard
	source.ChangeReceiver
	dataview.Dataview

	spaceIndex spaceindex.Store
}

func (f *ObjectFactory) newObjectType(spaceId string, sb smartblock.SmartBlock) *ObjectType {
	store := f.objectStore.SpaceIndex(spaceId)
	fileComponent := file.NewFile(sb, f.fileBlockService, f.picker, f.processService, f.fileUploaderService)
	return &ObjectType{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
		IHistory:       basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			store,
			f.eventSender,
		),
		Clipboard: clipboard.NewClipboard(
			sb,
			fileComponent,
			f.tempDirProvider,
			store,
			f.fileService,
			f.fileObjectService,
		),
		Dataview: dataview.NewDataview(sb, store),

		spaceIndex: store,
	}
}

func (ot *ObjectType) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, typeRequiredRelations...)

	if err = ot.SmartBlock.Init(ctx); err != nil {
		return
	}

	ot.AddHook(ot.syncLayoutForObjectsAndTemplates, smartblock.HookAfterApply)
	return nil
}

func (ot *ObjectType) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 5,
		Proc: func(s *state.State) {
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}

			templates := []template.StateTransformer{
				template.WithEmpty,
				template.WithObjectTypes(ctx.State.ObjectTypeKeys()),
				template.WithTitle,
				template.WithLayout(model.ObjectType_objectType),
				template.WithDetail(bundle.RelationKeyRecommendedLayout, domain.Int64(model.ObjectType_basic)),
			}
			templates = append(templates, ot.dataviewTemplates()...)

			template.InitTemplate(s, templates...)
		},
	}
}

func (ot *ObjectType) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc:    func(s *state.State) {},
		},
		{
			Version: 3,
			Proc:    ot.featuredRelationsMigration,
		},
		{
			Version: 4,
			Proc: func(s *state.State) {
				template.InitTemplate(s, ot.dataviewTemplates()...)
			},
		},
		{
			Version: 5,
			Proc:    removeDescriptionMigration,
		},
	})
}

func (ot *ObjectType) featuredRelationsMigration(s *state.State) {
	if ot.Type() != coresb.SmartBlockTypeObjectType {
		return
	}

	if s.HasRelation(bundle.RelationKeyRecommendedFeaturedRelations.String()) {
		return
	}

	typeKey := domain.TypeKey(s.UniqueKeyInternal())
	featuredRelationKeys := relationutils.DefaultFeaturedRelationKeys(typeKey)
	featuredRelationIds := make([]string, 0, len(featuredRelationKeys))

	for _, key := range featuredRelationKeys {
		id, err := ot.Space().DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
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

func removeDescriptionMigration(s *state.State) {
	uk := s.UniqueKeyInternal()
	if uk == "" {
		return
	}

	// we should delete description value only for bundled object types
	if !bundle.HasObjectTypeByKey(domain.TypeKey(uk)) {
		return
	}

	if s.Details().GetString(bundle.RelationKeyDescription) == "" {
		return
	}

	s.RemoveDetail(bundle.RelationKeyDescription)
}

func (ot *ObjectType) syncLayoutForObjectsAndTemplates(info smartblock.ApplyInfo) error {
	syncer := layout.NewSyncer(ot.Id(), ot.Space(), ot.spaceIndex)
	newLayout := layout.NewLayoutStateFromEvents(info.Events)
	oldLayout := layout.NewLayoutStateFromDetails(info.ParentDetails)
	return syncer.SyncLayoutWithType(oldLayout, newLayout, false, info.ApplyOtherObjects, true)
}

func (ot *ObjectType) dataviewTemplates() []template.StateTransformer {
	return []template.StateTransformer{
		func(s *state.State) {
			if s.Exists(state.DataviewBlockID) {
				return
			}
			details := s.Details()
			name := details.GetString(bundle.RelationKeyName)
			key := details.GetString(bundle.RelationKeyUniqueKey)

			// Build relation links from recommended and featured relations
			relationLinks := []*model.RelationLink{
				{
					Key:    bundle.RelationKeyName.String(),
					Format: model.RelationFormat_longtext,
				},
			}

			// Add featured relations
			featuredRelations := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
			for _, relId := range featuredRelations {
				// Get relation format from space index
				if rel, err := ot.spaceIndex.GetRelationById(relId); err == nil && rel != nil {
					relationLinks = append(relationLinks, &model.RelationLink{
						Key:    rel.Key,
						Format: rel.Format,
					})
				}
			}

			// Add recommended relations
			recommendedRelations := details.GetStringList(bundle.RelationKeyRecommendedRelations)
			for _, relId := range recommendedRelations {
				// Get relation format from space index
				if rel, err := ot.spaceIndex.GetRelationById(relId); err == nil && rel != nil {
					relationLinks = append(relationLinks, &model.RelationLink{
						Key:    rel.Key,
						Format: rel.Format,
					})
				}
			}

			relationLinks = slices.DeleteFunc(relationLinks, func(rel *model.RelationLink) bool {
				return rel.Key == bundle.RelationKeyType.String()
			})

			dvContent := template.MakeDataviewContent(false, &model.ObjectType{
				Url:           ot.Id(),
				Name:          name,
				Key:           key,
				RelationLinks: relationLinks,
			}, relationLinks, addr.ObjectTypeAllViewId)

			dvContent.Dataview.TargetObjectId = ot.Id()

			template.WithDataviewIDIfNotExists(state.DataviewBlockID, dvContent, false)(s)
		},
		template.WithForcedDetail(bundle.RelationKeySetOf, domain.StringList([]string{ot.Id()})),
	}
}
