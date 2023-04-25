package block

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	spaceservice "github.com/anytypeio/go-anytype-middleware/space"
)

type ctxKey int

var errAppIsNotRunning = errors.New("app is not running")

const (
	optsKey ctxKey = iota
)

type treeCreateCache struct {
	initFunc InitFunc
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
	// TODO Pass options as parameter?
	opts := ctx.Value(optsKey).(cacheOpts)

	buildTreeObject := func(id string) (sb smartblock.SmartBlock, err error) {
		return s.objectFactory.InitObject(id, &smartblock.InitContext{Ctx: ctx, BuildTreeOpts: opts.buildOption, SpaceID: opts.spaceId})
	}
	createTreeObject := func() (sb smartblock.SmartBlock, err error) {
		initCtx := opts.createOption.initFunc(id)
		initCtx.IsNewObject = true
		initCtx.SpaceID = opts.spaceId
		initCtx.BuildTreeOpts = opts.buildOption
		return s.objectFactory.InitObject(id, initCtx)
	}

	switch {
	case opts.createOption != nil:
		return createTreeObject()
	case opts.putObject != nil:
		// putting object through cache
		return opts.putObject, nil
	default:
		break
	}

	sbt, _ := s.sbtProvider.Type(id)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		return s.initSubObject(ctx, id)
	default:
		return buildTreeObject(id)
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
	_, err = s.cache.Remove(ctx, treeId)
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

	sbt, _ := s.sbtProvider.Type(id)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		err = s.OnDelete(id, func() error {
			return Do(s, s.anytype.PredefinedBlocks().Account, func(w *editor.Workspaces) error {
				return w.DeleteSubObject(id)
			})
		})
	case coresb.SmartBlockTypeFile:
		err = s.OnDelete(id, func() error {
			if err = s.fileStore.DeleteByTarget(id); err != nil {
				return err
			}
			if err = s.fileStore.DeleteFileKeys(id); err != nil {
				return err
			}
			_, err = s.fileService.FileOffload(id, true)
			if err != nil {
				return err
			}
			return s.fileSync.RemoveFile(s.clientService.AccountId(), id)
		})
	default:
		var space commonspace.Space
		space, err = s.clientService.AccountSpace(context.Background())
		if err != nil {
			return
		}
		// this will call DeleteTree asynchronously in the end
		return space.DeleteTree(context.Background(), id)
	}
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(id)
	_, err = s.cache.Remove(context.Background(), id)
	return
}

func (s *Service) CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return
	}
	changePayload, err := createChangePayload(tp)
	if err != nil {
		return
	}
	payload := objecttree.ObjectTreeCreatePayload{
		PrivKey:       s.commonAccount.Account().SignKey,
		ChangeType:    spaceservice.ChangeType,
		ChangePayload: changePayload,
		SpaceId:       space.Id(),
		IsEncrypted:   true,
	}
	create, err := space.CreateTree(context.Background(), payload)
	if err != nil {
		return
	}
	tr, err := space.PutTree(ctx, create, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	tr.Close()
	return s.cacheCreatedObject(ctx, create.RootRawChange.Id, space.Id(), initFunc)
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
	changePayload, err := createChangePayload(tp)
	if err != nil {
		return nil, err
	}
	payload := objecttree.ObjectTreeCreatePayload{
		PrivKey:       s.commonAccount.Account().SignKey,
		ChangeType:    spaceservice.ChangeType,
		ChangePayload: changePayload,
		SpaceId:       space.Id(),
		IsEncrypted:   true,
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
		var tr objecttree.ObjectTree
		tr, err = space.PutTree(ctx, *payload, nil)
		if err != nil {
			if !errors.Is(err, treestorage.ErrTreeExists) {
				err = fmt.Errorf("failed to put tree: %w", err)
				return
			}
			// the object exists locally
			return s.GetAccountObject(ctx, payload.RootRawChange.Id)
		}
		tr.Close()
		return s.cacheCreatedObject(ctx, payload.RootRawChange.Id, space.Id(), initFunc)
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
		if errors.Is(err, treechangeproto.ErrGetTree) {
			err = spacesyncproto.ErrSpaceMissing
		}
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

func (s *Service) cacheCreatedObject(ctx context.Context, id, spaceId string, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	ctx = context.WithValue(ctx, optsKey, cacheOpts{
		createOption: &treeCreateCache{
			initFunc: initFunc,
		},
	})
	return s.GetObject(ctx, spaceId, id)
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

func createChangePayload(sbType coresb.SmartBlockType) (data []byte, err error) {
	payload := &model.ObjectChangePayload{SmartBlockType: model.SmartBlockType(sbType)}
	return payload.Marshal()
}
