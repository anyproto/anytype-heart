package editor

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	bundle.RelationKeySingularName,
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
		Version: 2,
		Proc: func(s *state.State) {
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}

			templates := []template.StateTransformer{
				template.WithEmpty,
				template.WithObjectTypes(ctx.State.ObjectTypeKeys()),
				template.WithTitle,
				template.WithLayout(model.ObjectType_objectType),
			}

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
	})
}

func (ot *ObjectType) featuredRelationsMigration(s *state.State) {
	if ot.Type() != coresb.SmartBlockTypeObjectType {
		return
	}

	if s.HasRelation(bundle.RelationKeyRecommendedFeaturedRelations.String()) {
		return
	}

	var typeKey domain.TypeKey
	if uk, err := domain.UnmarshalUniqueKey(s.Details().GetString(bundle.RelationKeyUniqueKey)); err == nil {
		typeKey = domain.TypeKey(uk.InternalKey())
	}

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

type layoutState struct {
	layout            int64
	layoutAlign       int64
	featuredRelations []string

	isLayoutSet            bool
	isLayoutAlignSet       bool
	isFeaturedRelationsSet bool
}

func (ls layoutState) isAllSet() bool {
	return ls.isLayoutSet && ls.isLayoutAlignSet && ls.isFeaturedRelationsSet
}

func (ls layoutState) isAnySet() bool {
	return ls.isLayoutSet || ls.isLayoutAlignSet || ls.isFeaturedRelationsSet
}

type relationIdDeriver struct {
	space smartblock.Space
	cache map[domain.RelationKey]string
}

func (d *relationIdDeriver) deriveId(key domain.RelationKey) (string, error) {
	if d.cache != nil {
		if id, found := d.cache[key]; found {
			return id, nil
		}
	}

	id, err := d.space.DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
	if err != nil {
		return "", fmt.Errorf("failed to derive relation id: %w", err)
	}

	if d.cache == nil {
		d.cache = map[domain.RelationKey]string{}
	}
	d.cache[key] = id
	return id, nil
}

func (ot *ObjectType) syncLayoutForObjectsAndTemplates(info smartblock.ApplyInfo) error {
	newLayout := getLayoutStateFromMessages(info.Events)
	if newLayout.isLayoutSet && !isLayoutChangeApplicable(newLayout.layout) {
		// if layout change is not applicable, then it is init of some system type. Objects' layout should not be modified
		newLayout.isLayoutSet = false
	}

	if !newLayout.isAnySet() {
		// layout details were not changed
		return nil
	}

	oldLayout := getLayoutStateFromParent(info.ParentState)

	records, err := ot.queryObjectsAndTemplates()
	if err != nil {
		return err
	}

	var (
		resultErr error
		deriver   = relationIdDeriver{space: ot.Space()}
	)

	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}

		changes := collectRelationsChanges(record.Details, newLayout, oldLayout, deriver)
		if len(changes.relationsToRemove) > 0 || changes.isFeaturedRelationsChanged {
			// we should modify not local relations from object, that's why we apply changes even if object is not in cache
			err = ot.Space().Do(id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				st.RemoveDetail(changes.relationsToRemove...)
				if changes.isFeaturedRelationsChanged {
					st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(changes.newFeaturedRelations))
				}
				return b.Apply(st)
			})
			if err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			continue
		}

		if changes.isLayoutFound || !newLayout.isLayoutSet || record.Details.GetInt64(bundle.RelationKeyResolvedLayout) == newLayout.layout {
			// layout detail remains in object or recommendedLayout was not changed or relevant layout is already set, skipping
			continue
		}

		err = ot.Space().DoLockedIfNotExists(id, func() error {
			return ot.spaceIndex.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
				if details == nil {
					return nil, false, nil
				}
				if details.GetInt64(bundle.RelationKeyResolvedLayout) == newLayout.layout {
					return nil, false, nil
				}
				details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(newLayout.layout))
				return details, true, nil
			})
		})

		if err == nil {
			continue
		}

		if !errors.Is(err, ocache.ErrExists) {
			resultErr = errors.Join(resultErr, err)
			continue
		}

		if err = ot.Space().Do(id, func(b smartblock.SmartBlock) error {
			if cr, ok := b.(source.ChangeReceiver); ok {
				// we can do StateAppend here, so resolvedLayout will be injected automatically
				return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
					return d.NewState(), nil, nil
				})
			}
			return nil
		}); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}

	if resultErr != nil {
		return fmt.Errorf("failed to change layout details for objects: %w", resultErr)
	}
	return nil
}

func isLayoutChangeApplicable(layout int64) bool {
	return slices.Contains([]model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_todo,
		model.ObjectType_profile,
		model.ObjectType_note,
		model.ObjectType_collection,
	}, model.ObjectTypeLayout(layout)) // nolint:gosec
}

