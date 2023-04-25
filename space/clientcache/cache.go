package clientcache

import (
	"context"
	"errors"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/accountservice"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app/logger"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app/ocache"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/treestorage"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/treegetter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/space"
	"time"
)

var log = logger.NewNamed("treecache")
var ErrCacheObjectWithoutTree = errors.New("cache object contains no tree")

type ctxKey int

const (
	spaceKey ctxKey = iota
	treeCreateKey
)

type cache struct {
	gcttl         int
	cache         ocache.OCache
	account       accountservice.Service
	clientService space.Service
	objectFactory *editor.ObjectFactory
	objectDeleter ObjectDeleter
}

type Cache interface {
	treegetter.TreeGetter
	// TODO: remove this
	treegetter.TreePutter
	GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, release func(), err error)
	CreateTreeObject(tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, release func(), err error)
	PutObject(id string, obj smartblock.SmartBlock) (sb smartblock.SmartBlock, release func(), err error)
	DeleteObject(id string) (err error)
	ObjectCache() ocache.OCache
}

type ObjectDeleter interface {
	OnDelete(b smartblock.SmartBlock) (err error)
}

type InitFunc func(id string) *smartblock.InitContext

func New(ttl int) Cache {
	return &cache{
		gcttl: ttl,
	}
}

func (c *cache) Run(ctx context.Context) (err error) {
	return nil
}

func (c *cache) Close(ctx context.Context) (err error) {
	return c.cache.Close()
}

func (c *cache) Init(a *app.App) (err error) {
	c.clientService = a.MustComponent(space.CName).(space.Service)
	c.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.cache = ocache.New(
		func(ctx context.Context, id string) (value ocache.Object, err error) {
			spaceId := ctx.Value(spaceKey).(string)
			spc, err := c.clientService.GetSpace(ctx, spaceId)
			if err != nil {
				return
			}
			// creating tree if needed
			createPayload, exists := ctx.Value(treeCreateKey).(treestorage.TreeStorageCreatePayload)
			if exists {
				ot, err := spc.PutTree(ctx, createPayload, nil)
				if err != nil {
					return
				}
				ot.Close()
			}
			return c.objectFactory.InitObject(id, &smartblock.InitContext{
				Ctx: ctx,
			})
		},
		ocache.WithLogger(log.Sugar()),
		ocache.WithRefCounter(true),
		ocache.WithGCPeriod(time.Minute),
		ocache.WithTTL(time.Duration(c.gcttl)*time.Second),
	)
	c.objectDeleter = app.MustComponent[ObjectDeleter](a)
	return nil
}

func (c *cache) Name() (name string) {
	return treegetter.CName
}

func (c *cache) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, release func(), err error) {
	ctx = context.WithValue(ctx, spaceKey, c.account)
	v, err := c.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(smartblock.SmartBlock), func() {
		c.cache.Release(id)
	}, nil
}

func (c *cache) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	v, err := c.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(objecttree.ObjectTree), nil
}

func (c *cache) PutTree(ctx context.Context, spaceId string, payload treestorage.TreeStorageCreatePayload) (ot objecttree.ObjectTree, err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	ctx = context.WithValue(ctx, treeCreateKey, payload)
	v, err := c.cache.Get(ctx, payload.RootRawChange.Id)
	if err != nil {
		return
	}
	return v.(objecttree.ObjectTree), nil
}

func (c *cache) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	tr, _, err := c.GetObject(ctx, treeId)
	if err != nil {
		return
	}
	err = c.objectDeleter.OnDelete(tr)
	if err != nil {
		return
	}

	err = tr.(objecttree.ObjectTree).Delete()
	if err != nil {
		return
	}
	_, err = c.cache.Remove(treeId)
	return
}

func (c *cache) CreateTreeObject(tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, release func(), err error) {
	// create tree payload
	// put tree payload in context
	// call get method with tree payload
	// put
	panic("not implemented")
}

func (c *cache) PutObject(id string, obj smartblock.SmartBlock) (sb smartblock.SmartBlock, release func(), err error) {
	panic("not implemented")
}

func (c *cache) DeleteObject(id string) (err error) {
	panic("not implemented")
}

func (c *cache) ObjectCache() ocache.OCache {
	return c.cache
}
