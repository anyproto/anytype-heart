package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) AddChatDerivedObject(ctx context.Context, space clientspace.Space, chatObjectId string) (chatId string, err error) {
	chatDetails := &types.Struct{Fields: map[string]*types.Value{}}
	chatUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, chatObjectId)
	if err != nil {
		return "", fmt.Errorf("create payload: %w", err)
	}
	chatDetails.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(chatUniqueKey.Marshal())

	chatReq := CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyChatDerived,
		Details:       chatDetails,
	}

	chatId, _, err = s.createObjectInSpace(ctx, space, chatReq)
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}

	return chatId, nil
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
