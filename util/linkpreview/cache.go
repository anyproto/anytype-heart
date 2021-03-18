package linkpreview

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/hashicorp/golang-lru"
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
	c.cache, _ = lru.New(maxCacheEntries)
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
