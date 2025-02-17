package template

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName           = "template"
	BlankTemplateId = "blank"
)

var (
	log = logging.Logger("template")

	templateIsPreferableRelationKeys = []domain.RelationKey{
		bundle.RelationKeyFeaturedRelations, bundle.RelationKeyResolvedLayout,
		bundle.RelationKeyIconEmoji, bundle.RelationKeyCoverId,
		bundle.RelationKeySourceObject, bundle.RelationKeySetOf,
	}
)

type Service interface {
	CreateTemplateStateWithDetails(templateId string, details *domain.Details, withTemplateValidation bool) (st *state.State, err error)
	CreateTemplateStateFromSmartBlock(sb smartblock.SmartBlock, details *domain.Details) *state.State
	ObjectApplyTemplate(contextId string, templateId string) error
	TemplateCreateFromObject(ctx context.Context, id string) (templateId string, err error)

	TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error)
	TemplateClone(spaceId string, id string) (templateId string, err error)

	TemplateExportAll(ctx context.Context, path string) (string, error)

	SetDefaultTemplateInType(ctx context.Context, typeId, templateId string) error

	app.Component
}

type service struct {
	picker       cache.ObjectGetter
	store        objectstore.ObjectStore
	spaceService space.Service
	creator      objectcreator.Service
	resolver     idresolver.Resolver
	exporter     export.Export
	converter    converter.LayoutConverter
}

func New() Service {
	return &service{}
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.picker = app.MustComponent[cache.ObjectGetter](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.creator = app.MustComponent[objectcreator.Service](a)
	s.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
	s.exporter = a.MustComponent(export.CName).(export.Export)
	s.converter = app.MustComponent[converter.LayoutConverter](a)
	return nil
}

// CreateTemplateStateWithDetails creates clone of template object state with empty localDetails and updated objectTypes.
// If withTemplateValidation=true, templateId is queried in store. If template is empty or not found, last edited template is taken.
// Blank template is created in case template object is deleted or blank/empty templateId is provided
func (s *service) CreateTemplateStateWithDetails(
	templateId string,
	details *domain.Details,
	withTemplateValidation bool,
) (targetState *state.State, err error) {
	if withTemplateValidation {
		templateId, err = s.resolveValidTemplateId(templateId, details)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve valid template id: %w", err)
		}
	}
	switch templateId {
	case "", BlankTemplateId:
		layout := details.GetInt64(bundle.RelationKeyResolvedLayout)
		// nolint:gosec
		targetState = s.createBlankTemplateState(model.ObjectTypeLayout(layout), details)
	default:
		targetState, err = s.createCustomTemplateState(templateId)
		if err != nil {
			return
		}
	}

	addDetailsToState(targetState, details)
	return targetState, nil
}

func (s *service) resolveValidTemplateId(templateId string, details *domain.Details) (string, error) {
	var (
		typeId  = details.GetString(bundle.RelationKeyType)
		spaceId = details.GetString(bundle.RelationKeySpaceId)
		ctx     = context.Background()
	)

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("failed to get space: %w", err)
	}
	templateTypeId, err := spc.GetTypeIdByKey(ctx, bundle.TypeKeyTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to get template type id from space: %w", err)
	}

	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(templateTypeId),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(typeId),
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
		}},
	})

	if err != nil {
		return "", fmt.Errorf("failed to query templates: %w", err)
	}

	if len(records) == 0 {
		// if no valid templates presented, we shell create new object with blank template
		return "", nil
	}

	if templateId == "" {
		return records[0].Details.GetString(bundle.RelationKeyId), nil
	}

	for _, record := range records {
		if record.Details.GetString(bundle.RelationKeyId) == templateId {
			return templateId, nil
		}
	}

	// if requested templateId was not found in store, we should use last modified template
	return records[0].Details.GetString(bundle.RelationKeyId), nil
}

// CreateTemplateStateFromSmartBlock duplicates the logic of CreateTemplateStateWithDetails but does not take the lock on smartBlock.
// if building of state fails, state of blank template is returned
func (s *service) CreateTemplateStateFromSmartBlock(sb smartblock.SmartBlock, details *domain.Details) *state.State {
	st, err := s.buildState(sb)
	if err != nil {
		layout := details.GetInt64(bundle.RelationKeyResolvedLayout)
		// nolint:gosec
		st = s.createBlankTemplateState(model.ObjectTypeLayout(layout), nil)
	}
	addDetailsToState(st, details)
	return st
}

func extractTargetDetails(originDetails *domain.Details, templateDetails *domain.Details) *domain.Details {
	targetDetails := originDetails.Copy()
	if templateDetails == nil {
		return targetDetails
	}
	for key, originalVal := range originDetails.Iterate() {
		templateVal := templateDetails.Get(key)
		if templateVal.Ok() {
			inTemplateEmpty := templateVal.IsEmpty()
			if key == bundle.RelationKeyResolvedLayout {
				// layout = 0 is actually basic layout, so it counts
				inTemplateEmpty = false
			}
			inOriginEmpty := originalVal.IsEmpty()
			templateValueShouldBePreferred := lo.Contains(templateIsPreferableRelationKeys, key)
			if !inTemplateEmpty && (inOriginEmpty || templateValueShouldBePreferred) {
				targetDetails.Delete(key)
			}
		}
	}
	return targetDetails
}

