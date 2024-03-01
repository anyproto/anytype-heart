package syncer

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
)

type BookmarkSyncer struct {
	service *block.Service
}

func NewBookmarkSyncer(service *block.Service) *BookmarkSyncer {
	return &BookmarkSyncer{service: service}
}

func (bs *BookmarkSyncer) Sync(id domain.FullID, newIdsSet map[string]struct{}, b simple.Block, origin objectorigin.ObjectOrigin) error {
	if b.Model().GetBookmark().TargetObjectId != "" {
		return nil
	}
	if b.Model().GetBookmark().Url == "" {
		return nil
	}

	dto := block.BookmarkFetchRequest{
		RpcBlockBookmarkFetchRequest: pb.RpcBlockBookmarkFetchRequest{
			ContextId: id.ObjectID,
			BlockId:   b.Model().GetId(),
			Url:       b.Model().GetBookmark().Url,
		},
		ObjectOrigin: origin,
	}
	err := bs.service.BookmarkFetch(nil, dto)
	if err != nil {
		return fmt.Errorf("failed syncing bookmark: %w", err)
	}
	return nil
}
