package space

import (
	"context"
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"sync"

	"github.com/anyproto/any-sync/commonspace"

	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

type bundledInstaller interface {
	InstallBundledObjects(
		ctx context.Context,
		spaceID string,
		sourceObjectIds []string,
	) (ids []string, objects []*types.Struct, err error)
}

// TechSpace is a space for different objects related to account
// including spaces that user creates. You can think of it as account's settings
type TechSpace struct {
	*clientSpace
	spaceService *service
	installer    bundledInstaller
	objectCache  objectcache.Cache

	derivedIDs map[string]threads.DerivedSmartblockIds
	sync.RWMutex
}

func newTechSpace(sp *clientSpace, spaceService *service) *TechSpace {
	return &TechSpace{
		clientSpace:  sp,
		spaceService: spaceService,
		installer:    spaceService.installer,
		objectCache:  spaceService.objectCache,
		derivedIDs:   make(map[string]threads.DerivedSmartblockIds),
	}
}

// SpaceDerivedIDs returns derived smartblock ids for a given space
func (t *TechSpace) SpaceDerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error) {
	t.RLock()
	ids, ok := t.derivedIDs[spaceID]
	if ok || spaceID == addr.AnytypeMarketplaceWorkspace {
		t.RUnlock()
		return
	}
	t.RUnlock()
	err = t.DoSpaceObject(ctx, spaceID, func(spaceObject spacecore.SpaceObject) error {
		ids = spaceObject.DerivedIDs()
		t.Lock()
		defer t.Unlock()
		t.derivedIDs[spaceID] = ids
		return nil
	})
	return
}

// DoSpaceObject opens a space object with given spaceID and calls openBlock
func (t *TechSpace) DoSpaceObject(ctx context.Context, spaceID string, openBlock func(spaceObject spacecore.SpaceObject) error) error {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return err
	}
	id, err := t.deriveObjectId(ctx, spaceID, uniqueKey)
	if err != nil {
		return err
	}
	obj, err := t.objectCache.PickBlock(ctx, id)
	if err != nil {
		return err
	}
	spaceObject, ok := obj.(spacecore.SpaceObject)
	if !ok {
		return fmt.Errorf("object %s is not a space object", id)
	}
	obj.Lock()
	defer obj.Unlock()
	return openBlock(spaceObject)
}

// CreateSpace creates a new space with a respective SpaceObject
func (t *TechSpace) CreateSpace(ctx context.Context) (spacecore.SpaceObject, error) {
	sp, err := t.spaceService.CreateSpace(ctx)
	if err != nil {
		return nil, err
	}
	_, err = t.PredefinedObjects(ctx, sp, true)
	if err != nil {
		return nil, err
	}
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return nil, err
	}
	obj, err := t.objectCache.DeriveTreeObject(ctx, t.Id(), objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *editorsb.InitContext {
			return &editorsb.InitContext{Ctx: ctx, SpaceID: t.Id(), State: state.NewDoc(id, nil).(*state.State)}
		},
		TargetSpaceID: sp.Id(),
	})
	if err != nil {
		return nil, err
	}
	return obj.(spacecore.SpaceObject), nil
}

