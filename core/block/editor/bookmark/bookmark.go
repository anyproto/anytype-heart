package bookmark

import (
	"fmt"
	"sync"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("bookmark")

func NewBookmark(
	sb smartblock.SmartBlock,
	blockService BlockService,
	bookmarkSvc BookmarkService,
	objectStore objectstore.ObjectStore,
) Bookmark {
	return &sbookmark{
		SmartBlock:   sb,
		blockService: blockService,
		bookmarkSvc:  bookmarkSvc,
		objectStore:  objectStore,
	}
}

type Bookmark interface {
	Fetch(ctx session.Context, id string, url string, isSync bool) (err error)
	CreateAndFetch(ctx session.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error)
	UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) (err error)
	MigrateBlock(bm bookmark.Block) (err error)
}

type BookmarkService interface {
	CreateBookmarkObject(details *types.Struct, getContent bookmarksvc.ContentFuture) (objectId string, newDetails *types.Struct, err error)
	Fetch(id string, params bookmark.FetchParams) (err error)
}

type sbookmark struct {
	smartblock.SmartBlock
	blockService BlockService
	bookmarkSvc  BookmarkService
	objectStore  objectstore.ObjectStore
}

type BlockService interface {
	DoBookmark(id string, apply func(b Bookmark) error) error
}

func (b *sbookmark) Fetch(ctx session.Context, id string, url string, isSync bool) (err error) {
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
	url, err = uri.NormalizeURI(url)
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

func (b *sbookmark) CreateAndFetch(ctx session.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error) {
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
	pageId, _, err := b.bookmarkSvc.CreateBookmarkObject(block.ToDetails(), func() *model.BlockContentBookmark {
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

	// Fix broken empty bookmarks
	// we had a bug that migrated empty bookmarks blocks into bookmark objects. Now we need to reset them
	// todo: remove this after we stop to populate bookmark block fields
	if content.Url == "" && content.State == model.BlockContentBookmark_Done && content.Title == "" && content.FaviconHash == "" && content.TargetObjectId != "" {
		// nolint:errcheck
		det, _ := b.objectStore.GetDetails(content.TargetObjectId)
		if det != nil && pbtypes.GetString(det.Details, bundle.RelationKeyUrl.String()) == "" && pbtypes.GetString(det.Details, bundle.RelationKeySource.String()) == "" {
			bm.UpdateContent(func(content *model.BlockContentBookmark) {
				content.State = model.BlockContentBookmark_Empty
				content.TargetObjectId = ""
			})
		}
		return nil
	}

	if content.State == model.BlockContentBookmark_Error {
		bm.UpdateContent(func(content *model.BlockContentBookmark) {
			content.State = model.BlockContentBookmark_Empty
			content.TargetObjectId = ""
		})
		return nil
	}

	if content.TargetObjectId != "" {
		if content.State != model.BlockContentBookmark_Done {
			bm.UpdateContent(func(content *model.BlockContentBookmark) {
				content.State = model.BlockContentBookmark_Done
			})
		}
		return nil
	}

	if content.Url == "" {
		return nil
	}

	pageId, _, err := b.bookmarkSvc.CreateBookmarkObject(bm.ToDetails(), func() *model.BlockContentBookmark {
		return content
	})
	if err != nil {
		return fmt.Errorf("block %s: create bookmark object: %w", bm.Model().Id, err)
	}

	bm.UpdateContent(func(content *model.BlockContentBookmark) {
		content.TargetObjectId = pageId
		content.State = model.BlockContentBookmark_Done
	})
	return nil
}
