package clientspace

import (
	"context"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/commonspace"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type TechSpace struct {
	*space
	techspace.TechSpace
}

type TechSpaceDeps struct {
	CommonSpace     commonspace.Space
	ObjectFactory   objectcache.ObjectFactory
	AccountService  accountservice.Service
	PersonalSpaceId string
	Indexer         spaceIndexer
	Installer       bundledObjectsInstaller
	TechSpace       techspace.TechSpace
}

func NewTechSpace(deps TechSpaceDeps) *TechSpace {
	sp := &TechSpace{
		space: &space{
			indexer:                deps.Indexer,
			installer:              deps.Installer,
			common:                 deps.CommonSpace,
			loadMandatoryObjectsCh: make(chan struct{}),
			personalSpaceId:        deps.PersonalSpaceId,
			myIdentity:             deps.AccountService.Account().SignKey.GetPublic(),
		},
		TechSpace: deps.TechSpace,
	}
	sp.Cache = objectcache.New(deps.AccountService, deps.ObjectFactory, deps.PersonalSpaceId, sp)
	return sp
}

func (s *TechSpace) Close(ctx context.Context) error {
	if s == nil || s.space == nil {
		return nil
	}
	err := s.space.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	err = s.TechSpace.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	return nil
}

func (s *TechSpace) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	if key == bundle.TypeKeyProfile {
		return key.BundledURL(), nil
	}
	return s.space.GetTypeIdByKey(ctx, key)
}
