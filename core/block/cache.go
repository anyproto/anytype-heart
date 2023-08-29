package block

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ctxKey int

var errAppIsNotRunning = errors.New("app is not running")

const (
	optsKey                  ctxKey = iota
	derivedObjectLoadTimeout        = time.Minute * 30
	objectLoadTimeout               = time.Minute * 3
	concurrentTrees                 = 10
)

type treeCreateCache struct {
	initFunc InitFunc
}

type cacheOpts struct {
	spaceId      string
	createOption *treeCreateCache
	buildOption  source.BuildOptions
	putObject    smartblock.SmartBlock
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

	buildObject := func(id string) (sb smartblock.SmartBlock, err error) {
		return s.objectFactory.InitObject(id, &smartblock.InitContext{Ctx: ctx, BuildOpts: opts.buildOption, SpaceID: opts.spaceId})
	}
	createObject := func() (sb smartblock.SmartBlock, err error) {
		initCtx := opts.createOption.initFunc(id)
		initCtx.IsNewObject = true
		initCtx.Ctx = ctx
		initCtx.SpaceID = opts.spaceId
		initCtx.BuildOpts = opts.buildOption
		return s.objectFactory.InitObject(id, initCtx)
	}

	switch {
	case opts.createOption != nil:
		return createObject()
	case opts.putObject != nil:
		// putting object through cache
		return opts.putObject, nil
	default:
		break
	}

	sbt, _ := s.sbtProvider.Type(opts.spaceId, id)
	switch sbt {
	default:
		return buildObject(id)
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

func (s *Service) NewTreeSyncer(spaceId string, treeManager treemanager.TreeManager) treemanager.TreeSyncer {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	syncer := newTreeSyncer(spaceId, objectLoadTimeout, concurrentTrees, treeManager)
	s.syncer[spaceId] = syncer
	if s.syncStarted {
		log.With("spaceID", spaceId).Warn("creating tree syncer after run")
		syncer.Run()
	}
	return syncer
}

func (s *Service) StartSync() {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	s.syncStarted = true
	for _, syncer := range s.syncer {
		syncer.Run()
	}
}

func (s *Service) GetObject(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	ctx = updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		if opts.spaceId == "" {
			opts.spaceId = id.SpaceID
		}
		return opts
	})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		done    = make(chan struct{})
		closing bool
	)
	var start time.Time
	go func() {
		select {
		case <-done:
			cancel()
		case <-s.closing:
			start = time.Now()
			cancel()
			closing = true
		}
	}()
	v, err := s.cache.Get(ctx, id.ObjectID)
	close(done)
	if closing && errors.Is(err, context.Canceled) {
		log.With("close_delay", time.Since(start).Milliseconds()).With("objectID", id).Warnf("object was loading during closing")
	}
	if err != nil {
		return
	}
	if v == nil {
		fmt.Println()
	}
	return v.(smartblock.SmartBlock), nil
}

func (s *Service) GetObjectWithTimeout(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, objectLoadTimeout)
		defer cancel()
	}
	return s.GetObject(ctx, id)
}

// DeleteTree should only be called by space services
func (s *Service) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	if !s.anytype.IsStarted() {
		return errAppIsNotRunning
	}

	obj, err := s.GetObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	})
	if err != nil {
		return
	}
	s.MarkTreeDeleted(ctx, spaceId, treeId)
	// this should be done not inside lock
	// TODO: looks very complicated, I know
	err = obj.(smartblock.SmartBlock).Inner().(source.ObjectTreeProvider).Tree().Delete()
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(spaceId, treeId)
	_, err = s.cache.Remove(ctx, treeId)
	return
}

func (s *Service) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := s.OnDelete(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	}, nil)
	if err != nil {
		log.Error("failed to execute on delete for tree", zap.Error(err))
	}
	return err
}

func (s *Service) DeleteSpace(ctx context.Context, spaceID string) error {
	log.Debug("space deleted", zap.String("spaceID", spaceID))
	return nil
}

func (s *Service) DeleteObject(objectID string) (err error) {
	spaceID, err := s.spaceService.ResolveSpaceID(objectID)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	err = Do(s, objectID, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return
	}

	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: objectID,
	}
	sbt, _ := s.sbtProvider.Type(spaceID, objectID)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		return fmt.Errorf("subobjects deprecated")
	case coresb.SmartBlockTypeFile:
		err = s.OnDelete(id, func() error {
			if err := s.fileStore.DeleteFile(objectID); err != nil {
				return err
			}
			if err := s.fileSync.RemoveFile(spaceID, objectID); err != nil {
				return fmt.Errorf("failed to remove file from sync: %w", err)
			}
			_, err = s.fileService.FileOffload(context.Background(), objectID, true)
			if err != nil {
				return err
			}
			return nil
		})
	default:
		var space commonspace.Space
		space, err = s.spaceService.GetSpace(context.Background(), spaceID)
		if err != nil {
			return
		}
		// this will call DeleteTree asynchronously in the end
		return space.DeleteTree(context.Background(), objectID)
	}
	if err != nil {
		return
	}

	s.sendOnRemoveEvent(objectID)
	_, err = s.cache.Remove(context.Background(), objectID)
	return
}

