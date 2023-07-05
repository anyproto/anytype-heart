package block

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) TemplateCreateFromObject(ctx session.Context, id string) (templateID string, err error) {
	var st *state.State
	if err = Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = b.TemplateCreateFromObjectState()
		return err
	}); err != nil {
		return
	}

	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateClone(ctx session.Context, id string) (templateID string, err error) {
	var st *state.State
	if err = Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(id))
		t := st.ObjectTypes()
		t, _ = relationutils.MigrateObjectTypeIds(t)
		st.SetObjectTypes(t)
		targetObjectType, _ := relationutils.MigrateObjectTypeId(pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String()))
		st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(targetObjectType))
		return nil
	}); err != nil {
		return
	}
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectDuplicate(ctx session.Context, id string) (objectID string, err error) {
	var (
		st  *state.State
		sbt coresb.SmartBlockType
	)
	if err = Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		sbt = coresb.SmartBlockType(b.Type())
		if err = b.Restrictions().Object.Check(model.Restrictions_Duplicate); err != nil {
			return err
		}
		st = b.NewState().Copy()
		st.SetLocalDetails(nil)
		return nil
	}); err != nil {
		return
	}

	objectID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, sbt, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateCreateFromObjectByObjectType(ctx session.Context, otID string) (templateID string, err error) {
	if err = Do(s, ctx, otID, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
		return "", fmt.Errorf("can't open objectType: %v", err)
	}
	var st = state.NewDoc("", nil).(*state.State)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(otID))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), otID})
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) CreateWorkspace(ctx session.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, err error) {
	spc, err := s.spaceService.CreateSpace(ctx.Context())
	if err != nil {
		return "", fmt.Errorf("create space: %w", err)
	}

	newSpaceCtx := session.NewContext(ctx.Context(), spc.Id())
	predefinedObjectIDs, err := s.anytype.DerivePredefinedObjects(newSpaceCtx, true)
	if err != nil {
		// TODO Delete space?
		return "", fmt.Errorf("derive workspace object for space %s: %w", spc.Id(), err)
	}

	accountCtx := session.NewContext(ctx.Context(), s.spaceService.AccountId())
	err = DoStateAsync(s, accountCtx, s.anytype.AccountObjects().Account, func(st *state.State, b *editor.Workspaces) error {
		spaces := pbtypes.CopyVal(st.Store().GetFields()["spaces"])
		if spaces == nil {
			spaces = pbtypes.Struct(&types.Struct{
				Fields: map[string]*types.Value{
					spc.Id(): pbtypes.String(predefinedObjectIDs.Account),
				},
			})
		} else {
			spaces.GetStructValue().Fields[spc.Id()] = pbtypes.String(predefinedObjectIDs.Account)
		}
		st.SetInStore([]string{"spaces"}, spaces)
		return nil
	})

	err = Do(s, newSpaceCtx, predefinedObjectIDs.Account, func(b basic.DetailsSettable) error {
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
		return "", fmt.Errorf("set details for space %s: %w", spc.Id(), err)
	}

	err = s.indexer.ReindexSpace(newSpaceCtx)
	if err != nil {
		return "", fmt.Errorf("reindex space %s: %w", spc.Id(), err)
	}

	_, err = s.builtinObjectService.CreateObjectsForUseCase(newSpaceCtx, req.UseCase)
	if err != nil {
		return "", fmt.Errorf("import use-case: %w", err)
	}
	return spc.Id(), err
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(ctx session.Context, req *pb.RpcBlockLinkCreateWithObjectRequest) (linkID string, objectID string, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}

	s.objectCreator.InjectWorkspaceID(req.Details, ctx.SpaceID(), req.ContextId)
	objectID, _, err = s.CreateObject(ctx, req, "")
	if err != nil {
		return
	}
	if req.ContextId == "" {
		return
	}

	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, sb basic.Creatable) error {
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

func (s *Service) ObjectToSet(ctx session.Context, id string, source []string) error {
	if err := Do(s, ctx, id, func(b smartblock.SmartBlock) error {
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return fmt.Errorf("invalid smartblock impmlementation: %T", b)
		}
		st := b.NewState()
		st.SetDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		commonOperations.SetLayoutInState(st, model.ObjectType_set)
		st.SetObjectType(bundle.TypeKeySet.URL())
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

func (s *Service) CreateObject(ctx session.Context, req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	return s.objectCreator.CreateObject(ctx, req, forcedType)
}
