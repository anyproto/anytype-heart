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

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

const CName = "bookmark"

// ContentFuture represents asynchronous result of getting bookmark content
type ContentFuture func() *model.BlockContentBookmark

type Service interface {
	CreateBookmarkObject(details *types.Struct, getContent ContentFuture) (objectId string, newDetails *types.Struct, err error)
	UpdateBookmarkObject(objectId string, getContent ContentFuture) error
	Fetch(id string, params bookmark.FetchParams) (err error)
	ContentUpdaters(url string) (chan func(contentBookmark *model.BlockContentBookmark), error)

	app.Component
}

type ObjectManager interface {
	CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string) (id string, newDetails *types.Struct, err error)
	SetDetails(ctx *session.Context, req pb.RpcObjectSetDetailsRequest) (err error)
}

type service struct {
	objectManager ObjectManager
	store         objectstore.ObjectStore
	linkPreview   linkpreview.LinkPreview
	svc           core.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectManager = a.MustComponent("blockService").(ObjectManager)
	s.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.svc = a.MustComponent(core.CName).(core.Service)
	return nil
}

func (s service) Name() (name string) {
	return CName
}

var log = logging.Logger("anytype-mw-bookmark")

func (s *service) CreateBookmarkObject(details *types.Struct, getContent ContentFuture) (objectId string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("empty details")
	}

	url := pbtypes.GetString(details, bundle.RelationKeySource.String())
	if url == "" {
		return "", nil, fmt.Errorf("source field is empty or not provided")
	}

	records, _, err := s.store.Query(nil, database.Query{
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
			},
		},
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySource.String(),
				Value:       pbtypes.String(url),
			},
		},
		Limit: 1,
		ObjectTypeFilter: []string{
			bundle.TypeKeyBookmark.URL(),
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("query: %w", err)
	}

	if len(records) > 0 {
		rec := records[0]
		objectId = rec.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	} else {
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyBookmark.URL())
		objectId, newDetails, err = s.objectManager.CreateSmartBlock(context.TODO(), coresb.SmartBlockTypePage, details, nil)
		if err != nil {
			return "", nil, fmt.Errorf("create bookmark object: %w", err)
		}
	}

	go func() {
		if err := s.UpdateBookmarkObject(objectId, getContent); err != nil {

			log.Errorf("update bookmark object %s: %s", objectId, err)
			return
		}
	}()

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

	return s.objectManager.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: objectId,
		Details:   details,
	})
}

func (s *service) Fetch(id string, params bookmark.FetchParams) (err error) {
	if !params.Sync {
		go func() {
			if err := s.fetcher(id, params); err != nil {
				log.Errorf("fetch bookmark %s: %s", id, err)
			}
		}()
		return nil
	}

	return s.fetcher(id, params)
}

func (s *service) ContentUpdaters(url string) (chan func(contentBookmark *model.BlockContentBookmark), error) {
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
			hash, err := loadImage(s.svc, data.Title, data.ImageUrl)
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
			hash, err := loadImage(s.svc, "", data.FaviconUrl)
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

func (s *service) fetcher(id string, params bookmark.FetchParams) error {
	updaters, err := s.ContentUpdaters(params.Url)
	if err != nil {
		log.Errorf("can't get updates for %s: %s", id, err)
	}

	var upds []func(*model.BlockContentBookmark)
	for u := range updaters {
		upds = append(upds, u)
	}
	err = params.Updater(id, func(bm bookmark.Block) error {
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

func loadImage(stor core.Service, title, url string) (hash string, err error) {
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

	tmpFile, err := ioutil.TempFile(stor.TempDir(), "anytype_downloaded_file_*")
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

	im, err := stor.ImageAdd(context.TODO(), files.WithReader(tmpFile), files.WithName(fileName))
	if err != nil {
		return
	}
	return im.Hash(), nil
}
