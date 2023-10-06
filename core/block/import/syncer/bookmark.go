package syncer

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type BookmarkSyncer struct {
	service *block.Service
}

func NewBookmarkSyncer(service *block.Service) *BookmarkSyncer {
	return &BookmarkSyncer{service: service}
}

func (bs *BookmarkSyncer) Sync(id string, b simple.Block, origin model.ObjectOrigin) error {
	if b.Model().GetBookmark().TargetObjectId != "" {
		return nil
	}
	if b.Model().GetBookmark().Url == "" {
		return nil
	}

	dto := domain.BookmarkFetchRequest{
		RpcBlockBookmarkFetchRequest: pb.RpcBlockBookmarkFetchRequest{
			ContextId: id,
			BlockId:   b.Model().GetId(),
			Url:       b.Model().GetBookmark().Url,
		},
		Origin: origin,
	}
	err := bs.service.BookmarkFetch(nil, dto)
	if err != nil {
		return fmt.Errorf("failed syncing bookmark: %s", err)
	}
	return nil
}
