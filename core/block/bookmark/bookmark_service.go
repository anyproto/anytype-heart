package bookmark

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

const CName = "bookmark"

// ContentFuture represents asynchronous result of getting bookmark content
type ContentFuture func() *bookmark.ObjectContent

type Service interface {
	CreateObjectAndFetch(ctx context.Context, spaceId string, details *types.Struct) (objectID string, newDetails *types.Struct, err error)
	CreateBookmarkObject(ctx context.Context, spaceId string, details *types.Struct, getContent ContentFuture) (objectId string, newDetails *types.Struct, err error)
	UpdateObject(objectId string, getContent *bookmark.ObjectContent) error
	// TODO Maybe Fetch and FetchBookmarkContent do the same thing differently?
	FetchAsync(spaceID string, blockID string, params bookmark.FetchParams)
	FetchBookmarkContent(spaceID string, url string, parseBlock bool) ContentFuture
	ContentUpdaters(spaceID string, url string, parseBlock bool) (chan func(contentBookmark *bookmark.ObjectContent), error)

	app.Component
}

type ObjectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
}

type DetailsSetter interface {
	SetDetails(ctx session.Context, objectId string, details []*model.Detail) (err error)
}

type service struct {
	detailsSetter       DetailsSetter
	creator             ObjectCreator
	store               objectstore.ObjectStore
	linkPreview         linkpreview.LinkPreview
	tempDirService      core.TempDirProvider
	spaceService        space.Service
	fileUploaderFactory fileuploader.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.detailsSetter = app.MustComponent[DetailsSetter](a)
	s.creator = a.MustComponent("objectCreator").(ObjectCreator)
	s.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.spaceService = app.MustComponent[space.Service](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	s.fileUploaderFactory = app.MustComponent[fileuploader.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

var log = logging.Logger("anytype-mw-bookmark")

func (s *service) CreateObjectAndFetch(
	ctx context.Context, spaceId string, details *types.Struct,
) (objectID string, newDetails *types.Struct, err error) {
	source := pbtypes.GetString(details, bundle.RelationKeySource.String())
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
	return s.CreateBookmarkObject(ctx, spaceId, details, res)
}

func (s *service) CreateBookmarkObject(
	ctx context.Context, spaceID string, details *types.Struct, getContent ContentFuture,
) (objectId string, objectDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("empty details")
	}

	spc, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	typeId, err := spc.GetTypeIdByKey(ctx, bundle.TypeKeyBookmark)
	if err != nil {
		return "", nil, fmt.Errorf("get bookmark type id: %w", err)
	}
	url := pbtypes.GetString(details, bundle.RelationKeySource.String())

	records, err := s.store.SpaceIndex(spaceID).Query(database.Query{
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
			},
		},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySource.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(url),
			},
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeId),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return "", nil, fmt.Errorf("query: %w", err)
	}

	if len(records) > 0 {
		rec := records[0]
		objectId = rec.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
		objectDetails = rec.Details
	} else {
		creationState := state.NewDoc("", nil).(*state.State)
		creationState.SetDetails(details)
		objectId, objectDetails, err = s.creator.CreateSmartBlockFromState(
			ctx,
			spaceID,
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

func detailsFromContent(content *bookmark.ObjectContent) map[string]*types.Value {
	return map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(content.BookmarkContent.Title),
		bundle.RelationKeyDescription.String(): pbtypes.String(content.BookmarkContent.Description),
		bundle.RelationKeySource.String():      pbtypes.String(content.BookmarkContent.Url),
		bundle.RelationKeyPicture.String():     pbtypes.String(content.BookmarkContent.ImageHash),
		bundle.RelationKeyIconImage.String():   pbtypes.String(content.BookmarkContent.FaviconHash),
	}
}

func (s *service) UpdateObject(objectId string, getContent *bookmark.ObjectContent) error {
	detailsMap := detailsFromContent(getContent)

	details := make([]*model.Detail, 0, len(detailsMap))
	for k, v := range detailsMap {
		details = append(details, &model.Detail{
			Key:   k,
			Value: v,
		})
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
			hash, err := s.loadImage(spaceID, data.Title, data.ImageUrl)
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
			hash, err := s.loadImage(spaceID, "", data.FaviconUrl)
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

func (s *service) loadImage(spaceId string, title, url string) (hash string, err error) {
	uploader := s.fileUploaderFactory.NewUploader(spaceId, objectorigin.Bookmark())

	tempDir := s.tempDirService.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: %s", resp.Status)
	}

	tmpFile, err := ioutil.TempFile(tempDir, "anytype_downloaded_file_*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", err
	}

	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		return "", err
	}

	fileName := strings.Split(filepath.Base(url), "?")[0]
	if value := resp.Header.Get("Content-Disposition"); value != "" {
		contentDisposition := strings.Split(value, "filename=")
		if len(contentDisposition) > 1 {
			fileName = strings.Trim(contentDisposition[1], "\"")
		}

	}

	if title != "" {
		fileName = title
	}
	res := uploader.SetName(fileName).SetFile(tmpFile.Name()).SetImageKind(model.ImageKind_AutomaticallyAdded).Upload(ctx)
	return res.FileObjectId, res.Err
}
