package bookmark

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("bookmark")

type CreateAndFetchRequest struct {
	pb.RpcBlockBookmarkCreateAndFetchRequest
	Origin objectorigin.ObjectOrigin
}

func NewBookmark(sb smartblock.SmartBlock, bookmarkSvc BookmarkService) Bookmark {
	return &sbookmark{
		SmartBlock:  sb,
		bookmarkSvc: bookmarkSvc,
	}
}

type Bookmark interface {
	Fetch(ctx session.Context, id string, url string, origin objectorigin.ObjectOrigin) (err error)
	CreateAndFetch(ctx session.Context, req CreateAndFetchRequest) (newID string, err error)
	UpdateBookmark(ctx session.Context, id, groupID string, apply func(b bookmark.Block) error, origin objectorigin.ObjectOrigin) (err error)
}

type BookmarkService interface {
	CreateBookmarkObject(ctx context.Context, spaceID string, details *domain.Details, getContent bookmarksvc.ContentFuture) (objectId string, newDetails *domain.Details, err error)
	FetchAsync(spaceID string, blockID string, params bookmark.FetchParams)
}

type sbookmark struct {
	smartblock.SmartBlock
	bookmarkSvc BookmarkService
}

type BlockService interface {
	DoBookmark(id string, apply func(b Bookmark) error) error
}

func (b *sbookmark) Fetch(ctx session.Context, id string, url string, origin objectorigin.ObjectOrigin) (err error) {
	s := b.NewStateCtx(ctx).SetGroupId(bson.NewObjectId().Hex())
	if err = b.fetch(ctx, s, id, url, origin); err != nil {
		return
	}
	return b.Apply(s)
}

func (b *sbookmark) fetch(ctx session.Context, s *state.State, id, url string, origin objectorigin.ObjectOrigin) (err error) {
	bb := s.Get(id)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	url, err = uri.NormalizeURI(url)
	if err != nil {
		// Do nothing
	}
	groupId := s.GroupId()
	bm, ok := bb.(bookmark.Block)
	if !ok {
		return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
	}
	bm.SetState(model.BlockContentBookmark_Fetching)

	b.bookmarkSvc.FetchAsync(b.SpaceID(), id, bookmark.FetchParams{
		Url: url,
		Updater: func(blockID string, apply func(b bookmark.Block) error) (err error) {
			return b.Space().Do(b.Id(), func(sb smartblock.SmartBlock) error {
				bm, ok := sb.(Bookmark)
				// Should never happen, because we're updating the same block
				if !ok {
					return fmt.Errorf("not a bookmark")
				}
				return bm.UpdateBookmark(ctx, blockID, groupId, apply, origin)
			})
		},
	})
	return nil
}

func (b *sbookmark) CreateAndFetch(ctx session.Context, req CreateAndFetchRequest) (newID string, err error) {
	s := b.NewStateCtx(ctx).SetGroupId(bson.NewObjectId().Hex())
	nb := simple.New(&model.Block{
		Content: &model.BlockContentOfBookmark{
			Bookmark: &model.BlockContentBookmark{
				Url: req.Url,
			},
		},
	})
	s.Add(nb)
	newID = nb.Model().Id
	if err = s.InsertTo(req.TargetId, req.Position, newID); err != nil {
		return
	}
	if err = b.fetch(ctx, s, newID, req.Url, req.Origin); err != nil {
		return
	}
	if err = b.Apply(s); err != nil {
		return
	}
	return
}

func (b *sbookmark) UpdateBookmark(ctx session.Context, id, groupID string, apply func(b bookmark.Block) error, origin objectorigin.ObjectOrigin) error {
	s := b.NewState().SetGroupId(groupID)
	if bb := s.Get(id); bb != nil {
		if bm, ok := bb.(bookmark.Block); ok {
			if err := b.updateBlock(ctx, bm, apply, origin); err != nil {
				return fmt.Errorf("update block: %w", err)
			}
		} else {
			return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
		}
	} else {
		return smartblock.ErrSimpleBlockNotFound
	}
	return b.Apply(s)
}

// updateBlock updates a block and creates associated Bookmark object
func (b *sbookmark) updateBlock(_ session.Context, block bookmark.Block, apply func(bookmark.Block) error, origin objectorigin.ObjectOrigin) error {
	if err := apply(block); err != nil {
		return err
	}

	content := block.GetContent()
	pageID, _, err := b.bookmarkSvc.CreateBookmarkObject(context.Background(), b.SpaceID(), block.ToDetails(origin), func() *bookmark.ObjectContent {
		return &bookmark.ObjectContent{BookmarkContent: content}
	})
	if err != nil {
		return fmt.Errorf("create bookmark object: %w", err)
	}

	block.UpdateContent(func(content *bookmark.ObjectContent) {
		if content.BookmarkContent == nil {
			content.BookmarkContent = &model.BlockContentBookmark{}
		}
		content.BookmarkContent.TargetObjectId = pageID
	})
	return nil
}
