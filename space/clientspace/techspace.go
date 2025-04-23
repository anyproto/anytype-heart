package clientspace

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/commonspace"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace/keyvalueservice"
	"github.com/anyproto/anytype-heart/space/spacecore/keyvalueobserver"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type TechSpace struct {
	*space
	techspace.TechSpace
}

type TechSpaceDeps struct {
	CommonSpace      commonspace.Space
	ObjectFactory    objectcache.ObjectFactory
	AccountService   accountservice.Service
	PersonalSpaceId  string
	Indexer          spaceIndexer
	Installer        bundledObjectsInstaller
	TechSpace        techspace.TechSpace
	KeyValueObserver keyvalueobserver.Observer
}

func NewTechSpace(deps TechSpaceDeps) (*TechSpace, error) {
	sp := &TechSpace{
		space: &space{
			indexer:                deps.Indexer,
			installer:              deps.Installer,
			common:                 deps.CommonSpace,
			loadMandatoryObjectsCh: make(chan struct{}),
			personalSpaceId:        deps.PersonalSpaceId,
			aclIdentity:            deps.AccountService.Account().SignKey.GetPublic(),
		},
		TechSpace: deps.TechSpace,
	}
	var err error
	sp.keyValueService, err = keyvalueservice.New(sp.common, deps.KeyValueObserver)
	if err != nil {
		return nil, fmt.Errorf("create key value service: %w", err)
	}
	sp.Cache = objectcache.New(deps.AccountService, deps.ObjectFactory, deps.PersonalSpaceId, sp)
	return sp, nil
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

func (s *TechSpace) CommonSpace() commonspace.Space {
	return s.space.CommonSpace()
}