func (s *service) createCustomTemplateState(templateId string) (targetState *state.State, err error) {
	err = cache.Do(s.picker, templateId, func(sb smartblock.SmartBlock) (innerErr error) {
		targetState, innerErr = s.buildState(sb)
		if innerErr != nil {
			return innerErr
		}
		details := targetState.Details()
		if details.GetBool(bundle.RelationKeyIsDeleted) || details.GetBool(bundle.RelationKeyIsUninstalled) {
			return spacestorage.ErrTreeStorageAlreadyDeleted
		}
		return nil
	})
	if errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
		return s.createBlankTemplateState(model.ObjectType_basic, nil), nil
	}
	return
}

func (s *service) buildState(sb smartblock.SmartBlock) (st *state.State, err error) {
	if sb == nil {
		return nil, fmt.Errorf("smartblock is nil")
	}
	if !lo.Contains(sb.ObjectTypeKeys(), bundle.TypeKeyTemplate) {
		return nil, fmt.Errorf("object '%s' is not a template", sb.Id())
	}
	st = sb.NewState().Copy()

	if st.LocalDetails().GetBool(bundle.RelationKeyIsArchived) {
		return nil, spacestorage.ErrTreeStorageAlreadyDeleted
	}

	err = s.updateTypeKey(sb.SpaceID(), st)
	if err != nil {
		return
	}

	st.RemoveDetail(
		bundle.RelationKeyTargetObjectType,
		bundle.RelationKeyTemplateIsBundled,
		bundle.RelationKeyOrigin,
		bundle.RelationKeyAddedDate,
	)
	st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, domain.String(sb.Id()))
	// original created timestamp is used to set creationDate for imported objects, not for template-based objects
	st.SetOriginalCreatedTimestamp(0)
	st.SetLocalDetails(nil)
	return
}

func (s *service) ObjectApplyTemplate(contextId, templateId string) error {
	return cache.Do(s.picker, contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.CreateTemplateStateWithDetails(templateId, orig.Details(), false)
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)

		ts.BlocksInit(ts)

		objType := orig.ObjectTypeKey()
		ts.SetObjectTypeKey(objType)

		flags := internalflag.NewFromState(orig)
		flags.AddToState(ts)

		// we provide KeepInternalFlags to allow further template applying and object type change
		return b.Apply(ts, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
	})
}

func (s *service) TemplateCreateFromObject(ctx context.Context, id string) (templateId string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)

	if err = cache.Do(s.picker, id, func(b smartblock.SmartBlock) error {
		if b.Type() != coresb.SmartBlockTypePage {
			return fmt.Errorf("can't make template from this object type: %s", model.SmartBlockType_name[int32(b.Type())])
		}
		st, err = buildTemplateStateFromObject(b)
		objectTypeKeys = st.ObjectTypeKeys()
		return err
	}); err != nil {
		return
	}

	spaceId, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceId: %w", err)
	}

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}

	templateId, _, err = s.creator.CreateSmartBlockFromStateInSpace(ctx, spc, objectTypeKeys, st)
	if err != nil {
		return
	}

	typeId, err2 := spc.GetTypeIdByKey(ctx, objectTypeKeys[0])
	if err2 != nil {
		log.Errorf("failed to get typeId: %v", err2)
		return
	}

	if err2 = s.SetDefaultTemplateInType(ctx, typeId, templateId); err != nil {
		log.Errorf("failed to set defaultTemplateId to type: %v", err2)
	}
	return
}

func (s *service) TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)
	marketplaceSpace, err := s.spaceService.Get(context.Background(), addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return "", fmt.Errorf("get marketplace space: %w", err)
	}
	if err = marketplaceSpace.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != coresb.SmartBlockTypeBundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		objectTypeKeys = b.ObjectTypeKeys()
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled)
		st.SetLocalDetails(nil)
		st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, domain.String(id))

		targetObjectTypeBundledId := st.Details().GetString(bundle.RelationKeyTargetObjectType)
		targetObjectTypeKey, err := bundle.TypeKeyFromUrl(targetObjectTypeBundledId)
		if err != nil {
			return fmt.Errorf("get target object type key: %w", err)
		}
		targetObjectTypeId, err := space.GetTypeIdByKey(context.Background(), targetObjectTypeKey)
		if err != nil {
			return fmt.Errorf("get target object type id: %w", err)
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyTargetObjectType, domain.String(targetObjectTypeId))
		return nil
	}); err != nil {
		return
	}
	templateId, _, err = s.creator.CreateSmartBlockFromStateInSpace(context.Background(), space, objectTypeKeys, st)
	if err != nil {
		return
	}
	return
}