func getLayoutStateFromMessages(msgs []simple.EventMessage) layoutState {
	ls := layoutState{}
	for _, ev := range msgs {
		if amend := ev.Msg.GetObjectDetailsAmend(); amend != nil {
			for _, detail := range amend.Details {
				switch detail.Key {
				case bundle.RelationKeyRecommendedLayout.String():
					ls.layout = int64(detail.Value.GetNumberValue())
					ls.isLayoutSet = true
				case bundle.RelationKeyRecommendedFeaturedRelations.String():
					ls.featuredRelations = pbtypes.GetStringListValue(detail.Value)
					ls.isFeaturedRelationsSet = true
				case bundle.RelationKeyLayoutAlign.String():
					ls.layoutAlign = int64(detail.Value.GetNumberValue())
					ls.isLayoutAlignSet = true
				}
			}
			if ls.isAllSet() {
				return ls
			}
		}
	}
	return ls
}

func getLayoutStateFromParent(ps *state.State) layoutState {
	ls := layoutState{}
	if ps == nil {
		return ls
	}

	if layout, ok := ps.Details().TryInt64(bundle.RelationKeyRecommendedLayout); ok {
		ls.layout = layout
		ls.isLayoutSet = true
	}

	if layoutAlign, ok := ps.Details().TryInt64(bundle.RelationKeyLayoutAlign); ok {
		ls.layoutAlign = layoutAlign
		ls.isLayoutAlignSet = true
	}

	featuredRelations, ok := ps.Details().TryStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	// featuredRelations can present in objects as empty slice or containing only description
	if ok && len(featuredRelations) != 0 && !slices.Equal(featuredRelations, []string{bundle.RelationKeyDescription.String()}) {
		ls.featuredRelations = featuredRelations
		ls.isFeaturedRelationsSet = true
	}
	return ls
}

func (ot *ObjectType) queryObjectsAndTemplates() ([]database.Record, error) {
	records, err := ot.spaceIndex.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(ot.Id()),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get objects of single type: %w", err)
	}

	templates, err := ot.spaceIndex.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyTargetObjectType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(ot.Id()),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get templates with this target type: %w", err)
	}

	return append(records, templates...), nil
}

type layoutRelationsChanges struct {
	relationsToRemove          []domain.RelationKey
	isLayoutFound              bool
	isFeaturedRelationsChanged bool
	newFeaturedRelations       []string
}

func collectRelationsChanges(details *domain.Details, newLayout, oldLayout layoutState, deriver relationIdDeriver) (changes layoutRelationsChanges) {
	changes.relationsToRemove = make([]domain.RelationKey, 0, 2)
	if newLayout.isLayoutSet {
		layout, found := details.TryInt64(bundle.RelationKeyLayout)
		if found {
			changes.isLayoutFound = true
			if layout == newLayout.layout || oldLayout.isLayoutSet && layout == oldLayout.layout {
				changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayout)
			}
		}
	}

	if newLayout.isLayoutAlignSet {
		layoutAlign, found := details.TryInt64(bundle.RelationKeyLayoutAlign)
		if found && (layoutAlign == newLayout.layoutAlign || oldLayout.isLayoutAlignSet && layoutAlign == oldLayout.layoutAlign) {
			changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayoutAlign)
		}
	}

	if newLayout.isFeaturedRelationsSet {
		featuredRelations, found := details.TryStringList(bundle.RelationKeyFeaturedRelations)
		if found && isFeaturedRelationsCorrespondToType(featuredRelations, newLayout, oldLayout, deriver) {
			changes.isFeaturedRelationsChanged = true
			changes.newFeaturedRelations = []string{}
			if slices.Contains(featuredRelations, bundle.RelationKeyDescription.String()) {
				changes.newFeaturedRelations = append(changes.newFeaturedRelations, bundle.RelationKeyDescription.String())
			}
		}
	}
	return changes
}

func isFeaturedRelationsCorrespondToType(fr []string, newLayout, oldLayout layoutState, deriver relationIdDeriver) bool {
	featuredRelationIds := make([]string, 0, len(fr))
	for _, key := range fr {
		id, err := deriver.deriveId(domain.RelationKey(key))
		if err != nil {
			log.Errorf("failed to derive relation key %s", key)
			return false // let's fallback to true, so featuredRelations won't be changed
		}
		featuredRelationIds = append(featuredRelationIds, id)
	}

	if newLayout.isFeaturedRelationsSet && slices.Equal(featuredRelationIds, newLayout.featuredRelations) {
		return true
	}

	return oldLayout.isFeaturedRelationsSet && slices.Equal(featuredRelationIds, oldLayout.featuredRelations)
}
