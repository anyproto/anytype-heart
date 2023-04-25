package block

import (
	"context"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app/ocache"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/treestorage"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"go.uber.org/zap"
	"time"
)

type ctxKey int

const (
	spaceKey ctxKey = iota
	treeCreateKey
	putObjectKey
)

type treeCreateCache struct {
	treeCreate treestorage.TreeStorageCreatePayload
	initFunc   InitFunc
}

type InitFunc = func(id string) *smartblock.InitContext

func (s *Service) createCache() ocache.OCache {
	return ocache.New(
		func(ctx context.Context, id string) (value ocache.Object, err error) {
			spaceId := ctx.Value(spaceKey).(string)
			spc, err := s.clientService.GetSpace(ctx, spaceId)
			if err != nil {
				return
			}
			// creating tree if needed
			createPayload, exists := ctx.Value(treeCreateKey).(treeCreateCache)
			if exists {
				var ot objecttree.ObjectTree
				ot, err = spc.PutTree(ctx, createPayload.treeCreate, nil)
				if err != nil {
					return
				}
				ot.Close()
				return s.objectFactory.InitObject(id, createPayload.initFunc(id))
			}
			// putting object through cache
			putObject, exists := ctx.Value(putObjectKey).(smartblock.SmartBlock)
			if exists {
				return putObject, nil
			}
			// if it is subObject
			if sbt, _ := coresb.SmartBlockTypeFromID(id); sbt == coresb.SmartBlockTypeSubObject {
				return s.initSubObject(ctx, id)
			}
			// otherwise general init
			return s.objectFactory.InitObject(id, &smartblock.InitContext{
				Ctx: ctx,
			})
		},
		//ocache.WithLogger(log.Desugar()),
		ocache.WithRefCounter(true),
		ocache.WithGCPeriod(time.Minute),
		// TODO: [MR] Get ttl from config
		ocache.WithTTL(time.Duration(60)*time.Second),
	)
}

func (s *Service) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(objecttree.ObjectTree), nil
}

func (s *Service) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, release func(), err error) {
	ctx = context.WithValue(ctx, spaceKey, s.clientService.AccountId())
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), func() {
		s.cache.Release(id)
	}, nil
}

func (s *Service) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	tr, _, err := s.GetObject(ctx, treeId)
	if err != nil {
		return
	}
	err = s.OnDelete(tr)
	if err != nil {
		return
	}

	err = tr.(objecttree.ObjectTree).Delete()
	if err != nil {
		return
	}
	_, err = s.cache.Remove(treeId)
	return
}

func (s *Service) DeleteObject(id string) (err error) {
	err = s.Do(id, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return
	}
	space, err := s.clientService.AccountSpace(context.Background())
	if err != nil {
		return
	}
	// this will call DeleteTree in the end
	return space.DeleteTree(context.Background(), id)
}

func (s *Service) CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, release func(), err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return
	}
	payload := objecttree.ObjectTreeCreatePayload{
		SignKey:     s.commonAccount.Account().SignKey,
		ChangeType:  tp.ToProto().String(),
		SpaceId:     space.Id(),
		Identity:    s.commonAccount.Account().Identity,
		IsEncrypted: true,
	}
	create, err := space.CreateTree(context.Background(), payload)
	if err != nil {
		return
	}
	return s.cacheCreatedObject(ctx, space.Id(), initFunc, create)
}

func (s *Service) DeriveTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, release func(), err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return
	}
	log.With(zap.String("sbt", tp.ToProto().String())).Warn("deriving smartblock")
	payload := objecttree.ObjectTreeCreatePayload{
		SignKey:     s.commonAccount.Account().SignKey,
		ChangeType:  tp.ToProto().String(),
		SpaceId:     space.Id(),
		Identity:    s.commonAccount.Account().Identity,
		IsEncrypted: true,
	}
	create, err := space.DeriveTree(context.Background(), payload)
	if err != nil {
		return
	}
	return s.cacheCreatedObject(ctx, space.Id(), initFunc, create)
}

func (s *Service) DeriveObject(ctx context.Context, tp coresb.SmartBlockType) (id string, err error) {
	obj, release, err := s.DeriveTreeObject(ctx, tp, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{Ctx: ctx, State: state.NewDoc(id, nil).(*state.State)}
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("derived object with error")
		return
	}
	defer release()
	return obj.Id(), nil
}

func (s *Service) PutObject(ctx context.Context, id string, obj smartblock.SmartBlock) (sb smartblock.SmartBlock, release func(), err error) {
	ctx = context.WithValue(ctx, putObjectKey, obj)
	return s.GetObject(ctx, id)
}

func (s *Service) cacheCreatedObject(ctx context.Context, spaceId string, initFunc InitFunc, create treestorage.TreeStorageCreatePayload) (sb smartblock.SmartBlock, release func(), err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	ctx = context.WithValue(ctx, treeCreateKey, treeCreateCache{
		treeCreate: create,
		initFunc:   initFunc,
	})
	id := create.RootRawChange.Id
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), func() {
		s.cache.Release(id)
	}, nil
}

func (s *Service) initSubObject(ctx context.Context, id string) (account ocache.Object, err error) {
	accountId := s.clientService.AccountId()
	if account, err = s.cache.Get(ctx, accountId); err != nil {
		return
	}
	return account.(SmartblockOpener).Open(id)
}