func (s *Service) CreateTreePayload(ctx context.Context, spaceID string, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, createdTime)
}

func (s *Service) CreateTreePayloadWithSpace(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType) (treestorage.TreeStorageCreatePayload, error) {
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, time.Now())
}

func (s *Service) CreateTreePayloadWithSpaceAndCreatedTime(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(tp, nil)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(space.Id(), s.commonAccount.Account().SignKey, changePayload, createdTime.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return space.TreeBuilder().CreateTree(ctx, treePayload)
}

func (s *Service) CreateTreeObjectWithPayload(ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	return s.cacheCreatedObject(ctx, id, initFunc)
}

func (s *Service) CreateTreeObject(ctx context.Context, spaceID string, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	payload, err := s.CreateTreePayloadWithSpace(ctx, space, tp)
	if err != nil {
		return nil, err
	}

	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	return s.cacheCreatedObject(ctx, id, initFunc)
}

func (s *Service) CreateTreeObjectWithUniqueKey(ctx context.Context, spaceID string, key domain.UniqueKey, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	payload, err := s.DeriveTreeCreatePayload(ctx, spaceID, key)
	if err != nil {
		return nil, err
	}

	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	return s.cacheCreatedObject(ctx, id, initFunc)
}

// DeriveTreeCreatePayload creates payload for the tree of derived object.
// Method should be called before DeriveObject to prepare payload
func (s *Service) DeriveTreeCreatePayload(
	ctx context.Context,
	spaceID string,
	key domain.UniqueKey,
) (treestorage.TreeStorageCreatePayload, error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	changePayload, err := createChangePayload(coresb.SmartBlockType(key.SmartblockType()), key)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload := derivePayload(space.Id(), s.commonAccount.Account().SignKey, changePayload)
	create, err := space.TreeBuilder().CreateTree(context.Background(), treePayload)
	return create, err
}

// DeriveObject derives the object with id specified in the payload and triggers cache.Get
// DeriveTreeCreatePayload should be called first to prepare the payload and derive the tree
func (s *Service) DeriveObject(
	ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, newAccount bool,
) (err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	_, err = s.getDerivedObject(ctx, space, &payload, newAccount, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{Ctx: ctx, SpaceID: spaceID, State: state.NewDoc(id, nil).(*state.State)}
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("derived object with error")
		return
	}
	return nil
}

func (s *Service) getDerivedObject(
	ctx context.Context, space commonspace.Space, payload *treestorage.TreeStorageCreatePayload, newAccount bool, initFunc InitFunc,
) (sb smartblock.SmartBlock, err error) {
	id := domain.FullID{
		SpaceID:  space.Id(),
		ObjectID: payload.RootRawChange.Id,
	}
	if newAccount {
		var tr objecttree.ObjectTree
		tr, err = space.TreeBuilder().PutTree(ctx, *payload, nil)
		s.predefinedObjectWasMissing = true
		if err != nil {
			if !errors.Is(err, treestorage.ErrTreeExists) {
				err = fmt.Errorf("failed to put tree: %w", err)
				return
			}
			s.predefinedObjectWasMissing = false
			// the object exists locally
			return s.GetObjectWithTimeout(ctx, id)
		}
		tr.Close()
		return s.cacheCreatedObject(ctx, id, initFunc)
	}

	// timing out when getting objects from remote
	// here we set very long timeout, because we must load these documents
	ctx, cancel := context.WithTimeout(ctx, derivedObjectLoadTimeout)
	ctx = context.WithValue(ctx,
		optsKey,
		cacheOpts{
			buildOption: source.BuildOptions{
				// TODO: revive p2p (right now we are not ready to load from local clients due to the fact that we need to know when local peers connect)
			},
		},
	)
	defer cancel()

	sb, err = s.GetObjectWithTimeout(ctx, id)
	if err != nil {
		if errors.Is(err, treechangeproto.ErrGetTree) {
			err = spacesyncproto.ErrSpaceMissing
		}
		err = fmt.Errorf("failed to get object from node: %w", err)
		return
	}
	return
}

func (s *Service) cacheCreatedObject(ctx context.Context, id domain.FullID, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	err = s.spaceService.StoreSpaceID(id.ObjectID, id.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to store space id: %w", err)
	}

	ctx = context.WithValue(ctx, optsKey, cacheOpts{
		createOption: &treeCreateCache{
			initFunc: initFunc,
		},
	})
	return s.GetObject(ctx, id)
}

func CacheOptsWithRemoteLoadDisabled(ctx context.Context) context.Context {
	return updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		opts.buildOption.DisableRemoteLoad = true
		return opts
	})
}

func CacheOptsSetSpaceID(ctx context.Context, spaceID string) context.Context {
	return updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		opts.spaceId = spaceID
		return opts
	})
}

func updateCacheOpts(ctx context.Context, update func(opts cacheOpts) cacheOpts) context.Context {
	opts, ok := ctx.Value(optsKey).(cacheOpts)
	if !ok {
		opts = cacheOpts{}
	}
	return context.WithValue(ctx, optsKey, update(opts))
}
