package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/cache"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func (s *Service) CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, err error) {
	chatUniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeChatObject, "")
	if err != nil {
		return "", fmt.Errorf("chat unique key creation: %w", err)
	}

	newSpace, err := s.spaceService.Create(ctx)
	if err != nil {
		return "", fmt.Errorf("error creating space: %w", err)
	}
	predefinedObjectIDs := newSpace.DerivedIDs()

	err = cache.Do(s, predefinedObjectIDs.Workspace, func(b basic.DetailsSettable) error {
		details := make([]*model.Detail, 0, len(req.Details.GetFields()))
		for k, v := range req.Details.GetFields() {
			details = append(details, &model.Detail{
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

	chatName := pbtypes.GetString(req.Details, bundle.RelationKeyName.String())
	if chatName == "" {
		chatName = "Space"
	}

	chatName += " chat"
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String(): pbtypes.String(chatName),
	}}

	chatId, _, err := s.objectCreator.CreateChatWithUniqueKey(ctx, newSpace.Id(), chatUniqueKey, details)
	if err != nil {
		return spaceID, fmt.Errorf("create chat: %w", err)
	}
	err = cache.Do(s, predefinedObjectIDs.Workspace, func(b basic.DetailsSettable) error {
		return b.SetDetails(nil, []*model.Detail{
			{
				Key:   bundle.RelationKeyChatId.String(),
				Value: pbtypes.String(chatId),
			}}, true)
	})

	return newSpace.Id(), err
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(
	ctx context.Context,
	sctx session.Context,
	req *pb.RpcBlockLinkCreateWithObjectRequest,
) (linkID string, objectId string, objectDetails *types.Struct, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}

	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(req.ObjectTypeUniqueKey)
	if err != nil {
		return "", "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}

	createReq := objectcreator.CreateObjectRequest{
		Details:       req.Details,
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
		st.SetDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		return b.SetObjectTypesInState(st, []domain.TypeKey{bundle.TypeKeySet}, true)
	})
}
