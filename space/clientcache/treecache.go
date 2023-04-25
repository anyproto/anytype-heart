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
	"github.com/anytypeio/go-anytype-middleware/space"
	"go.uber.org/zap"
	"time"
)

var log = logger.NewNamed("treecache")
var ErrCacheObjectWithoutTree = errors.New("cache object contains no tree")

type ctxKey int

const (
	spaceKey ctxKey = iota
	treeCreateKey
)

type treeCache struct {
	gcttl         int
	cache         ocache.OCache
	account       accountservice.Service
	clientService space.Service
	objectFactory *editor.ObjectFactory
}

type TreeCache interface {
	treegetter.TreeGetter
	treegetter.TreePutter
}

type updateListener struct {
}

func (u *updateListener) Update(tree objecttree.ObjectTree) {
	log.With(
		zap.Strings("heads", tree.Heads()),
		zap.String("tree id", tree.Id())).
		Debug("updating tree")
}

func (u *updateListener) Rebuild(tree objecttree.ObjectTree) {
	log.With(
		zap.Strings("heads", tree.Heads()),
		zap.String("tree id", tree.Id())).
		Debug("rebuilding tree")
}

func New(ttl int) TreeCache {
	return &treeCache{
		gcttl: ttl,
	}
}

func (c *treeCache) Run(ctx context.Context) (err error) {
	return nil
}

func (c *treeCache) Close(ctx context.Context) (err error) {
	return c.cache.Close()
}

func (c *treeCache) Init(a *app.App) (err error) {
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
				ot, err := spc.PutTree(ctx, createPayload, &updateListener{})
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
		ocache.WithGCPeriod(time.Minute),
		ocache.WithTTL(time.Duration(c.gcttl)*time.Second),
	)
	return nil
}

func (c *treeCache) Name() (name string) {
	return treegetter.CName
}

func (c *treeCache) GetObject()

func (c *treeCache) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	v, err := c.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return v.(objecttree.ObjectTree), nil
}

func (c *treeCache) PutTree(ctx context.Context, spaceId string, payload treestorage.TreeStorageCreatePayload) (ot objecttree.ObjectTree, err error) {
	ctx = context.WithValue(ctx, spaceKey, spaceId)
	ctx = context.WithValue(ctx, treeCreateKey, payload)
	v, err := c.cache.Get(ctx, payload.RootRawChange.Id)
	if err != nil {
		return
	}
	return v.(objecttree.ObjectTree), nil
}

func (c *treeCache) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	tr, err := c.GetTree(ctx, spaceId, treeId)
	if err != nil {
		return
	}
	err = tr.Delete()
	if err != nil {
		return
	}
	_, err = c.cache.Remove(treeId)
	return
}
