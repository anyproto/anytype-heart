package objectcreator

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) createChat(ctx context.Context, space clientspace.Space, details *domain.Details) (string, *domain.Details, error) {
	createState := state.NewDoc("", nil).(*state.State)
	details.Set(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_chat)))
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChat}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
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
