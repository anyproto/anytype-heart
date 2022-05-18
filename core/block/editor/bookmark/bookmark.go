package bookmark

import (
	"context"
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

func NewBookmark(sb smartblock.SmartBlock, lp linkpreview.LinkPreview, pageManager PageManager) Bookmark {
	return &sbookmark{SmartBlock: sb, lp: lp, manager: pageManager}
}

type Bookmark interface {
	Fetch(ctx *state.Context, id string, url string, isSync bool) (err error)
	CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error)
	UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) (err error)
}

type sbookmark struct {
	smartblock.SmartBlock
	lp      linkpreview.LinkPreview
	manager PageManager
}

type PageManager interface {
	CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error)
	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error)
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

	err = bm.Fetch(bookmark.FetchParams{
		Url:     url,
		Anytype: b.Anytype(),
		Updater: func(id string, apply func(b bookmark.Block) error) (err error) {
			if isSync {
				updMu.Lock()
				defer updMu.Unlock()
				return apply(bm)
			}
			return b.manager.DoBookmark(b.Id(), func(b Bookmark) error {
				return b.UpdateBookmark(id, groupId, apply)
			})
		},
		LinkPreview: b.lp,
		Sync:        isSync,
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

func (b *sbookmark) createBookmarkObject(bm bookmark.Block) (pageId string, err error) {
	store := b.Anytype().ObjectStore()

	content := bm.Content()

	records, _, err := store.Query(nil, database.Query{
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
			},
		},
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyWebsite.String(),
				Value:       pbtypes.String(content.Url),
			},
		},
		Limit: 1,
		ObjectTypeFilter: []string{
			bundle.TypeKeyBookmark.URL(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}

	ogDetails := map[string]*types.Value{
		bundle.RelationKeyDescription.String(): pbtypes.String(content.Description),
		bundle.RelationKeyUrl.String():         pbtypes.String(content.Url),
		bundle.RelationKeyPicture.String():     pbtypes.String(content.ImageHash),
		bundle.RelationKeyIconImage.String():   pbtypes.String(content.FaviconHash),
	}

	if len(records) > 0 {
		rec := records[0]

		details := make([]*pb.RpcBlockSetDetailsDetail, 0, len(ogDetails))
		for k, v := range ogDetails {
			details = append(details, &pb.RpcBlockSetDetailsDetail{
				Key:   k,
				Value: v,
			})
		}

		pageId = rec.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
		return pageId, b.manager.SetDetails(nil, pb.RpcBlockSetDetailsRequest{
			ContextId: pageId,
			Details:   details,
		})
	}

	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyBookmark.URL()),
			bundle.RelationKeyName.String(): pbtypes.String(content.Title),
		},
	}
	for k, v := range ogDetails {
		details.Fields[k] = v
	}

	pageId, _, err = b.manager.CreateSmartBlock(context.TODO(), coresb.SmartBlockTypePage, details, nil)
	return
}

func (b *sbookmark) UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) error {
	s := b.NewState().SetGroupId(groupId)
	if bb := s.Get(id); bb != nil {
		if bm, ok := bb.(bookmark.Block); ok {
			if err := apply(bm); err != nil {
				return err
			}

			pageId, err := b.createBookmarkObject(bm)
			if err != nil {
				return fmt.Errorf("create bookmark object: %w", err)
			}

			bm.SetTargetObjectId(pageId)

		} else {
			return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
		}
	} else {
		return smartblock.ErrSimpleBlockNotFound
	}
	return b.Apply(s)
}
