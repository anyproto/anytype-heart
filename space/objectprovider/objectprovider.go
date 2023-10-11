package objectprovider

import (
	"context"
	"fmt"
	"sync"

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
	DeriveObjectIDs(ctx context.Context) (objIDs threads.DerivedSmartblockIds, err error)
	LoadObjects(ctx context.Context, ids []string) (err error)
	CreateMandatoryObjects(ctx context.Context) (err error)
	InstallBundledObjects(ctx context.Context) error
}

func NewObjectProvider(spaceId string, personalSpaceId string, cache objectcache.Cache, installer bundledObjectsInstaller) ObjectProvider {
	return &objectProvider{
		spaceId:         spaceId,
		personalSpaceId: personalSpaceId,
		cache:           cache,
		installer:       installer,
	}
}

type objectProvider struct {
	personalSpaceId string
	spaceId         string
	cache           objectcache.Cache
	installer       bundledObjectsInstaller

	mu               sync.Mutex
	derivedObjectIds threads.DerivedSmartblockIds
}

func (o *objectProvider) isPersonal() bool {
	return o.personalSpaceId == o.spaceId
}

func (o *objectProvider) DeriveObjectIDs(ctx context.Context) (objIDs threads.DerivedSmartblockIds, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.derivedObjectIds.IsFilled() {
		return o.derivedObjectIds, nil
	}

	var sbTypes []smartblock.SmartBlockType
	if o.isPersonal() {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}

	objIDs.SystemRelations = make(map[domain.RelationKey]string)
	objIDs.SystemTypes = make(map[domain.TypeKey]string)
	// deriving system objects like archive etc
	for _, sbt := range sbTypes {
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return objIDs, err
		}
		id, err := o.cache.DeriveObjectID(ctx, uk)
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
		id, err := o.cache.DeriveObjectID(ctx, uk)
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
		id, err := o.cache.DeriveObjectID(ctx, uk)
		if err != nil {
			return objIDs, err
		}
		objIDs.SystemRelations[rk] = id
	}
	o.derivedObjectIds = objIDs
	return
}

func (o *objectProvider) LoadObjects(ctx context.Context, objIDs []string) (err error) {
	for _, id := range objIDs {
		_, err = o.cache.GetObject(ctx, id)
		if err != nil {
			return err
		}
	}
	return
}

func (o *objectProvider) CreateMandatoryObjects(ctx context.Context) (err error) {
	var sbTypes []smartblock.SmartBlockType
	if o.isPersonal() {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}

	for _, sbt := range sbTypes {
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return err
		}
		_, err = o.cache.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
			Key: uk,
			InitFunc: func(id string) *editorsb.InitContext {
				return &editorsb.InitContext{Ctx: ctx, SpaceID: o.spaceId, State: state.NewDoc(id, nil).(*state.State)}
			},
		})
		if err != nil {
			log.Error("create payload for derived object", zap.Error(err), zap.String("uniqueKey", uk.Marshal()))
			return fmt.Errorf("derive tree object: %w", err)
		}
	}
	return
}

func (o *objectProvider) InstallBundledObjects(ctx context.Context) error {
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}
	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := o.installer.InstallBundledObjects(ctx, o.spaceId, ids)
	if err != nil {
		return err
	}
	return nil
}
