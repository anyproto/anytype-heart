package linkpreview

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/golang/groupcache/lru"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	maxCacheEntries = 10
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

type LinkPreviewResponse struct {
	lp     model.LinkPreview
	body   []byte
	isFile bool
}

func (c *cache) Fetch(ctx context.Context, url string) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error) {
	if res, ok := c.cache.Get(url); ok {
		resCasted := res.(LinkPreviewResponse)
		return resCasted.lp, resCasted.body, resCasted.isFile, nil
	}
	linkPreview, responseBody, isFile, err = c.lp.Fetch(ctx, url)
	if err != nil {
		return
	}
	c.cache.Add(url, LinkPreviewResponse{lp: linkPreview, body: responseBody, isFile: isFile})
	return
}
