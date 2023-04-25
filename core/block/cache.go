package block

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type ctxKey int

var errAppIsNotRunning = errors.New("app is not running")

const (
	optsKey ctxKey = iota
)

type treeCreateCache struct {
	treeCreate treestorage.TreeStorageCreatePayload
	initFunc   InitFunc
}

type cacheOpts struct {
	spaceId      string
	createOption *treeCreateCache
	buildOption  commonspace.BuildTreeOpts
	putObject    smartblock.SmartBlock

	waitRemote bool
}

type InitFunc = func(id string) *smartblock.InitContext

func (s *Service) createCache() ocache.OCache {
	return ocache.New(
		s.cacheLoad,
		// ocache.WithLogger(log.Desugar()),
		ocache.WithGCPeriod(time.Minute),
		// TODO: [MR] Get ttl from config
		ocache.WithTTL(time.Duration(60)*time.Second),
	)
}

func (s *Service) cacheLoad(ctx context.Context, id string) (value ocache.Object, err error) {
	opts := ctx.Value(optsKey).(cacheOpts)
	spc, err := s.clientService.GetSpace(ctx, opts.spaceId)
	if err != nil {
		return
	}

	buildTreeObject := func(id string) (sb smartblock.SmartBlock, err error) {
		var ot objecttree.ObjectTree
		ot, err = spc.BuildTree(ctx, id, opts.buildOption)
		if err != nil {
			return
		}
		return s.objectFactory.InitObject(id, &smartblock.InitContext{Ctx: ctx, ObjectTree: ot})
	}
	createTreeObject := func(ot objecttree.ObjectTree) (sb smartblock.SmartBlock, err error) {
		initCtx := opts.createOption.initFunc(id)
		initCtx.ObjectTree = ot
		return s.objectFactory.InitObject(id, initCtx)
	}

	switch {
	case opts.createOption != nil:
		// creating tree if needed
		var ot objecttree.ObjectTree
		ot, err = spc.PutTree(ctx, opts.createOption.treeCreate, nil)
		if err != nil {
			if err == treestorage.ErrTreeExists {
				return buildTreeObject(id)
			}
			return
		}
		return createTreeObject(ot)
	case opts.putObject != nil:
		// putting object through cache
		return opts.putObject, nil
	default:
		break
	}

	sbt, _ := coresb.SmartBlockTypeFromID(id)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		return s.initSubObject(ctx, id)
	case coresb.SmartBlockTypePage:
		return buildTreeObject(id)
	default:
		return s.objectFactory.InitObject(id, &smartblock.InitContext{
			Ctx: ctx,
		})
	}
}

// GetTree should only be called by either space services or debug apis, not the client code
func (s *Service) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	if !s.anytype.IsStarted() {
		err = errAppIsNotRunning
		return
	}

	ctx = context.WithValue(ctx, optsKey, cacheOpts{spaceId: spaceId})
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	sb := v.(smartblock.SmartBlock).Inner()
	return sb.(source.ObjectTreeProvider).Tree(), nil
}

func (s *Service) GetObject(ctx context.Context, spaceId, id string) (sb smartblock.SmartBlock, err error) {
	ctx = updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		opts.spaceId = spaceId
		return opts
	})
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), nil
}

func (s *Service) GetAccountTree(ctx context.Context, id string) (tr objecttree.ObjectTree, err error) {
	return s.GetTree(ctx, s.clientService.AccountId(), id)
}

func (s *Service) GetAccountObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return s.GetObject(ctx, s.clientService.AccountId(), id)
}

// DeleteTree should only be called by space services
func (s *Service) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	if !s.anytype.IsStarted() {
		return errAppIsNotRunning
	}

	obj, err := s.GetObject(ctx, spaceId, treeId)
	if err != nil {
		return
	}
	err = s.OnDelete(obj.Id(), nil)
	if err != nil {
		log.With(zap.Error(err)).Error("failed to execute on delete for tree")
	}
	// this should be done not inside lock
	// TODO: looks very complicated, I know
	err = obj.(smartblock.SmartBlock).Inner().(source.ObjectTreeProvider).Tree().Delete()
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(treeId)
	_, err = s.cache.Remove(treeId)
	return
}

