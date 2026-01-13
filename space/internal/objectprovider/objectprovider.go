package objectprovider

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

var log = logger.NewNamed("client.spaceobject.objectprovider")

type ObjectProvider interface {
	DeriveObjectIDs(ctx context.Context) (objIDs threads.DerivedSmartblockIds, err error)
	LoadObjects(ctx context.Context, ids []string) (err error)
	LoadObjectsIgnoreErrs(ctx context.Context, objIDs []string)
	CreateMandatoryObjects(ctx context.Context, space smartblock.Space) (err error)
}

func NewObjectProvider(spaceId string, personalSpaceId string, cache objectcache.Cache) ObjectProvider {
	return &objectProvider{
		spaceId:         spaceId,
		personalSpaceId: personalSpaceId,
		cache:           cache,
	}
}

type objectProvider struct {
	personalSpaceId string
	spaceId         string
	cache           objectcache.Cache

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

	var sbTypes []coresb.SmartBlockType
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
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, ot.String())
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
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, rk.String())
		if err != nil {
			return objIDs, err
		}
		id, err := o.cache.DeriveObjectID(ctx, uk)
		if err != nil {
			return objIDs, err
		}
		objIDs.SystemRelations[rk] = id
	}
	if !o.isPersonal() {
		chatUk, err := domain.NewUniqueKey(coresb.SmartBlockTypeChatDerivedObject, objIDs.Workspace)
		if err == nil {
			objIDs.SpaceChat, err = o.cache.DeriveObjectID(context.Background(), chatUk)
			if err != nil {
				log.WarnCtx(ctx, "failed to derive chat id", zap.Error(err), zap.String("spaceId", o.spaceId), zap.String("uk", chatUk.Marshal()))
			}
		}
	}
	o.derivedObjectIds = objIDs
	return
}

func (o *objectProvider) LoadObjects(ctx context.Context, objIDs []string) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	results := o.loadObjectsAsync(ctx, objIDs)
	for i := 0; i < len(objIDs); i++ {
		if err := <-results; err != nil {
			cancel()
			return err
		}
	}
	return nil
}

func (o *objectProvider) LoadObjectsIgnoreErrs(ctx context.Context, objIDs []string) {
	results := o.loadObjectsAsync(ctx, objIDs)
	for i := 0; i < len(objIDs); i++ {
		if err := <-results; err != nil {
			log.WarnCtx(ctx, "can't load object", zap.Error(err))
		}
	}
}

func (o *objectProvider) loadObjectsAsync(ctx context.Context, objIDs []string) (results chan error) {
	results = make(chan error, len(objIDs))

	go func() {
		var limiter = make(chan struct{}, 10)
		for _, id := range objIDs {
			select {
			case <-ctx.Done():
				log.WarnCtx(ctx, "loadObjectsAsync context done", zap.Error(ctx.Err()), zap.String("spaceId", o.spaceId), zap.String("objectId", id))

				results <- ctx.Err()
				continue
			case limiter <- struct{}{}:
			}
			go func(id string) {
				defer func() {
					<-limiter
				}()
				_, err := o.cache.GetObject(ctx, id)
				if err != nil {
					log.WarnCtx(ctx, "loadObjectsAsync failed", zap.Error(err), zap.String("spaceId", o.spaceId), zap.String("objectId", id))

					// we had a bug that allowed some users to remove their profile
					// this workaround is to allow these users to load their accounts without errors and export their anytype data
					if id == o.derivedObjectIds.Profile {
						if errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) || errors.Is(err, treechangeproto.ErrGetTree) {
							log.Error("load profile error", zap.Error(err), zap.String("objectID", id), zap.String("spaceId", o.spaceId))
							err = nil
						}
					}
				}
				results <- err
			}(id)
		}
	}()
	return results
}

func (o *objectProvider) CreateMandatoryObjects(ctx context.Context, space smartblock.Space) (err error) {
	log = log.With(zap.String("spaceId", o.spaceId))

	var sbTypes []coresb.SmartBlockType
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
			InitFunc: func(id string) *smartblock.InitContext {
				return &smartblock.InitContext{
					Ctx:     ctx,
					SpaceID: o.spaceId,
					State:   state.NewDoc(id, nil).(*state.State),
				}
			},
		})
		if err != nil {
			if errors.Is(err, treestorage.ErrTreeExists) {
				log.Info("tree object already exists", zap.String("uniqueKey", uk.Marshal()))
				return nil
			}
			log.Error("create payload for derived object", zap.Error(err), zap.String("uniqueKey", uk.Marshal()))
			return fmt.Errorf("derive tree object: %w", err)
		}
	}

	err = space.Do(space.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyAnalyticsSpaceId, domain.String(metrics.GenerateAnalyticsId()))
		return sb.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("set analytics id: %w", err)
	}

	return
}
