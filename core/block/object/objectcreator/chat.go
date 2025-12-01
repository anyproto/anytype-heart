package objectcreator

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) AddChatDerivedObject(ctx context.Context, space clientspace.Space, chatObjectId string) (chatId string, err error) {
	chatDetails := domain.NewDetails()
	chatUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, chatObjectId)
	if err != nil {
		return "", fmt.Errorf("create payload: %w", err)
	}
	chatDetails.SetString(bundle.RelationKeyUniqueKey, chatUniqueKey.Marshal())

	chatId, _, err = s.createChatDerived(ctx, space, chatDetails, false)
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}

	return chatId, nil
}

func (s *service) createChatDerived(ctx context.Context, space clientspace.Space, details *domain.Details, addAnalyticsId bool) (string, *domain.Details, error) {
	uniqueKey, hasUniqueKey := details.TryString(bundle.RelationKeyUniqueKey)
	var createState *state.State
	if hasUniqueKey {
		key, err := domain.UnmarshalUniqueKey(uniqueKey)
		if err != nil {
			return "", nil, fmt.Errorf("unmarshal unique key: %w", err)
		}
		createState = state.NewDocWithUniqueKey("", nil, key).(*state.State)
	} else {
		createState = state.NewDoc("", nil).(*state.State)
	}

	details.Set(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_chatDerived)))
	if addAnalyticsId {
		details.Set(bundle.RelationKeyAnalyticsChatId, domain.String(metrics.GenerateAnalyticsId()))
	}
	details.Delete(bundle.RelationKeyInternalFlags)
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyChatDerived}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}
