package block

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"

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
	"github.com/anyproto/anytype-heart/space/spaceinfo"

	"go.uber.org/zap"
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

func (s *Service) CreateOneToOneFromInbox(ctx context.Context, identityProfileWithKey *model.IdentityProfileWithKey, inviteSentStatus spaceinfo.OneToOneInboxSentStatus) (spaceID string, startingPageId string, err error) {
	key, err := crypto.UnmarshallAESKeyProto(identityProfileWithKey.RequestMetadata)
	if err != nil {
		return "", "", fmt.Errorf("unmarshal RequestMetadata: %w", err)
	}

	// TODO: send encrypted rawProfile in inbox, with key?
	err = s.identityService.AddIdentityProfile(identityProfileWithKey.IdentityProfile, key)
	if err != nil {
		return "", "", fmt.Errorf("addIdentityProfile: %w", err)
	}

	requestMetadataKeyStr := base64.StdEncoding.EncodeToString(identityProfileWithKey.RequestMetadata)
	spaceDescription := &spaceinfo.SpaceDescription{
		Name:                       identityProfileWithKey.IdentityProfile.Name,
		IconImage:                  identityProfileWithKey.IdentityProfile.IconCid,
		SpaceUxType:                model.SpaceUxType_OneToOne,
		OneToOneIdentity:           identityProfileWithKey.IdentityProfile.Identity,
		OneToOneRequestMetadataKey: requestMetadataKeyStr,
		OneToOneInboxSentStatus:    inviteSentStatus,
	}

	newSpace, err := s.spaceService.CreateOneToOne(ctx, spaceDescription, identityProfileWithKey)
	if err != nil {
		return "", "", fmt.Errorf("createOneToOneFromInbox: %w", err)
	}
	err = s.spaceService.TechSpace().SpaceViewSetData(ctx, newSpace.Id(),
		domain.NewDetails().
			SetString(bundle.RelationKeyName, identityProfileWithKey.IdentityProfile.Name).
			SetString(bundle.RelationKeyIconImage, identityProfileWithKey.IdentityProfile.IconCid).
			SetInt64(bundle.RelationKeyOneToOneInboxSentStatus, int64(inviteSentStatus)))
	if err != nil {
		return "", "", fmt.Errorf("onetoone, SpaceViewSetData  %s: %w", newSpace.Id(), err)
	}

	predefinedObjectIDs := newSpace.DerivedIDs()

	details := []domain.Detail{
		{Key: bundle.RelationKeySpaceUxType, Value: domain.Float64(float64(model.SpaceUxType_OneToOne))},
		{Key: bundle.RelationKeyName, Value: domain.String(identityProfileWithKey.IdentityProfile.Name)},
		{Key: bundle.RelationKeyIconImage, Value: domain.String(identityProfileWithKey.IdentityProfile.IconCid)},
		{Key: bundle.RelationKeyIconOption, Value: domain.Float64(float64(5))},
		{Key: bundle.RelationKeyOneToOneIdentity, Value: domain.String(identityProfileWithKey.IdentityProfile.Identity)},
		{Key: bundle.RelationKeyOneToOneRequestMetadataKey, Value: domain.String(requestMetadataKeyStr)},
		{Key: bundle.RelationKeyOneToOneInboxSentStatus, Value: domain.Int64(int64(inviteSentStatus))},
		{Key: bundle.RelationKeySpaceDashboardId, Value: domain.String("lastOpened")},
	}

	err = cache.Do(s, predefinedObjectIDs.Workspace, func(b basic.DetailsSettable) error {
		return b.SetDetails(nil, details, true)
	})

	if err != nil {
		return "", "", fmt.Errorf("set details for space %s: %w", newSpace.Id(), err)
	}

	workspaceId := newSpace.DerivedIDs().Workspace
	chatUk, err := domain.NewUniqueKey(coresb.SmartBlockTypeChatDerivedObject, workspaceId)
	if err != nil {
		return
	}

	chatId, err := newSpace.DeriveObjectID(context.Background(), chatUk)
	if err != nil {
		return "", "", fmt.Errorf("onetoone, failed to derive chatId for space %s: %w", newSpace.Id(), err)
	}

	return newSpace.Id(), chatId, nil
}

func (s *Service) CreateOneToOneFromLink(ctx context.Context, spaceDescription spaceinfo.SpaceDescription) (spaceID string, startingPageId string, err error) {
	if spaceDescription.OneToOneIdentity == "" {
		return "", "", fmt.Errorf("createWorkspace: failed to decode onetoone from Identity+RequestMetadata: identity is empty")
	}
	requestMetadataKeyBytes, err := base64.StdEncoding.DecodeString(spaceDescription.OneToOneRequestMetadataKey)
	if err != nil {
		return "", "", fmt.Errorf("createWorkspace: failed to decode onetoone RequestMetadata: %w", err)
	}

	identityProfileWithKey := model.IdentityProfileWithKey{
		IdentityProfile: &model.IdentityProfile{
			Identity: spaceDescription.OneToOneIdentity,
		},
		RequestMetadata: requestMetadataKeyBytes,
	}

	spaceID, startingPageId, err = s.CreateOneToOneFromInbox(ctx, &identityProfileWithKey, spaceinfo.OneToOneInboxSentStatusToSend)
	if err != nil {
		return "", "", fmt.Errorf("createWorkspace: failed to CreateOneToOneFromInbox: %w", err)
	}

	err = s.onetoone.ResendFailedOneToOneInvites(ctx)
	if err != nil {
		log.Error("failed to reschedule onetoone inbox resend", zap.Error(err))
	}

	return spaceID, startingPageId, nil

}
func (s *Service) CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, startingPageId string, err error) {
	spaceDetails := domain.NewDetailsFromProto(req.Details)
	spaceDescription := spaceinfo.NewSpaceDescriptionFromDetails(spaceDetails)

	// when RequestMetadataKey is passed it means we create from a deeplink / QR code
	if spaceDescription.OneToOneRequestMetadataKey != "" {
		return s.CreateOneToOneFromLink(ctx, spaceDescription)
	}

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
	if spaceDescription.SpaceUxType != model.SpaceUxType_OneToOne {
		startingPageId, _, err = s.builtinObjectService.CreateObjectsForUseCase(nil, newSpace.Id(), req.UseCase)
		if err != nil {
			return "", "", fmt.Errorf("import use-case: %w", err)
		}
	} else {
		workspaceId := newSpace.DerivedIDs().Workspace
		chatUk, err := domain.NewUniqueKey(coresb.SmartBlockTypeChatDerivedObject, workspaceId)
		if err != nil {
			return "", "", err
		}

		chatId, err := newSpace.DeriveObjectID(context.Background(), chatUk)
		if err != nil {
			return "", "", err
		}
		startingPageId = chatId
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
