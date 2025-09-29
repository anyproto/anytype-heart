package linkpreview

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/hashicorp/golang-lru/v2/simplelru"

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
	cache simplelru.LRUCache[string, model.LinkPreview]
}

func (c *cache) Init(_ *app.App) (err error) {
	c.lp = New()
	c.cache, err = simplelru.NewLRU[string, model.LinkPreview](maxCacheEntries, nil)
	return
}

func (c *cache) Name() string {
	return CName
}

func (c *cache) Fetch(
	ctx context.Context, url string, withResponseBody bool,
) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error) {
	// we do not cache responseBody, that's why withResponseBody flag is needed
	if linkPreview, ok := c.cache.Get(url); ok && !withResponseBody {
		return linkPreview, nil, false, nil
	}
	linkPreview, responseBody, isFile, err = c.lp.Fetch(ctx, url, withResponseBody)
	if err != nil {
		return
	}
	c.cache.Add(url, linkPreview)
	return
}
