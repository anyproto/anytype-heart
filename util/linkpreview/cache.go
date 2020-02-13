package linkpreview

import (
	"context"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/hashicorp/golang-lru"
)

const (
	maxCacheEntries = 100
)

func NewWithCache() LinkPreview {
	lruCache, _ := lru.New(maxCacheEntries)
	return &cache{
		lp:    New(),
		cache: lruCache,
	}
}

type cache struct {
	lp    LinkPreview
	cache *lru.Cache
}

func (c *cache) Fetch(ctx context.Context, url string) (lp model.ModelLinkPreview, err error) {
	if res, ok := c.cache.Get(url); ok {
		return res.(model.ModelLinkPreview), nil
	}
	lp, err = c.lp.Fetch(ctx, url)
	if err != nil {
		return
	}
	c.cache.Add(url, lp)
	return
}
