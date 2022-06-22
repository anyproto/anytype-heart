package bookmark

import (
	"fmt"
	"sync"

	bookmarksvc "github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/globalsign/mgo/bson"
)

func NewBookmark(sb smartblock.SmartBlock, blockService BlockService, bookmarkSvc BookmarkService) Bookmark {
	return &sbookmark{SmartBlock: sb, blockService: blockService, bookmarkSvc: bookmarkSvc}
}

type Bookmark interface {
	Fetch(ctx *state.Context, id string, url string, isSync bool) (err error)
	CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error)
	UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) (err error)
	MigrateBlock(bm bookmark.Block) (err error)
}

type BookmarkService interface {
	CreateBookmarkObject(url string, getContent bookmarksvc.ContentFuture) (objectId string, err error)
	Fetch(id string, params bookmark.FetchParams) (err error)
}

type sbookmark struct {
	smartblock.SmartBlock
	blockService BlockService
	bookmarkSvc  BookmarkService
}

type BlockService interface {
	DoBookmark(id string, apply func(b Bookmark) error) error
}

func (b *sbookmark) Fetch(ctx *state.Context, id string, url string, isSync bool) (err error) {
	s := b.NewStateCtx(ctx).SetGroupId(bson.NewObjectId().Hex())
	if err = b.fetch(s, id, url, isSync); err != nil {
		return
	}
	return b.Apply(s)
}

func (b *sbookmark) fetch(s *state.State, id, url string, isSync bool) (err error) {
	bb := s.Get(id)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	url, err = uri.ProcessURI(url)
	if err != nil {
		// Do nothing
	}
	groupId := s.GroupId()
	var updMu sync.Mutex
	bm, ok := bb.(bookmark.Block)
	if !ok {
		return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
	}
	bm.SetState(model.BlockContentBookmark_Fetching)

	err = b.bookmarkSvc.Fetch(id, bookmark.FetchParams{
		Url: url,
		Updater: func(id string, apply func(b bookmark.Block) error) (err error) {
			if isSync {
				updMu.Lock()
				defer updMu.Unlock()
				return b.updateBlock(bm, apply)
			}
			return b.blockService.DoBookmark(b.Id(), func(b Bookmark) error {
				return b.UpdateBookmark(id, groupId, apply)
			})
		},
		Sync: isSync,
	})
	return err
}

func (b *sbookmark) CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error) {
	s := b.NewStateCtx(ctx).SetGroupId(bson.NewObjectId().Hex())
	nb := simple.New(&model.Block{
		Content: &model.BlockContentOfBookmark{
			Bookmark: &model.BlockContentBookmark{
				Url: req.Url,
			},
		},
	})
	s.Add(nb)
	newId = nb.Model().Id
	if err = s.InsertTo(req.TargetId, req.Position, newId); err != nil {
		return
	}
	if err = b.fetch(s, newId, req.Url, false); err != nil {
		return
	}
	if err = b.Apply(s); err != nil {
		return
	}
	return
}

func (b *sbookmark) UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) error {
	s := b.NewState().SetGroupId(groupId)
	if bb := s.Get(id); bb != nil {
		if bm, ok := bb.(bookmark.Block); ok {
			if err := b.updateBlock(bm, apply); err != nil {
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
func (b *sbookmark) updateBlock(block bookmark.Block, apply func(bookmark.Block) error) error {
	if err := apply(block); err != nil {
		return err
	}

	content := block.GetContent()
	pageId, err := b.bookmarkSvc.CreateBookmarkObject(content.Url, func() *model.BlockContentBookmark {
		return content
	})
	if err != nil {
		return fmt.Errorf("create bookmark object: %w", err)
	}

	block.UpdateContent(func(content *model.BlockContentBookmark) {
		content.TargetObjectId = pageId
	})
	return nil
}

func (b *sbookmark) MigrateBlock(bm bookmark.Block) error {
	content := bm.GetContent()
	if content.State == model.BlockContentBookmark_Empty && content.Url == "" {
		content.State = model.BlockContentBookmark_Done
	}
	if content.TargetObjectId != "" {
		return nil
	}

	pageId, err := b.bookmarkSvc.CreateBookmarkObject(content.Url, func() *model.BlockContentBookmark {
		return content
	})
	if err != nil {
		return fmt.Errorf("block %s: create bookmark object: %w", bm.Model().Id, err)
	}

	bm.UpdateContent(func(content *model.BlockContentBookmark) {
		content.TargetObjectId = pageId
	})
	return nil
}