// DerivePersonalSpace derives a personal space for an account
func (t *TechSpace) DerivePersonalSpace(ctx context.Context, newAccount bool) (spaceObject spacecore.SpaceObject, err error) {
	payload := commonspace.SpaceDerivePayload{
		SigningKey: t.spaceService.wallet.GetAccountPrivkey(),
		MasterKey:  t.spaceService.wallet.GetMasterKey(),
		SpaceType:  SpaceType,
	}
	var sp commonspace.Space
	if newAccount { //nolint:nestif
		spaceID, err := t.spaceService.commonSpace.DeriveSpace(ctx, payload)
		if err != nil {
			return nil, err
		}
		sp, err = t.spaceService.GetSpace(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		t.spaceService.accountId = sp.Id()
	} else {
		spaceID, err := t.spaceService.commonSpace.DeriveId(ctx, payload)
		if err != nil {
			return nil, err
		}
		sp, err = t.spaceService.GetSpace(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		t.spaceService.accountId = sp.Id()
	}
	ids, err := t.PredefinedObjects(ctx, sp, false)
	if err != nil {
		return nil, err
	}
	t.Lock()
	t.derivedIDs[sp.Id()] = ids
	t.Unlock()
	if newAccount {
		_, err = t.PredefinedObjects(ctx, sp, true)
		if err != nil {
			return nil, err
		}
	}
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return nil, err
	}
	// TODO: [MR] should we try loading the object before deriving it
	obj, err := t.objectCache.DeriveTreeObject(ctx, t.Id(), objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *editorsb.InitContext {
			return &editorsb.InitContext{Ctx: ctx, SpaceID: t.Id(), State: state.NewDoc(id, nil).(*state.State)}
		},
		TargetSpaceID: sp.Id(),
	})
	if err != nil {
		return nil, err
	}
	return obj.(spacecore.SpaceObject), nil
}

// deriveObjectId derives object id from a given key
func (t *TechSpace) deriveObjectId(ctx context.Context, spaceID string, key domain.UniqueKey) (string, error) {
	return t.objectCache.DeriveObjectId(ctx, spaceID, key)
}

// PredefinedObjects creates mandatory objects for a given space
func (t *TechSpace) PredefinedObjects(ctx context.Context, sp commonspace.Space, create bool) (objIDs threads.DerivedSmartblockIds, err error) {
	log := log.With(zap.String("spaceId", sp.Id()))
	sbTypes := []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeWorkspace,
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeWidget,
		smartblock.SmartBlockTypeHome,
	}
	if t.spaceService.AccountId() == sp.Id() {
		sbTypes = append(sbTypes, smartblock.SmartBlockTypeProfilePage)
	}
	objIDs.SystemRelations = make(map[domain.RelationKey]string)
	objIDs.SystemTypes = make(map[domain.TypeKey]string)

	// deriving system objects like archive etc
	for _, sbt := range sbTypes {
		// we have only 1 object per sbtype so key is empty (also for the backward compatibility, because before we didn't have a key)
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return objIDs, err
		}
		if create {
			obj, err := t.objectCache.DeriveTreeObject(ctx, sp.Id(), objectcache.TreeDerivationParams{
				Key: uk,
				InitFunc: func(id string) *editorsb.InitContext {
					return &editorsb.InitContext{Ctx: ctx, SpaceID: sp.Id(), State: state.NewDoc(id, nil).(*state.State)}
				},
			})
			if err != nil {
				log.Error("create payload for derived object", zap.Error(err), zap.String("uniqueKey", uk.Marshal()))
				return objIDs, fmt.Errorf("derive tree object: %w", err)
			}
			objIDs.InsertId(sbt, obj.Id())
		} else {
			id, err := t.deriveObjectId(ctx, sp.Id(), uk)
			if err != nil {
				return objIDs, fmt.Errorf("derive object id: %w", err)
			}
			objIDs.InsertId(sbt, id)
		}
	}
	// deriving system types
	for _, ot := range bundle.SystemTypes {
		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, ot.String())
		if err != nil {
			return objIDs, err
		}
		id, err := t.deriveObjectId(ctx, sp.Id(), uk)
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
		id, err := t.deriveObjectId(ctx, sp.Id(), uk)
		if err != nil {
			return objIDs, err
		}
		objIDs.SystemRelations[rk] = id
	}
	return
}

// PreinstalledObjects installs bundled objects for a given space
func (t *TechSpace) PreinstalledObjects(ctx context.Context, spaceID string) error {
	//start := time.Now()
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}

	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := t.installer.InstallBundledObjects(ctx, spaceID, ids)
	if err != nil {
		return err
	}
	// TODO: [MR] think how to add metrics here
	//i.logFinishedReindexStat(metrics.ReindexTypeSystem, len(ids), len(ids), time.Since(start))
	return nil
}
