package block

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) TemplateCreateFromObject(ctx context.Context, id string) (templateID string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)

	if err = Do(s, id, func(b smartblock.SmartBlock) error {
		if b.Type() != coresb.SmartBlockTypePage {
			return fmt.Errorf("can't make template from this obect type")
		}
		objectTypeKeys = b.ObjectTypeKeys()
		st, err = s.templateCreateFromObjectState(b)
		return err
	}); err != nil {
		return
	}

	spaceID, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}

	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, objectTypeKeys, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) templateCreateFromObjectState(sb smartblock.SmartBlock) (*state.State, error) {
	st := sb.NewState().Copy()
	st.SetLocalDetails(nil)
	targetObjectTypeID, err := s.systemObjectService.GetTypeIdByKey(context.Background(), sb.SpaceID(), st.ObjectTypeKey())
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %s", err)
	}
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(targetObjectTypeID))
	st.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, st.ObjectTypeKey()})
	for _, rel := range sb.Relations(st) {
		if rel.DataSource == model.Relation_details && !rel.Hidden {
			st.RemoveDetail(rel.Key)
		}
	}
	return st, nil
}

func (s *Service) TemplateClone(spaceID string, id string) (templateID string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)
	if err = Do(s, id, func(b smartblock.SmartBlock) error {
		if b.Type() != coresb.SmartBlockTypeBundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		objectTypeKeys = b.ObjectTypeKeys()
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(id))

		targetObjectTypeBundledID := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
		targetObjectTypeKey, err := bundle.TypeKeyFromUrl(targetObjectTypeBundledID)
		if err != nil {
			return fmt.Errorf("get target object type key: %w", err)
		}
		targetObjectTypeID, err := s.systemObjectService.GetTypeIdByKey(context.Background(), spaceID, targetObjectTypeKey)
		if err != nil {
			return fmt.Errorf("get target object type id: %w", err)
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyTargetObjectType, pbtypes.String(targetObjectTypeID))
		return nil
	}); err != nil {
		return
	}
	// TODO Check this, we need to create template, so pass template type key, and creator should use template sbtype
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(context.Background(), spaceID, objectTypeKeys, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectDuplicate(ctx context.Context, id string) (objectID string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)
	if err = Do(s, id, func(b smartblock.SmartBlock) error {
		objectTypeKeys = b.ObjectTypeKeys()
		if err = b.Restrictions().Object.Check(model.Restrictions_Duplicate); err != nil {
			return err
		}
		st = b.NewState().Copy()
		st.SetLocalDetails(nil)
		st.SetDetail(bundle.RelationKeySourceObject.String(), pbtypes.String(id))
		return nil
	}); err != nil {
		return
	}

	spaceID, err := s.resolver.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	objectID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, objectTypeKeys, st)
	if err != nil {
		return
	}
	return
}

// TODO Seems like it unused by clients
func (s *Service) TemplateCreateFromObjectByObjectType(ctx context.Context, objectTypeID string) (templateID string, err error) {
	return
	// spaceID, err := s.ResolveSpaceID(objectTypeID)
	// if err != nil {
	// 	return "", fmt.Errorf("resolve spaceID: %w", err)
	// }
	// if err = Do(s, objectTypeID, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
	// 	return "", fmt.Errorf("can't open objectType: %v", err)
	// }
	// objectType, err := s.objectStore.GetObjectType(objectTypeID)
	// if err != nil {
	// 	return "", fmt.Errorf("get object type: %w", err)
	// }
	// st := state.NewDoc("", nil).(*state.State)
	// st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(objectTypeID))
	// templateID, _, err = s.objectCreator.CreateSmartBlockFromState(
	// 	ctx,
	// 	spaceID,
	// 	coresb.SmartBlockTypeTemplate,
	// 	[]bundle.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKey(objectType.Key)},
	// 	nil,
	// 	st,
	// )
	// if err != nil {
	// 	return
	// }
	// return
}

func (s *Service) CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, err error) {
	newSpace, err := s.spaceService.Create(ctx)
	if err != nil {
		return "", fmt.Errorf("error creating space: %w", err)
	}
	predefinedObjectIDs := newSpace.DerivedIDs()

	err = Do(s, predefinedObjectIDs.Workspace, func(b basic.DetailsSettable) error {
		details := make([]*pb.RpcObjectSetDetailsDetail, 0, len(req.Details.GetFields()))
		for k, v := range req.Details.GetFields() {
			details = append(details, &pb.RpcObjectSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}
		return b.SetDetails(nil, details, true)
	})
	if err != nil {
		return "", fmt.Errorf("set details for space %s: %w", newSpace.Id(), err)
	}
	_, err = s.builtinObjectService.CreateObjectsForUseCase(nil, newSpace.Id(), req.UseCase)
	if err != nil {
		return "", fmt.Errorf("import use-case: %w", err)
	}
	return newSpace.Id(), err
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(
	ctx context.Context,
	sctx session.Context,
	req *pb.RpcBlockLinkCreateWithObjectRequest,
) (linkID string, objectID string, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}

	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return "", "", fmt.Errorf("get type key from raw unique key: %w", err)
	}

	createReq := objectcreator.CreateObjectRequest{
		Details:       req.Details,
		InternalFlags: req.InternalFlags,
		ObjectTypeKey: objectTypeKey,
		TemplateId:    req.TemplateId,
	}
	objectID, _, err = s.objectCreator.CreateObject(ctx, req.SpaceId, createReq)
	if err != nil {
		return
	}
	if req.ContextId == "" {
		return
	}

	err = DoStateCtx(s, sctx, req.ContextId, func(st *state.State, sb basic.Creatable) error {
		linkID, err = sb.CreateBlock(st, pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: objectID,
						Style:         model.BlockContentLink_Page,
					},
				},
				Fields: req.Fields,
			},
			Position: req.Position,
		})
		if err != nil {
			return fmt.Errorf("link create error: %v", err)
		}
		return nil
	})
	return
}

func (s *Service) ObjectToSet(id string, source []string) error {
	if err := Do(s, id, func(b smartblock.SmartBlock) error {
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return fmt.Errorf("invalid smartblock impmlementation: %T", b)
		}
		st := b.NewState()
		st.SetDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		err := commonOperations.SetLayoutInStateAndIgnoreRestriction(st, model.ObjectType_set)
		if err != nil {
			return fmt.Errorf("set layout: %w", err)
		}
		st.SetObjectTypeKey(bundle.TypeKeySet)
		flags := internalflag.NewFromState(st)
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorDeleteEmpty)
		flags.AddToState(st)

		return b.Apply(st)
	}); err != nil {
		return err
	}

	return nil
}
