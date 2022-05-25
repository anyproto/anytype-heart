package bookmark

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
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

func NewBookmark(sb smartblock.SmartBlock, lp linkpreview.LinkPreview, blockService BlockService) Bookmark {
	return &sbookmark{SmartBlock: sb, lp: lp, blockService: blockService}
}

type Bookmark interface {
	Fetch(ctx *state.Context, id string, url string, isSync bool) (err error)
	CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error)
	UpdateBookmark(id, groupId string, apply func(b bookmark.Block) error) (err error)
}

type sbookmark struct {
	smartblock.SmartBlock
	lp           linkpreview.LinkPreview
	blockService BlockService
}

type BlockService interface {
	PageManager
	DoBookmark(id string, apply func(b Bookmark) error) error
}

type PageManager interface {
	CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error)
	SetDetails(ctx *state.Context, req pb.RpcObjectSetDetailsRequest) (err error)
	Do(id string, apply func(b smartblock.SmartBlock) error) error
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

	err = Fetch(id, FetchParams{
		Url:     url,
		Anytype: b.Anytype(),
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

func detailsFromContent(content *model.BlockContentBookmark) map[string]*types.Value {
	return map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(content.Title),
		bundle.RelationKeyDescription.String(): pbtypes.String(content.Description),
		bundle.RelationKeyUrl.String():         pbtypes.String(content.Url),
		bundle.RelationKeyPicture.String():     pbtypes.String(content.ImageHash),
		bundle.RelationKeyIconImage.String():   pbtypes.String(content.FaviconHash),
	}
}

var relationBlockKeys = []string{
	bundle.RelationKeyUrl.String(),
	bundle.RelationKeyPicture.String(),
	bundle.RelationKeyCreatedDate.String(),
	bundle.RelationKeyTag.String(),
	bundle.RelationKeyNotes.String(),
	bundle.RelationKeyQuote.String(),
}

var log = logging.Logger("anytype-mw-bookmark")

func CreateBookmarkObject(store objectstore.ObjectStore, manager PageManager, url string, getContent func() (*model.BlockContentBookmark, error)) (objectId string, err error) {
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
				RelationKey: bundle.RelationKeyUrl.String(),
				Value:       pbtypes.String(url),
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

	if len(records) > 0 {
		rec := records[0]
		objectId = rec.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	} else {
		details := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyBookmark.URL()),
				bundle.RelationKeyUrl.String():  pbtypes.String(url),
			},
		}
		objectId, _, err = manager.CreateSmartBlock(context.TODO(), coresb.SmartBlockTypePage, details, nil)
	}

	go func() {
		if err := UpdateBookmarkObject(manager, objectId, getContent); err != nil {

			log.Errorf("update bookmark object %s: %s", objectId, err)
			return
		}
	}()

	return objectId, nil
}

func UpdateBookmarkObject(manager PageManager, objectId string, getContent func() (*model.BlockContentBookmark, error)) error {
	content, err := getContent()
	if err != nil {
		return fmt.Errorf("get content: %w", err)
	}
	detailsMap := detailsFromContent(content)

	err = manager.Do(objectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()

		for _, k := range relationBlockKeys {
			if b := st.Pick(k); b != nil {
				if ok := st.Unlink(b.Model().Id); !ok {
					return fmt.Errorf("can't unlink block %s", b.Model().Id)
				}
				continue
			}

			ok := st.Add(simple.New(&model.Block{
				Id: k,
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: k,
					},
				},
			}))
			if !ok {
				return fmt.Errorf("can't add block %s", k)
			}
		}

		if err := st.InsertTo(st.RootId(), model.Block_InnerFirst, relationBlockKeys...); err != nil {
			return fmt.Errorf("insert relation blocks: %w", err)
		}

		return sb.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("update blocks: %w", err)
	}

	details := make([]*pb.RpcObjectSetDetailsDetail, 0, len(detailsMap))
	for k, v := range detailsMap {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   k,
			Value: v,
		})
	}

	return manager.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: objectId,
		Details:   details,
	})
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
	store := b.ObjectStore()
	pageId, err := CreateBookmarkObject(store, b.blockService, content.Url, func() (*model.BlockContentBookmark, error) {
		return content, nil
	})
	if err != nil {
		return fmt.Errorf("create bookmark object: %w", err)
	}

	block.UpdateContent(func(content *model.BlockContentBookmark) {
		content.TargetObjectId = pageId
	})
	return nil
}

func MigrateBlock(store objectstore.ObjectStore, manager PageManager, bm bookmark.Block) error {
	content := bm.GetContent()
	if content.TargetObjectId != "" {
		return nil
	}

	pageId, err := CreateBookmarkObject(store, manager, content.Url, func() (*model.BlockContentBookmark, error) {
		return content, nil
	})
	if err != nil {
		return fmt.Errorf("block %s: create bookmark object: %w", bm.Model().Id, err)
	}

	bm.UpdateContent(func(content *model.BlockContentBookmark) {
		content.TargetObjectId = pageId
	})
	return nil
}
