package block

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	spaceservice "github.com/anyproto/anytype-heart/space"
)

type ctxKey int

var errAppIsNotRunning = errors.New("app is not running")

const (
	optsKey                  ctxKey = iota
	derivedObjectLoadTimeout        = time.Minute * 30
	objectLoadTimeout               = time.Minute * 3
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

	sbt, _ := s.sbtProvider.Type(id)
	switch sbt {
	case coresb.SmartBlockTypeSubObject:
		return s.initSubObject(ctx, id)
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

func (s *Service) NewTreeSyncer(spaceId string) treemanager.TreeSyncer {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	if s.syncer != nil {
		s.syncer.Close()
		s.syncer = newTreeSyncer(spaceId, time.Second, 10, s)
	}
	if s.syncStarted {
		s.syncer.Run()
	}
	return s.syncer
}

func (s *Service) StartSync() {
	s.syncerLock.Lock()
	defer s.syncerLock.Unlock()
	s.syncStarted = true
	if s.syncer != nil {
		s.syncer.Run()
	}
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
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, objectLoadTimeout)
		defer cancel()
	}
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
	s.MarkTreeDeleted(ctx, spaceId, treeId)
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

func (s *Service) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := s.OnDelete(treeId, nil)
	if err != nil {
		log.Error("failed to execute on delete for tree", zap.Error(err))
	}
	return err
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
			if err := s.fileStore.DeleteFile(id); err != nil {
				return err
			}
			if err := s.fileSync.RemoveFile(s.clientService.AccountId(), id); err != nil {
				return fmt.Errorf("failed to remove file from sync: %w", err)
			}
			_, err = s.fileService.FileOffload(id, true)
			if err != nil {
				return err
			}
			return nil
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

func (s *Service) CreateTreePayload(ctx context.Context, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, createdTime)
}

func (s *Service) CreateTreePayloadWithSpace(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType) (treestorage.TreeStorageCreatePayload, error) {
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, time.Now())
}

func (s *Service) CreateTreePayloadWithSpaceAndCreatedTime(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(tp)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(space.Id(), s.commonAccount.Account().SignKey, changePayload, createdTime.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return space.CreateTree(ctx, treePayload)
}

func (s *Service) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return nil, err
	}
	tr, err := space.PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	tr.Close()
	return s.cacheCreatedObject(ctx, payload.RootRawChange.Id, space.Id(), initFunc)
}

func (s *Service) CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := s.CreateTreePayloadWithSpace(ctx, space, tp)
	if err != nil {
		return nil, err
	}

	tr, err := space.PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	tr.Close()
	return s.cacheCreatedObject(ctx, payload.RootRawChange.Id, space.Id(), initFunc)
}

func (s *Service) ResetTreeObject(ctx context.Context, id string, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.clientService.AccountSpace(ctx)
	if err != nil {
		return
	}

	return s.cacheCreatedObject(ctx, id, space.Id(), initFunc)
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
	treePayload := derivePayload(space.Id(), s.commonAccount.Account().SignKey, changePayload)
	create, err := space.CreateTree(context.Background(), treePayload)
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
	// here we set very long timeout, because we must load these documents
	ctx, cancel = context.WithTimeout(ctx, derivedObjectLoadTimeout)
	ctx = context.WithValue(ctx,
		optsKey,
		cacheOpts{
			buildOption: source.BuildOptions{
				RetryRemoteLoad: true,
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

func CacheOptsWithRemoteLoadDisabled(ctx context.Context) context.Context {
	return updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		opts.buildOption.DisableRemoteLoad = true
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

func createChangePayload(sbType coresb.SmartBlockType) (data []byte, err error) {
	payload := &model.ObjectChangePayload{SmartBlockType: model.SmartBlockType(sbType)}
	return payload.Marshal()
}

func derivePayload(spaceId string, signKey crypto.PrivKey, changePayload []byte) objecttree.ObjectTreeCreatePayload {
	return objecttree.ObjectTreeCreatePayload{
		PrivKey:       signKey,
		ChangeType:    spaceservice.ChangeType,
		ChangePayload: changePayload,
		SpaceId:       spaceId,
		IsEncrypted:   true,
	}
}

func createPayload(spaceId string, signKey crypto.PrivKey, changePayload []byte, timestamp int64) (objecttree.ObjectTreeCreatePayload, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return objecttree.ObjectTreeCreatePayload{}, err
	}
	return objecttree.ObjectTreeCreatePayload{
		PrivKey:       signKey,
		ChangeType:    spaceservice.ChangeType,
		ChangePayload: changePayload,
		SpaceId:       spaceId,
		IsEncrypted:   true,
		Timestamp:     timestamp,
		Seed:          seed,
	}, nil
}
