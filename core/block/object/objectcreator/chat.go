package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) createChat(ctx context.Context, space clientspace.Space, details *domain.Details) (string, *domain.Details, error) {
	payload, err := space.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
		Time:           time.Now(),
		SmartblockType: smartblock.SmartBlockTypeChatObject,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create tree payload: %w", err)
	}

	createState := state.NewDoc(payload.RootRawChange.Id, nil).(*state.State)
	details.Set(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_chat)))
	createState.SetDetails(details)
	err = s.addChatDerivedObject(ctx, space, createState, payload.RootRawChange.Id)
	if err != nil {
		return "", nil, fmt.Errorf("add chat derived object: %w", err)
	}

	id, newDetails, err := s.CreateSmartBlockFromStateInSpaceWithOptions(ctx, space, []domain.TypeKey{bundle.TypeKeyChat}, createState, WithPayload(&payload))
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}

func (s *service) addChatDerivedObject(ctx context.Context, space clientspace.Space, st *state.State, chatObjectId string) error {
	chatDetails := domain.NewDetails()
	chatUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, chatObjectId)
	if err != nil {
		return fmt.Errorf("create payload: %w", err)
	}
	chatDetails.SetString(bundle.RelationKeyUniqueKey, chatUniqueKey.Marshal())

	chatReq := CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyChatDerived,
		Details:       chatDetails,
	}

	chatId, _, err := s.createObjectInSpace(ctx, space, chatReq)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}

	st.SetDetailAndBundledRelation(bundle.RelationKeyChatId, domain.String(chatId))
	st.SetDetailAndBundledRelation(bundle.RelationKeyHasChat, domain.Bool(true))
	return nil
}

func (s *service) createChatDerived(ctx context.Context, space clientspace.Space, details *domain.Details) (string, *domain.Details, error) {
	uniqueKey := details.GetString(bundle.RelationKeyUniqueKey)
	key, err := domain.UnmarshalUniqueKey(uniqueKey)
	if err != nil {
		return "", nil, fmt.Errorf("unmarshal unique key: %w", err)
	}

	createState := state.NewDocWithUniqueKey("", nil, key).(*state.State)
	details.Set(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_chatDerived)))
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChatDerived}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}
