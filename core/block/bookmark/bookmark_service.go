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
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "bookmark"

// ContentFuture represents asynchronous result of getting bookmark content
type ContentFuture func() *model.BlockContentBookmark

type Service interface {
	CreateBookmarkObject(ctx context.Context, spaceID string, details *types.Struct, getContent ContentFuture) (objectId string, newDetails *types.Struct, err error)
	UpdateBookmarkObject(objectId string, getContent ContentFuture) error
	// TODO Maybe Fetch and FetchBookmarkContent do the same thing differently?
	Fetch(spaceID string, blockID string, params bookmark.FetchParams) (err error)
	FetchBookmarkContent(spaceID string, url string) ContentFuture
	ContentUpdaters(spaceID string, url string) (chan func(contentBookmark *model.BlockContentBookmark), error)

	app.Component
}

type ObjectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
}

type DetailsSetter interface {
	SetDetails(ctx session.Context, req pb.RpcObjectSetDetailsRequest) (err error)
}

type service struct {
	detailsSetter  DetailsSetter
	creator        ObjectCreator
	store          objectstore.ObjectStore
	linkPreview    linkpreview.LinkPreview
	tempDirService core.TempDirProvider
	fileService    files.Service
	coreService    core.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.detailsSetter = a.MustComponent(treemanager.CName).(DetailsSetter)
	s.creator = a.MustComponent("objectCreator").(ObjectCreator)
	s.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.coreService = a.MustComponent(core.CName).(core.Service)

	s.fileService = app.MustComponent[files.Service](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	return nil
}

func (s service) Name() (name string) {
	return CName
}

var log = logging.Logger("anytype-mw-bookmark")

func (s *service) CreateBookmarkObject(ctx context.Context, spaceID string, details *types.Struct, getContent ContentFuture) (objectId string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("empty details")
	}

	typeID := s.coreService.PredefinedObjects(spaceID).SystemTypes[bundle.TypeKeyBookmark]
	url := pbtypes.GetString(details, bundle.RelationKeySource.String())

	records, _, err := s.store.Query(database.Query{
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
				Value:       pbtypes.String(typeID),
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
	} else {
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(typeID)
		objectId, newDetails, err = s.creator.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypePage, details, nil)
		if err != nil {
			return "", nil, fmt.Errorf("create bookmark object: %w", err)
		}
	}

	if url != "" {
		go func() {
			if err := s.UpdateBookmarkObject(objectId, getContent); err != nil {

				log.Errorf("update bookmark object %s: %s", objectId, err)
				return
			}
		}()
	}

	return objectId, newDetails, nil
}

func detailsFromContent(content *model.BlockContentBookmark) map[string]*types.Value {
	return map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(content.Title),
		bundle.RelationKeyDescription.String(): pbtypes.String(content.Description),
		bundle.RelationKeySource.String():      pbtypes.String(content.Url),
		bundle.RelationKeyPicture.String():     pbtypes.String(content.ImageHash),
		bundle.RelationKeyIconImage.String():   pbtypes.String(content.FaviconHash),
	}
}

func (s *service) UpdateBookmarkObject(objectId string, getContent ContentFuture) error {
	detailsMap := detailsFromContent(getContent())

	details := make([]*pb.RpcObjectSetDetailsDetail, 0, len(detailsMap))
	for k, v := range detailsMap {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   k,
			Value: v,
		})
	}

	return s.detailsSetter.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: objectId,
		Details:   details,
	})
}

func (s *service) Fetch(spaceID string, blockID string, params bookmark.FetchParams) (err error) {
	if !params.Sync {
		go func() {
			if err := s.fetcher(spaceID, blockID, params); err != nil {
				log.Errorf("fetch bookmark %s: %s", blockID, err)
			}
		}()
		return nil
	}

	return s.fetcher(spaceID, blockID, params)
}

func (s *service) FetchBookmarkContent(spaceID string, url string) ContentFuture {
	contentCh := make(chan *model.BlockContentBookmark, 1)
	go func() {
		defer close(contentCh)

		content := &model.BlockContentBookmark{
			Url: url,
		}
		updaters, err := s.ContentUpdaters(spaceID, url)
		if err != nil {
			log.Errorf("fetch bookmark content %s: %s", url, err)
		}
		for upd := range updaters {
			upd(content)
		}
		contentCh <- content
	}()

	return func() *model.BlockContentBookmark {
		return <-contentCh
	}
}

func (s *service) ContentUpdaters(spaceID string, url string) (chan func(contentBookmark *model.BlockContentBookmark), error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	updaters := make(chan func(contentBookmark *model.BlockContentBookmark), 1)

	data, err := s.linkPreview.Fetch(ctx, url)
	if err != nil {
		updaters <- func(c *model.BlockContentBookmark) {
			c.State = model.BlockContentBookmark_Error
			c.Url = url
		}
		close(updaters)
		return updaters, fmt.Errorf("bookmark: can't fetch link %s: %w", url, err)
	}

	updaters <- func(c *model.BlockContentBookmark) {
		c.State = model.BlockContentBookmark_Done
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		updaters <- func(c *model.BlockContentBookmark) {
			c.Url = data.Url
			c.Title = data.Title
			c.Description = data.Description
			c.Type = data.Type
		}
	}()

	if data.ImageUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := loadImage(spaceID, s.fileService, s.tempDirService.TempDir(), data.Title, data.ImageUrl)
			if err != nil {
				log.Errorf("can't load image url %s: %s", data.ImageUrl, err)
				return
			}
			updaters <- func(c *model.BlockContentBookmark) {
				c.ImageHash = hash
			}
		}()
	}
	if data.FaviconUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := loadImage(spaceID, s.fileService, s.tempDirService.TempDir(), "", data.FaviconUrl)
			if err != nil {
				log.Errorf("can't load favicon url %s: %s", data.FaviconUrl, err)
				return
			}
			updaters <- func(c *model.BlockContentBookmark) {
				c.FaviconHash = hash
			}
		}()
	}

	go func() {
		wg.Wait()
		close(updaters)
	}()
	return updaters, nil
}

func (s *service) fetcher(spaceID string, blockID string, params bookmark.FetchParams) error {
	updaters, err := s.ContentUpdaters(spaceID, params.Url)
	if err != nil {
		log.Errorf("can't get updates for %s: %s", blockID, err)
	}

	var upds []func(*model.BlockContentBookmark)
	for u := range updaters {
		upds = append(upds, u)
	}
	err = params.Updater(blockID, func(bm bookmark.Block) error {
		for _, u := range upds {
			bm.UpdateContent(u)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("can't update bookmark data: %w", err)
	}
	return nil
}

func loadImage(spaceID string, fileService files.Service, tempDir string, title, url string) (hash string, err error) {
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
		return "", fmt.Errorf("can't download '%s': %s", url, resp.Status)
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

	im, err := fileService.ImageAdd(context.Background(), spaceID, files.WithReader(tmpFile), files.WithName(fileName))
	if err != nil {
		return
	}
	return im.Hash(), nil
}
