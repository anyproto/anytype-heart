package space

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

type marketplaceSpace struct {
	*VirtualSpace
}

type builtinTemplateService interface {
	RegisterBuiltinTemplates(space Space) error
}

func (s *service) initMarketplaceSpace() error {
	vs := NewVirtualSpace(s, addr.AnytypeMarketplaceWorkspace)
	marketplace := &marketplaceSpace{vs}
	marketplace.Cache = objectcache.New(s.accountService, s.objectFactory, s.personalSpaceID, marketplace)
	s.preLoad(marketplace)

	err := s.virtualSpace.RegisterVirtualSpace(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return fmt.Errorf("register virtual space: %w", err)
	}
	err = s.builtinTemplateService.RegisterBuiltinTemplates(marketplace)
	if err != nil {
		return fmt.Errorf("register builtin templates: %w", err)
	}
	err = s.indexer.ReindexMarketplaceSpace(marketplace)
	if err != nil {
		return fmt.Errorf("reindex marketplace space: %w", err)
	}
	s.marketplaceSpace = marketplace
	return nil
}

func (s *marketplaceSpace) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	return addr.BundledRelationURLPrefix + key.String(), nil
}

func (s *marketplaceSpace) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	return addr.BundledObjectTypeURLPrefix + key.String(), nil
}