func (s *Service) DeleteSpace(ctx context.Context, spaceID string) error {
	log.Debug("space deleted", zap.String("spaceID", spaceID))
	return nil
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

	sbt, _ := coresb.SmartBlockTypeFromID(id)
	switch sbt {
	case coresb.SmartBlockTypePage:
		var space commonspace.Space
		space, err = s.clientService.AccountSpace(context.Background())
		if err != nil {
			return
		}
		// this will call DeleteTree asynchronously in the end
		return space.DeleteTree(context.Background(), id)
	case coresb.SmartBlockTypeSubObject:
		err = s.OnDelete(id, func() error {
			return Do(s, s.anytype.PredefinedBlocks().Account, func(w editor.Workspaces) error {
				return w.DeleteSubObject(id)
			})
		})
	default:
		err = s.OnDelete(id, nil)
	}
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(id)
	_, err = s.cache.Remove(id)
	return
}

func (s *Service) CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
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

// DeriveTreeCreatePayload creates payload for the tree of derived object.
// Method should be called before DeriveObject to prepare payload
func (s *Service) DeriveTreeCreatePayload(
	ctx context.Context, tp coresb.SmartBlockType,
) (*treestorage.TreeStorageCreatePayload, error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return nil, err
	}
	payload := objecttree.ObjectTreeCreatePayload{
		SignKey:     s.commonAccount.Account().SignKey,
		ChangeType:  tp.ToProto().String(),
		SpaceId:     space.Id(),
		Identity:    s.commonAccount.Account().Identity,
		IsEncrypted: true,
	}
	create, err := space.DeriveTree(context.Background(), payload)
	return &create, err
}

// DeriveObject derives the object with id specified in the payload and triggers cache.Get
// DeriveTreeCreatePayload should be called first to prepare the payload and derive the tree
func (s *Service) DeriveObject(
	ctx context.Context, payload *treestorage.TreeStorageCreatePayload, newAccount bool,
) (err error) {
	_, err = s.getDerivedObject(ctx, payload, newAccount, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{Ctx: ctx, State: state.NewDoc(id, nil).(*state.State)}
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("derived object with error")
		return
	}
	return nil
}

func (s *Service) getDerivedObject(
	ctx context.Context, payload *treestorage.TreeStorageCreatePayload, newAccount bool, initFunc InitFunc,
) (sb smartblock.SmartBlock, err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if newAccount {
		return s.cacheCreatedObject(ctx, space.Id(), initFunc, *payload)
	}

	var (
		cancel context.CancelFunc
		id     = payload.RootRawChange.Id
	)
	// timing out when getting objects from remote
	ctx, cancel = context.WithTimeout(ctx, time.Second*40)
	ctx = context.WithValue(ctx,
		optsKey,
		cacheOpts{
			buildOption: commonspace.BuildTreeOpts{
				WaitTreeRemoteSync: true,
			},
		},
	)
	defer cancel()

	sb, err = s.GetAccountObject(ctx, id)
	if err != nil {
		err = fmt.Errorf("failed to get object from node: %w", err)
		return
	}
	return
}

func (s *Service) PutObject(ctx context.Context, id string, obj smartblock.SmartBlock) (sb smartblock.SmartBlock, err error) {
	ctx = context.WithValue(ctx, optsKey, cacheOpts{
		putObject: obj,
	})
	return s.GetAccountObject(ctx, id)
}

func (s *Service) cacheCreatedObject(ctx context.Context, spaceId string, initFunc InitFunc, create treestorage.TreeStorageCreatePayload) (sb smartblock.SmartBlock, err error) {
	ctx = context.WithValue(ctx, optsKey, cacheOpts{
		createOption: &treeCreateCache{
			treeCreate: create,
			initFunc:   initFunc,
		},
	})
	return s.GetObject(ctx, spaceId, create.RootRawChange.Id)
}

func (s *Service) initSubObject(ctx context.Context, id string) (account ocache.Object, err error) {
	if account, err = s.cache.Get(ctx, s.anytype.PredefinedBlocks().Account); err != nil {
		return
	}
	return account.(SmartblockOpener).Open(id)
}

func updateCacheOpts(ctx context.Context, update func(opts cacheOpts) cacheOpts) context.Context {
	opts, ok := ctx.Value(optsKey).(cacheOpts)
	if !ok {
		opts = cacheOpts{}
	}
	return context.WithValue(ctx, optsKey, update(opts))
}
