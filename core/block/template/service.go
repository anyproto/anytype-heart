package template

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/getblock"
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
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName           = "template"
	BlankTemplateId = "blank"
)

var log = logging.Logger("template")

type Service interface {
	StateFromTemplate(templateId, name string) (st *state.State, err error)
	ObjectApplyTemplate(contextId string, templateId string) error
	TemplateCreateFromObject(ctx context.Context, id string) (templateId string, err error)

	TemplateCloneInSpace(space space.Space, id string) (templateId string, err error)
	TemplateClone(spaceId string, id string) (templateId string, err error)

	TemplateExportAll(ctx context.Context, path string) (string, error)

	app.Component
}

type service struct {
	picker       getblock.ObjectGetter
	store        objectstore.ObjectStore
	spaceService space.Service
	creator      objectcreator.Service
	resolver     idresolver.Resolver
	exporter     export.Export
}

func New() Service {
	return &service{}
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.picker = app.MustComponent[getblock.ObjectGetter](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.creator = app.MustComponent[objectcreator.Service](a)
	s.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
	s.exporter = a.MustComponent(export.CName).(export.Export)
	return nil
}

// StateFromTemplate creates clone of template object state with empty localDetails and updated objectTypes.
// Blank template is created in case template object is deleted or blank/empty templateIв is provided
func (s *service) StateFromTemplate(templateId, name string) (st *state.State, err error) {
	if templateId == BlankTemplateId || templateId == "" {
		return s.blankTemplateState(), nil
	}
	if err = getblock.Do(s.picker, templateId, func(b smartblock.SmartBlock) (innerErr error) {
		if lo.Contains(b.ObjectTypeKeys(), bundle.TypeKeyTemplate) {
			st, innerErr = s.getNewPageState(b, name)
		} else {
			return fmt.Errorf("object '%s' is not a template", templateId)
		}
		return
	}); err != nil {
		if errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
			return s.blankTemplateState(), nil
		}
		return nil, fmt.Errorf("can't apply template: %w", err)
	}
	return
}

func (s *service) ObjectApplyTemplate(contextId, templateId string) error {
	return getblock.Do(s.picker, contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.StateFromTemplate(templateId, "")
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)

		layout, found := orig.Layout()
		if found {
			if commonOperations, ok := b.(basic.CommonOperations); ok {
				if err = commonOperations.SetLayoutInStateAndIgnoreRestriction(ts, layout); err != nil {
					return fmt.Errorf("convert layout: %w", err)
				}
			}
		}

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

	if err = getblock.Do(s.picker, id, func(b smartblock.SmartBlock) error {
		if b.Type() != coresb.SmartBlockTypePage {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = s.templateCreateFromObjectState(b)
		objectTypeKeys = st.ObjectTypeKeys()
		return err
	}); err != nil {
		return
	}

	spaceId, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceId: %w", err)
	}

	templateId, _, err = s.creator.CreateSmartBlockFromState(ctx, spaceId, objectTypeKeys, st)
	if err != nil {
		return
	}
	return
}

func (s *service) TemplateCloneInSpace(space space.Space, id string) (templateId string, err error) {
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
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(id))

		targetObjectTypeBundledId := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
		targetObjectTypeKey, err := bundle.TypeKeyFromUrl(targetObjectTypeBundledId)
		if err != nil {
			return fmt.Errorf("get target object type key: %w", err)
		}
		targetObjectTypeId, err := space.GetTypeIdByKey(context.Background(), targetObjectTypeKey)
		if err != nil {
			return fmt.Errorf("get target object type id: %w", err)
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyTargetObjectType, pbtypes.String(targetObjectTypeId))
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
	var spaceObject space.Space
	spaceObject, err = s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}
	return s.TemplateCloneInSpace(spaceObject, id)
}

func (s *service) TemplateExportAll(ctx context.Context, path string) (string, error) {
	docIds, _, err := s.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
			{
				RelationKey: database.NestedRelationKey(bundle.RelationKeyType, bundle.RelationKeyUniqueKey),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyTemplate.URL()),
			},
			// We don't want templates from marketplace
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(addr.AnytypeMarketplaceWorkspace),
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(docIds) == 0 {
		return "", fmt.Errorf("no templates")
	}
	path, _, err = s.exporter.Export(ctx, pb.RpcObjectListExportRequest{
		Path:      path,
		ObjectIds: docIds,
		Format:    pb.RpcObjectListExport_Protobuf,
		Zip:       true,
	})
	return path, err
}

func (s *service) blankTemplateState() (st *state.State) {
	st = state.NewDoc(BlankTemplateId, nil).NewState()
	template.InitTemplate(st, template.WithEmpty,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithRequiredRelations(),
		template.WithTitle,
		template.WithDescription,
	)
	return
}

func (s *service) getNewPageState(sb smartblock.SmartBlock, name string) (st *state.State, err error) {
	st = sb.NewState().Copy()

	if err = s.updateTypeKey(st); err != nil {
		return nil, err
	}

	st.RemoveDetail(bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyTemplateIsBundled.String())
	st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(sb.Id()))
	// clean-up local details from the template state
	st.SetLocalDetails(nil)

	if name != "" {
		st.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(name))
		if title := st.Get(template.TitleBlockId); title != nil {
			title.Model().GetText().Text = name
		}
	}
	return
}

func (s *service) updateTypeKey(st *state.State) (err error) {
	objectTypeId := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
	if objectTypeId != "" {
		var uniqueKey domain.UniqueKey
		uniqueKey, err = s.store.GetUniqueKeyById(objectTypeId)
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

func (s *service) templateCreateFromObjectState(sb smartblock.SmartBlock) (*state.State, error) {
	st := sb.NewState().Copy()
	st.SetLocalDetails(nil)
	targetObjectTypeId, err := sb.Space().GetTypeIdByKey(context.Background(), st.ObjectTypeKey())
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %w", err)
	}
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(targetObjectTypeId))
	st.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, st.ObjectTypeKey()})
	for _, rel := range sb.Relations(st) {
		if rel.DataSource == model.Relation_details && !rel.Hidden {
			st.RemoveDetail(rel.Key)
		}
	}
	return st, nil
}
