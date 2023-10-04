package objectprovider

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

var log = logger.NewNamed("client.spaceobject.objectprovider")

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
}

type ObjectProvider interface {
	DeriveObjectIDs(ctx context.Context, spaceID string, sbTypes []smartblock.SmartBlockType) (objIDs threads.DerivedSmartblockIds, err error)
	LoadObjects(ctx context.Context, spaceID string, ids []string) (err error)
	CreateMandatoryObjects(ctx context.Context, spaceID string, sbTypes []smartblock.SmartBlockType) (err error)
	InstallBundledObjects(ctx context.Context, spaceID string) error
}

func NewObjectProvider(cache objectcache.Cache, installer bundledObjectsInstaller) ObjectProvider {
	return &objectProvider{
		cache:     cache,
		installer: installer,
	}
}

type objectProvider struct {
	cache     objectcache.Cache
	installer bundledObjectsInstaller
}

func (o *objectProvider) DeriveObjectIDs(ctx context.Context, spaceID string, sbTypes []smartblock.SmartBlockType) (objIDs threads.DerivedSmartblockIds, err error) {
	objIDs.SystemRelations = make(map[domain.RelationKey]string)
	objIDs.SystemTypes = make(map[domain.TypeKey]string)
	// deriving system objects like archive etc
	for _, sbt := range sbTypes {
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return objIDs, err
		}
		id, err := o.cache.DeriveObjectID(ctx, spaceID, uk)
		if err != nil {
			return objIDs, fmt.Errorf("derive object id: %w", err)
		}
		objIDs.InsertId(sbt, id)
	}
	// deriving system types
	for _, ot := range bundle.SystemTypes {
		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, ot.String())
		if err != nil {
			return objIDs, err
		}
		id, err := o.cache.DeriveObjectID(ctx, spaceID, uk)
		if err != nil {
			return objIDs, err
		}
		objIDs.SystemTypes[ot] = id
	}
	// deriving system relations
	for _, rk := range bundle.SystemRelations {
		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, rk.String())
		if err != nil {
			return objIDs, err
		}
		id, err := o.cache.DeriveObjectID(ctx, spaceID, uk)
		if err != nil {
			return objIDs, err
		}
		objIDs.SystemRelations[rk] = id
	}
	return
}

func (o *objectProvider) LoadObjects(ctx context.Context, spaceID string, objIDs []string) (err error) {
	for _, id := range objIDs {
		_, err = o.cache.GetObject(ctx, domain.FullID{
			ObjectID: id,
			SpaceID:  spaceID,
		})
		if err != nil {
			return err
		}
	}
	return
}

func (o *objectProvider) CreateMandatoryObjects(ctx context.Context, spaceID string, sbTypes []smartblock.SmartBlockType) (err error) {
	for _, sbt := range sbTypes {
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return err
		}
		_, err = o.cache.DeriveTreeObject(ctx, spaceID, objectcache.TreeDerivationParams{
			Key: uk,
			InitFunc: func(id string) *editorsb.InitContext {
				return &editorsb.InitContext{Ctx: ctx, SpaceID: spaceID, State: state.NewDoc(id, nil).(*state.State)}
			},
		})
		if err != nil {
			log.Error("create payload for derived object", zap.Error(err), zap.String("uniqueKey", uk.Marshal()))
			return fmt.Errorf("derive tree object: %w", err)
		}
	}
	return
}

func (o *objectProvider) InstallBundledObjects(ctx context.Context, spaceID string) error {
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}
	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := o.installer.InstallBundledObjects(ctx, spaceID, ids)
	if err != nil {
		return err
	}
	return nil
}
