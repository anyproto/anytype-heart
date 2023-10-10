package linkpreview

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/golang/groupcache/lru"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	maxCacheEntries = 100
)

func NewWithCache() LinkPreview {
	return &cache{}
}

type cache struct {
	lp    LinkPreview
	cache *lru.Cache
}

func (c *cache) Init(_ *app.App) (err error) {
	c.lp = New()
	c.cache = lru.New(maxCacheEntries)
	return
}

func (c *cache) Name() string {
	return CName
}

func (c *cache) Fetch(ctx context.Context, url string) (lp model.LinkPreview, err error) {
	if res, ok := c.cache.Get(url); ok {
		return res.(model.LinkPreview), nil
	}
	lp, err = c.lp.Fetch(ctx, url)
	if err != nil {
		return
	}
	c.cache.Add(url, lp)
	return
}
