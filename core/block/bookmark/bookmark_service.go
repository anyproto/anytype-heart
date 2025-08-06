package bookmark

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/uri"
)

const CName = "bookmark"

// ContentFuture represents asynchronous result of getting bookmark content
type ContentFuture func() *bookmark.ObjectContent

type Service interface {
	CreateObjectAndFetch(
		ctx context.Context, spaceId, templateId string, details *domain.Details,
	) (objectID string, newDetails *domain.Details, err error)
	CreateBookmarkObject(
		ctx context.Context, spaceId, templateId string, details *domain.Details, getContent ContentFuture,
	) (objectId string, newDetails *domain.Details, err error)

	UpdateObject(objectId string, getContent *bookmark.ObjectContent) error
	// TODO Maybe Fetch and FetchBookmarkContent do the same thing differently?
	FetchAsync(spaceID string, blockID string, params bookmark.FetchParams)
	FetchBookmarkContent(spaceID string, url string, parseBlock bool) ContentFuture
	ContentUpdaters(spaceID string, url string, parseBlock bool) (chan func(contentBookmark *bookmark.ObjectContent), error)

	app.Component
}

type (
	ObjectCreator interface {
		CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *domain.Details, err error)
	}

	DetailsSetter interface {
		SetDetails(ctx session.Context, objectId string, details []domain.Detail) (err error)
	}
)

type service struct {
	detailsSetter       DetailsSetter
	creator             ObjectCreator
	store               objectstore.ObjectStore
	linkPreview         linkpreview.LinkPreview
	tempDirService      core.TempDirProvider
	spaceService        space.Service
	fileUploaderFactory fileuploader.Service
	templateService     template.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.detailsSetter = app.MustComponent[DetailsSetter](a)
	s.creator = app.MustComponent[ObjectCreator](a)
	s.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.spaceService = app.MustComponent[space.Service](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	s.fileUploaderFactory = app.MustComponent[fileuploader.Service](a)
	s.templateService = app.MustComponent[template.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

var log = logging.Logger("anytype-mw-bookmark")

func (s *service) CreateObjectAndFetch(
	ctx context.Context, spaceId, templateId string, details *domain.Details,
) (objectID string, newDetails *domain.Details, err error) {
	source := details.GetString(bundle.RelationKeySource)
	var res ContentFuture
	if source != "" {
		u, err := uri.NormalizeURI(source)
		if err != nil {
			return "", nil, fmt.Errorf("process uri: %w", err)
		}
		res = s.FetchBookmarkContent(spaceId, u, false)
	} else {
		res = func() *bookmark.ObjectContent {
			return nil
		}
	}
	return s.CreateBookmarkObject(ctx, spaceId, templateId, details, res)
}

func (s *service) CreateBookmarkObject(
	ctx context.Context, spaceId, templateId string, details *domain.Details, getContent ContentFuture,
) (objectId string, objectDetails *domain.Details, err error) {
	if details == nil {
		return "", nil, fmt.Errorf("empty details")
	}

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	typeId, err := spc.GetTypeIdByKey(ctx, bundle.TypeKeyBookmark)
	if err != nil {
		return "", nil, fmt.Errorf("get bookmark type id: %w", err)
	}
	url := details.GetString(bundle.RelationKeySource)

	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Sorts: []database.SortRequest{
			{
				RelationKey: bundle.RelationKeyLastModifiedDate,
				Type:        model.BlockContentDataviewSort_Desc,
			},
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySource,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(url),
			},
			{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(typeId),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return "", nil, fmt.Errorf("query: %w", err)
	}

	if len(records) > 0 {
		rec := records[0]
		objectId = rec.Details.GetString(bundle.RelationKeyId)
		objectDetails = rec.Details
	} else {
		creationState, err := s.templateService.CreateTemplateStateWithDetails(template.CreateTemplateRequest{
			SpaceId:                spaceId,
			TemplateId:             templateId,
			TypeId:                 typeId,
			Layout:                 model.ObjectType_bookmark,
			Details:                details,
			WithTemplateValidation: true,
		})
		if err != nil {
			log.Errorf("failed to build state for bookmark: %v", err)
		}
		objectId, objectDetails, err = s.creator.CreateSmartBlockFromState(
			ctx,
			spaceId,
			[]domain.TypeKey{bundle.TypeKeyBookmark},
			creationState,
		)
		if err != nil {
			return "", nil, fmt.Errorf("create bookmark object: %w", err)
		}
	}

	if url != "" {
		go func() {
			if err := s.UpdateObject(objectId, getContent()); err != nil {

				log.Errorf("update bookmark object %s: %s", objectId, err)
				return
			}
		}()
	}

	return objectId, objectDetails, nil
}

func (s *service) UpdateObject(objectId string, content *bookmark.ObjectContent) error {
	details := []domain.Detail{
		{Key: bundle.RelationKeyName, Value: domain.String(content.BookmarkContent.Title)},
		{Key: bundle.RelationKeyDescription, Value: domain.String(content.BookmarkContent.Description)},
		{Key: bundle.RelationKeySource, Value: domain.String(content.BookmarkContent.Url)},
		{Key: bundle.RelationKeyPicture, Value: domain.String(content.BookmarkContent.ImageHash)},
		{Key: bundle.RelationKeyIconImage, Value: domain.String(content.BookmarkContent.FaviconHash)},
	}

	return s.detailsSetter.SetDetails(nil, objectId, details)
}

func (s *service) FetchAsync(spaceID string, blockID string, params bookmark.FetchParams) {
	go func() {
		if err := s.fetcher(spaceID, blockID, params); err != nil {
			log.Errorf("fetch bookmark %s: %s", blockID, err)
		}
	}()
}

func (s *service) FetchBookmarkContent(spaceID string, url string, parseBlock bool) ContentFuture {
	contentCh := make(chan *bookmark.ObjectContent, 1)
	go func() {
		defer close(contentCh)

		content := &bookmark.ObjectContent{
			BookmarkContent: &model.BlockContentBookmark{Url: url},
		}
		updaters, err := s.ContentUpdaters(spaceID, url, parseBlock)
		if err != nil {
			log.Errorf("fetch bookmark content: %s", err)
		}
		for upd := range updaters {
			upd(content)
		}
		contentCh <- content
	}()

	return func() *bookmark.ObjectContent {
		return <-contentCh
	}
}

func (s *service) ContentUpdaters(spaceID string, url string, parseBlock bool) (chan func(contentBookmark *bookmark.ObjectContent), error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	updaters := make(chan func(contentBookmark *bookmark.ObjectContent), 1)

	data, body, isFile, err := s.linkPreview.Fetch(ctx, url)
	if err != nil {
		updaters <- func(c *bookmark.ObjectContent) {
			if c.BookmarkContent == nil {
				c.BookmarkContent = &model.BlockContentBookmark{}
			}
			c.BookmarkContent.State = model.BlockContentBookmark_Done
			c.BookmarkContent.Url = url
		}
		close(updaters)
		return updaters, fmt.Errorf("bookmark: can't fetch link: %w", err)
	}

	updaters <- func(c *bookmark.ObjectContent) {
		c.BookmarkContent.State = model.BlockContentBookmark_Done
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		updaters <- func(c *bookmark.ObjectContent) {
			if c.BookmarkContent == nil {
				c.BookmarkContent = &model.BlockContentBookmark{}
			}
			c.BookmarkContent.Url = data.Url
			c.BookmarkContent.Title = data.Title
			c.BookmarkContent.Description = data.Description
			c.BookmarkContent.Type = data.Type
		}
	}()

	if data.ImageUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := s.loadImage(spaceID, getFileNameFromURL(url, data.ImageUrl, "cover"), data.ImageUrl)
			if err != nil {
				log.Errorf("load image: %s", err)
				return
			}
			updaters <- func(c *bookmark.ObjectContent) {
				if c.BookmarkContent == nil {
					c.BookmarkContent = &model.BlockContentBookmark{}
				}
				c.BookmarkContent.ImageHash = hash
			}
		}()
	}
	if data.FaviconUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := s.loadImage(spaceID, getFileNameFromURL(url, data.FaviconUrl, "icon"), data.FaviconUrl)
			if err != nil {
				log.Errorf("load favicon: %s", err)
				return
			}
			updaters <- func(c *bookmark.ObjectContent) {
				if c.BookmarkContent == nil {
					c.BookmarkContent = &model.BlockContentBookmark{}
				}
				c.BookmarkContent.FaviconHash = hash
			}
		}()
	}

	if parseBlock {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updaters <- func(c *bookmark.ObjectContent) {
				if isFile {
					s.handleFileBlock(c, url)
					return
				}
				blocks, _, err := anymark.HTMLToBlocks(body, url)
				if err != nil {
					log.Errorf("parse blocks: %s", err)
					return
				}
				c.Blocks = blocks
			}
		}()
	}
	go func() {
		wg.Wait()
		close(updaters)
	}()
	return updaters, nil
}

