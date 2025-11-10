package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *Service) ObjectDuplicate(ctx context.Context, id string) (objectID string, err error) {
	var (
		st             *state.State
		objectTypeKeys []domain.TypeKey
	)
	if err = cache.Do(s, id, func(b smartblock.SmartBlock) error {
		objectTypeKeys = b.ObjectTypeKeys()
		if err = b.Restrictions().Object.Check(model.Restrictions_Duplicate); err != nil {
			return err
		}
		st = b.NewState().Copy()
		st.SetLocalDetails(nil)
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(id))
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

func (s *Service) CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, startingPageId string, err error) {
	spaceDetails := domain.NewDetailsFromProto(req.Details)
	spaceDescription := spaceinfo.NewSpaceDescriptionFromDetails(spaceDetails)

	newSpace, err := s.spaceService.Create(ctx, &spaceDescription)
	if err != nil {
		return "", "", fmt.Errorf("error creating space: %w", err)
	}
	predefinedObjectIDs := newSpace.DerivedIDs()

	err = cache.Do(s, predefinedObjectIDs.Workspace, func(b basic.DetailsSettable) error {
		details := make([]domain.Detail, 0, len(req.Details.GetFields()))
		for k, v := range req.Details.GetFields() {
			details = append(details, domain.Detail{
				Key:   domain.RelationKey(k),
				Value: domain.ValueFromProto(v),
			})
		}
		return b.SetDetails(nil, details, true)
	})
	if err != nil {
		return "", "", fmt.Errorf("set details for space %s: %w", newSpace.Id(), err)
	}
	startingPageId, _, err = s.builtinObjectService.CreateObjectsForUseCase(nil, newSpace.Id(), req.UseCase)
	if err != nil {
		return "", "", fmt.Errorf("import use-case: %w", err)
	}
	return newSpace.Id(), startingPageId, err
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(
	ctx context.Context,
	sctx session.Context,
	req *pb.RpcBlockLinkCreateWithObjectRequest,
) (linkID string, objectId string, objectDetails *domain.Details, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}

	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return "", "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}

	createReq := objectcreator.CreateObjectRequest{
		Details:       domain.NewDetailsFromProto(req.Details),
		InternalFlags: req.InternalFlags,
		ObjectTypeKey: objectTypeKey,
		TemplateId:    req.TemplateId,
	}
	objectId, objectDetails, err = s.objectCreator.CreateObject(ctx, req.SpaceId, createReq)
	if err != nil {
		return
	}
	if req.ContextId == "" {
		return
	}

	if req.Block == nil {
		req.Block = &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: objectId,
					Style:         model.BlockContentLink_Page,
				},
			},
			Fields: req.Fields,
		}
	} else {
		link := req.Block.GetLink()
		if link == nil {
			return "", "", nil, errors.New("block content is not a link")
		} else {
			link.TargetBlockId = objectId
		}
	}

	err = cache.DoStateCtx(s, sctx, req.ContextId, func(st *state.State, sb basic.Creatable) error {
		linkID, err = sb.CreateBlock(st, pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block:    req.Block,
			Position: req.Position,
		})
		if err != nil {
			return fmt.Errorf("link create error: %w", err)
		}
		return nil
	})
	return
}

func (s *Service) ObjectToSet(id string, source []string) error {
	return cache.DoState(s, id, func(st *state.State, b basic.CommonOperations) error {
		st.SetDetail(bundle.RelationKeySetOf, domain.StringList(source))
		return b.SetObjectTypesInState(st, []domain.TypeKey{bundle.TypeKeySet}, true)
	})
}
