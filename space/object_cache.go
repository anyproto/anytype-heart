package space

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/session"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type ctxKey int

const (
	optsKey                  ctxKey = iota
	derivedObjectLoadTimeout        = time.Minute * 30
	ObjectLoadTimeout               = time.Minute * 3
)

type InitFunc = func(id string) *smartblock.InitContext

type treeCreateCache struct {
	initFunc InitFunc
}

type cacheOpts struct {
	spaceId      string
	createOption *treeCreateCache
	buildOption  source.BuildOptions
	putObject    smartblock.SmartBlock
}

func updateCacheOpts(ctx context.Context, update func(opts cacheOpts) cacheOpts) context.Context {
	opts, ok := ctx.Value(optsKey).(cacheOpts)
	if !ok {
		opts = cacheOpts{}
	}
	return context.WithValue(ctx, optsKey, update(opts))
}

func (s *clientSpace) cacheLoad(cctx context.Context, id string) (value ocache.Object, err error) {
	opts := cctx.Value(optsKey).(cacheOpts)

	ctx := session.NewContext(cctx, opts.spaceId)
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
	case coresb.SmartBlockTypeSubObject:
		return s.initSubObject(ctx, id)
	default:
		return buildObject(id)
	}
}

func (s *clientSpace) cacheCreatedObject(ctx context.Context, id string, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	ctx = context.WithValue(ctx, optsKey, cacheOpts{
		createOption: &treeCreateCache{
			initFunc: initFunc,
		},
	})
	return s.GetObject(ctx, id)
}

type SmartblockOpener interface {
	Open(id string) (sb smartblock.SmartBlock, err error)
}

func (s *clientSpace) initSubObject(ctx session.Context, id string) (account ocache.Object, err error) {
	if account, err = s.cache.Get(ctx.Context(), s.core.PredefinedObjects(ctx.SpaceID()).Account); err != nil {
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

func (s *clientSpace) GetObjectWithTimeout(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ObjectLoadTimeout)
		defer cancel()
	}
	return s.GetObject(ctx, id)
}

func (s *clientSpace) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
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
	v, err := s.cache.Get(ctx, id)
	close(done)
	if closing && errors.Is(err, context.Canceled) {
		log.With(zap.Int64("close_delay", time.Since(start).Milliseconds()), zap.String("objectID", id)).Warn("object was loading during closing")
	}
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), nil
}

func (s *clientSpace) getDerivedObject(
	ctx context.Context,
	payload *treestorage.TreeStorageCreatePayload,
	newAccount bool,
	initFunc InitFunc,
) (sb smartblock.SmartBlock, err error) {
	if newAccount {
		var tr objecttree.ObjectTree
		tr, err = s.Space.TreeBuilder().PutTree(ctx, *payload, nil)
		s.predefinedObjectWasMissing = true
		if err != nil {
			if !errors.Is(err, treestorage.ErrTreeExists) {
				err = fmt.Errorf("failed to put tree: %w", err)
				return
			}
			s.predefinedObjectWasMissing = false
			// the object exists locally
			return s.GetObjectWithTimeout(ctx, payload.RootRawChange.Id)
		}
		tr.Close()
		return s.cacheCreatedObject(ctx, payload.RootRawChange.Id, initFunc)
	}

	var (
		cancel context.CancelFunc
		id     = payload.RootRawChange.Id
	)
	// timing out when getting objects from remote
	// here we set very long timeout, because we must load these documents
	cctx, cancel := context.WithTimeout(ctx, derivedObjectLoadTimeout)
	cctx = context.WithValue(cctx,
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

func (s *clientSpace) RemoveObjectFromCache(ctx context.Context, id string) error {
	_, err := s.cache.Remove(ctx, id)
	return err
}
