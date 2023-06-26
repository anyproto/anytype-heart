package syncer

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

type BookmarkSyncer struct {
	service *block.Service
}

func NewBookmarkSyncer(service *block.Service) *BookmarkSyncer {
	return &BookmarkSyncer{service: service}
}

func (bs *BookmarkSyncer) Sync(ctx session.Context, id string, b simple.Block) error {
	if b.Model().GetBookmark().TargetObjectId != "" {
		return nil
	}
	if b.Model().GetBookmark().Url == "" {
		return nil
	}

	err := bs.service.BookmarkFetch(ctx, pb.RpcBlockBookmarkFetchRequest{
		ContextId: id,
		BlockId:   b.Model().GetId(),
		Url:       b.Model().GetBookmark().Url,
	})
	if err != nil {
		return fmt.Errorf("failed syncing bookmark: %s", err)
	}
	return nil
}
