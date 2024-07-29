package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createChat(ctx context.Context, space clientspace.Space, details *types.Struct) (string, *types.Struct, error) {
	createState := state.NewDoc("", nil).(*state.State)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.String(model.ObjectType_chat.String())
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChat}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}

func (s *service) createChatDerived(ctx context.Context, space clientspace.Space, details *types.Struct) (string, *types.Struct, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	key, err := domain.UnmarshalUniqueKey(uniqueKey)
	if err != nil {
		return "", nil, fmt.Errorf("unmarshal unique key: %w", err)
	}

	createState := state.NewDocWithUniqueKey("", nil, key).(*state.State)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.String(model.ObjectType_chatDerived.String())
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChatDerived}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}