func (s *service) TemplateClone(spaceId string, id string) (templateId string, err error) {
	var spaceObject clientspace.Space
	spaceObject, err = s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}
	return s.TemplateCloneInSpace(spaceObject, id)
}

func (s *service) TemplateExportAll(ctx context.Context, path string) (string, error) {
	records, err := s.store.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(false),
			},
			{
				RelationKey: database.NestedRelationKey(bundle.RelationKeyType, bundle.RelationKeyUniqueKey),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(bundle.TypeKeyTemplate.URL()),
			},
			// We don't want templates from marketplace
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(addr.AnytypeMarketplaceWorkspace),
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("no templates")
	}
	ids := make([]string, 0, len(records))
	for _, rec := range records {
		ids = append(ids, rec.Details.GetString(bundle.RelationKeyId))
	}
	path, _, err = s.exporter.Export(ctx, pb.RpcObjectListExportRequest{
		Path:      path,
		ObjectIds: ids,
		Format:    model.Export_Protobuf,
		Zip:       true,
	})
	return path, err
}

// SetDefaultTemplateInType sets defaultTemplateId to type object in case it is empty
func (s *service) SetDefaultTemplateInType(ctx context.Context, typeId, templateId string) error {
	return cache.DoContext(s.picker, ctx, typeId, func(sb smartblock.SmartBlock) error {
		if sb.Details().GetString(bundle.RelationKeyDefaultTemplateId) != "" {
			return nil
		}
		ds := sb.(basic.DetailsSettable)
		if ds == nil {
			return fmt.Errorf("cannot set defaultTemplateId as type object does not support DetailsSettable interface")
		}
		return ds.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeyDefaultTemplateId,
			Value: domain.String(templateId),
		}}, false)
	})
}

func (s *service) createBlankTemplateState(layout model.ObjectTypeLayout, details *domain.Details) (st *state.State) {
	st = state.NewDoc(BlankTemplateId, nil).NewState()
	template.InitTemplate(st, template.WithEmpty,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithAddedFeaturedRelation(bundle.RelationKeyTag),
		template.WithDetail(bundle.RelationKeyTag, domain.StringList(nil)),
		template.WithTitle,
	)
	if slices.Contains([]model.ObjectTypeLayout{model.ObjectType_set, model.ObjectType_collection}, layout) && details != nil {
		template.InitTemplate(st,
			template.WithDetail(bundle.RelationKeySpaceId, details.Get(bundle.RelationKeySpaceId)),
			template.WithDetail(bundle.RelationKeyType, details.Get(bundle.RelationKeyType)),
		)
	}
	if err := s.converter.Convert(st, model.ObjectType_basic, layout, true); err != nil {
		log.Errorf("failed to set '%s' layout to blank template: %v", layout.String(), err)
	}
	return
}

func (s *service) updateTypeKey(spaceId string, st *state.State) (err error) {
	objectTypeId := st.Details().GetString(bundle.RelationKeyTargetObjectType)
	if objectTypeId != "" {
		var uniqueKey domain.UniqueKey
		uniqueKey, err = s.store.SpaceIndex(spaceId).GetUniqueKeyById(objectTypeId)
		if err != nil {
			err = fmt.Errorf("get target object type %s: %w", objectTypeId, err)
		} else if uniqueKey.SmartblockType() != coresb.SmartBlockTypeObjectType {
			err = fmt.Errorf("unique key %s does not belong to object type", uniqueKey.InternalKey())
		}
		if err == nil {
			st.SetObjectTypeKey(domain.TypeKey(uniqueKey.InternalKey()))
			return nil
		}
		log.Errorf(err.Error())
	}
	updatedTypeKeys := slice.Remove(st.ObjectTypeKeys(), bundle.TypeKeyTemplate)
	if len(updatedTypeKeys) != 1 {
		return fmt.Errorf("failed to gather type key from template's ObjectTypeKeys (%v) and from object store: %w", st.ObjectTypeKeys(), err)
	}
	st.SetObjectTypeKey(updatedTypeKeys[0])
	return nil
}

func buildTemplateStateFromObject(sb smartblock.SmartBlock) (*state.State, error) {
	st := sb.NewState().Copy()
	st.SetLocalDetails(nil)
	targetObjectTypeId, err := sb.Space().GetTypeIdByKey(context.Background(), st.ObjectTypeKey())
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %w", err)
	}
	st.SetDetail(bundle.RelationKeyTargetObjectType, domain.String(targetObjectTypeId))
	st.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, st.ObjectTypeKey()})
	for _, rel := range sb.Relations(st) {
		if rel.DataSource == model.Relation_details && !rel.Hidden {
			st.RemoveDetail(domain.RelationKey(rel.Key))
		}
	}
	flags := internalflag.NewFromState(st)
	flags.Remove(model.InternalFlag_editorDeleteEmpty)
	flags.AddToState(st)
	return st, nil
}

func addDetailsToState(s *state.State, details *domain.Details) {
	targetDetails := extractTargetDetails(details, s.Details())
	s.AddDetails(targetDetails)
	s.BlocksInit(s)
}
