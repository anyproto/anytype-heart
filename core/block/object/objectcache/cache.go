package objectcache

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

var log = logging.Logger("anytype-mw-object-cache")

type ctxKey int

const (
	optsKey ctxKey = iota

	ObjectLoadTimeout = time.Minute * 3
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

type Cache interface {
	app.ComponentRunnable

	PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
	GetObject(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error)
	GetObjectWithTimeout(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error)
	DoLockedIfNotExists(objectID string, proc func() error) error
	Remove(ctx context.Context, objectID string) error
	CloseBlocks()
}

type objectCache struct {
	objectFactory *editor.ObjectFactory
	sbtProvider   typeprovider.SmartBlockTypeProvider
	spaceService  space.Service
	cache         ocache.OCache
	closing       chan struct{}
}

func New() Cache {
	return &objectCache{
		closing: make(chan struct{}),
	}
}

func (c *objectCache) Init(a *app.App) error {
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	c.spaceService = app.MustComponent[space.Service](a)
	c.cache = ocache.New(
		c.cacheLoad,
		// ocache.WithLogger(log.Desugar()),
		ocache.WithGCPeriod(time.Minute),
		// TODO: [MR] Get ttl from config
		ocache.WithTTL(time.Duration(60)*time.Second),
	)
	return nil
}

func (c *objectCache) Name() string {
	return "object-cache"
}

func (c *objectCache) Run(_ context.Context) error {
	return nil
}

func (c *objectCache) Close(_ context.Context) error {
	close(c.closing)
	return c.cache.Close()
}

func ContextWithCreateOption(ctx context.Context, initFunc InitFunc) context.Context {
	return context.WithValue(ctx, optsKey, cacheOpts{
		createOption: &treeCreateCache{
			initFunc: initFunc,
		},
	})
}

func ContextWithBuildOptions(ctx context.Context, buildOpts source.BuildOptions) context.Context {
	return context.WithValue(ctx,
		optsKey,
		cacheOpts{
			buildOption: buildOpts,
		},
	)
}

func (c *objectCache) cacheLoad(ctx context.Context, id string) (value ocache.Object, err error) {
	// TODO Pass options as parameter?
	opts := ctx.Value(optsKey).(cacheOpts)

	buildObject := func(id string) (sb smartblock.SmartBlock, err error) {
		return c.objectFactory.InitObject(id, &smartblock.InitContext{Ctx: ctx, BuildOpts: opts.buildOption, SpaceID: opts.spaceId})
	}
	createObject := func() (sb smartblock.SmartBlock, err error) {
		initCtx := opts.createOption.initFunc(id)
		initCtx.IsNewObject = true
		initCtx.Ctx = ctx
		initCtx.SpaceID = opts.spaceId
		initCtx.BuildOpts = opts.buildOption
		return c.objectFactory.InitObject(id, initCtx)
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

	sbt, _ := c.sbtProvider.Type(opts.spaceId, id)
	switch sbt {
	default:
		return buildObject(id)
	}
}

func (c *objectCache) GetObject(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
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
		case <-c.closing:
			start = time.Now()
			cancel()
			closing = true
		}
	}()
	v, err := c.cache.Get(ctx, id.ObjectID)
	close(done)
	if closing && errors.Is(err, context.Canceled) {
		log.With("close_delay", time.Since(start).Milliseconds()).With("objectID", id).Warnf("object was loading during closing")
	}
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), nil
}

func (c *objectCache) Remove(ctx context.Context, objectID string) error {
	_, err := c.cache.Remove(ctx, objectID)
	return err
}

func (c *objectCache) GetObjectWithTimeout(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ObjectLoadTimeout)
		defer cancel()
	}
	return c.GetObject(ctx, id)
}

// PickBlock returns opened smartBlock or opens smartBlock in silent mode
func (c *objectCache) PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error) {
	spaceID, err := c.spaceService.ResolveSpaceID(objectID)
	if err != nil {
		// Object not loaded yet
		return nil, source.ErrObjectNotFound
	}
	return c.GetObjectWithTimeout(ctx, domain.FullID{
		SpaceID:  spaceID,
		ObjectID: objectID,
	})
}

func (c *objectCache) DoLockedIfNotExists(objectID string, proc func() error) error {
	return c.cache.DoLockedIfNotExists(objectID, proc)
}

func (c *objectCache) CloseBlocks() {
	c.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(smartblock.SmartBlock)
		ob.Lock()
		ob.ObjectCloseAllSessions()
		ob.Unlock()
		return true
	})
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
