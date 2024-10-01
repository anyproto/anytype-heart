package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createChatWithPayload(ctx context.Context, space clientspace.Space, details *types.Struct, payload treestorage.TreeStorageCreatePayload) (string, *types.Struct, error) {
	createState := state.NewDoc(payload.RootRawChange.Id, nil).(*state.State)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_chat))
	createState.SetDetails(details)
	err := s.addChatDerivedObject(ctx, space, createState, payload.RootRawChange.Id)
	if err != nil {
		return "", nil, fmt.Errorf("add chat derived object: %w", err)
	}

	id, newDetails, err := s.CreateSmartBlockFromStateInSpaceWithOptions(ctx, space, []domain.TypeKey{bundle.TypeKeyChat}, createState, WithPayload(&payload))
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}

func (s *service) createChat(ctx context.Context, space clientspace.Space, details *types.Struct) (string, *types.Struct, error) {
	payload, err := space.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
		Time:           time.Now(),
		SmartblockType: smartblock.SmartBlockTypeChatObject,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create tree payload: %w", err)
	}
	return s.createChatWithPayload(ctx, space, details, payload)
}

func (s *service) addChatDerivedObject(ctx context.Context, space clientspace.Space, st *state.State, chatObjectId string) error {
	chatDetails := &types.Struct{Fields: map[string]*types.Value{}}
	chatUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, chatObjectId)
	if err != nil {
		return fmt.Errorf("create payload: %w", err)
	}
	chatDetails.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(chatUniqueKey.Marshal())

	chatReq := CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyChatDerived,
		Details:       chatDetails,
	}

	chatId, _, err := s.createObjectInSpace(ctx, space, chatReq)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}

	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceMainChatId, pbtypes.String(chatId))
	st.SetDetailAndBundledRelation(bundle.RelationKeyHasChat, pbtypes.Bool(true))
	return nil
}

func (s *service) createChatDerived(ctx context.Context, space clientspace.Space, details *types.Struct) (string, *types.Struct, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	key, err := domain.UnmarshalUniqueKey(uniqueKey)
	if err != nil {
		return "", nil, fmt.Errorf("unmarshal unique key: %w", err)
	}

	createState := state.NewDocWithUniqueKey("", nil, key).(*state.State)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_chatDerived))
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChatDerived}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}
