package objectcreator

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) AddDiscussionDerivedObject(ctx context.Context, space clientspace.Space, parentObjectId string) (discussionId string, err error) {
	details := domain.NewDetails()
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeDiscussionObject, parentObjectId)
	if err != nil {
		return "", fmt.Errorf("create unique key: %w", err)
	}
	details.SetString(bundle.RelationKeyUniqueKey, uk.Marshal())

	discussionId, _, err = s.createDiscussion(ctx, space, details, parentObjectId)
	if err != nil {
		return "", fmt.Errorf("create discussion: %w", err)
	}
	return discussionId, nil
}

func (s *service) createDiscussion(ctx context.Context, space clientspace.Space, details *domain.Details, parentObjectId string) (string, *domain.Details, error) {
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

	details.Set(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_discussion)))
	details.Delete(bundle.RelationKeyInternalFlags)
	createState.SetDetails(details)

	id, newDetails, err := s.CreateSmartBlockFromStateInSpaceWithOptions(ctx, space, []domain.TypeKey{bundle.TypeKeyDiscussion}, createState, WithParentId(parentObjectId))
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	return id, newDetails, nil
}