func (s *service) handleFileBlock(c *bookmark.ObjectContent, url string) {
	c.Blocks = append(
		c.Blocks,
		&model.Block{
			Id: bson.NewObjectId().Hex(),
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name: url,
				}},
		},
	)
}

func (s *service) fetcher(spaceID string, blockID string, params bookmark.FetchParams) error {
	updaters, err := s.ContentUpdaters(spaceID, params.Url, false)
	if err != nil {
		log.Errorf("can't get updates for %s: %s", blockID, err)
	}

	upds := make([]func(content *bookmark.ObjectContent), 0, len(updaters))
	for u := range updaters {
		upds = append(upds, u)
	}
	err = params.Updater(blockID, func(bm bookmark.Block) error {
		for _, u := range upds {
			bm.UpdateContent(u)
			// todo: we have title/description of bookmark block deprecated but still update them
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("can't update bookmark data: %w", err)
	}
	return nil
}

func getFileNameFromURL(baseUrl, fileUrl, filename string) string {
	bu, err := uri.ParseURI(baseUrl)
	if err != nil {
		return ""
	}
	if bu.Hostname() == "" {
		return ""
	}
	fu, err := uri.ParseURI(fileUrl)
	if err != nil {
		return ""
	}
	urlFileExt := path.Ext(fu.Path)

	source := strings.TrimPrefix(bu.Hostname(), "www.")
	source = strings.ReplaceAll(source, ".", "_")
	if source != "" {
		source += "_"
	}
	source += filename + urlFileExt
	return source
}

func (s *service) loadImage(spaceId string, title, url string) (hash string, err error) {
	uploader := s.fileUploaderFactory.NewUploader(spaceId, objectorigin.Bookmark())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res := uploader.SetName(title).SetUrl(url).SetImageKind(model.ImageKind_AutomaticallyAdded).Upload(ctx)
	return res.FileObjectId, res.Err
}
