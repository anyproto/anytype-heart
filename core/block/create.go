package block

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) TemplateCreateFromObject(ctx context.Context, id string) (templateID string, err error) {
	var st *state.State
	if err = Do(s, id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = b.TemplateCreateFromObjectState()
		return err
	}); err != nil {
		return
	}

	spaceID, err := s.ResolveSpaceID(id)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}

	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateClone(spaceID string, id string) (templateID string, err error) {
	var st *state.State
	if err = Do(s, id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(id))
		t := st.ObjectTypes()
		st.SetObjectTypes(t)
		return nil
	}); err != nil {
		return
	}
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(context.Background(), spaceID, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectDuplicate(ctx context.Context, id string) (objectID string, err error) {
	var (
		st  *state.State
		sbt coresb.SmartBlockType
	)
	if err = Do(s, id, func(b smartblock.SmartBlock) error {
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

	spaceID, err := s.objectStore.ResolveSpaceID(objectID)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	objectID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, sbt, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateCreateFromObjectByObjectType(ctx context.Context, otID string) (templateID string, err error) {
	spaceID, err := s.ResolveSpaceID(otID)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	if err = Do(s, otID, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
		return "", fmt.Errorf("can't open objectType: %v", err)
	}
	var st = state.NewDoc("", nil).(*state.State)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(otID))
	templateTypeID, err := s.relationService.GetSystemTypeId(spaceID, bundle.TypeKeyTemplate)
	if err != nil {
		return "", fmt.Errorf("get template type id: %w", err)
	}
	st.SetObjectTypes([]string{templateTypeID, otID})
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeTemplate, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, err error) {
	spc, err := s.spaceService.CreateSpace(ctx)
	if err != nil {
		return "", fmt.Errorf("create space: %w", err)
	}

	predefinedObjectIDs, err := s.anytype.DerivePredefinedObjects(ctx, spc.Id(), true)
	if err != nil {
		// TODO Delete space?
		return "", fmt.Errorf("derive workspace object for space %s: %w", spc.Id(), err)
	}

	err = DoStateAsync(s, s.anytype.AccountObjects().Account, func(st *state.State, b *editor.Workspaces) error {
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

	err = Do(s, predefinedObjectIDs.Account, func(b basic.DetailsSettable) error {
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

	err = s.indexer.ReindexSpace(spc.Id())
	if err != nil {
		return "", fmt.Errorf("reindex space %s: %w", spc.Id(), err)
	}

	_, err = s.builtinObjectService.CreateObjectsForUseCase(ctx, spc.Id(), req.UseCase)
	if err != nil {
		return "", fmt.Errorf("import use-case: %w", err)
	}
	return spc.Id(), err
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

	fmt.Printf("%+v", s.anytype.PredefinedObjects(req.SpaceId))
	uk, err := uniquekey.New(model.SmartBlockType_STType, bundle.TypeKeyPage.String())
	sss, _ := s.anytype.DeriveObjectId(ctx, req.SpaceId, uk)
	fmt.Printf("%+v", sss)

	s.objectCreator.InjectWorkspaceID(req.Details, req.SpaceId, req.ContextId)
	objectID, _, err = s.CreateObject(ctx, req.SpaceId, req, "")
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

func (s *Service) ObjectToSet(ctx session.Context, id string, source []string) error {
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
		st.SetObjectType(bundle.TypeKeySet.String())
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

func (s *Service) CreateObject(ctx context.Context, spaceID string, req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	return s.objectCreator.CreateObject(ctx, spaceID, req, forcedType)
}
