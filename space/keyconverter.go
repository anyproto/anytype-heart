package space

import (
	"context"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func (s *space) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	// if spaceId == addr.AnytypeMarketplaceWorkspace {
	// 	return addr.BundledRelationURLPrefix + key.String(), nil
	// }
	return s.DeriveObjectID(ctx, uk)
}

func (s *space) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	// if spaceId == addr.AnytypeMarketplaceWorkspace {
	// 	return addr.BundledObjectTypeURLPrefix + key.String(), nil
	// }
	//
	// spc, err := s.spaceService.Get(ctx, spaceId)
	// if err != nil {
	// 	return "", fmt.Errorf("get space: %w", err)
	// }
	return s.DeriveObjectID(ctx, uk)
}
