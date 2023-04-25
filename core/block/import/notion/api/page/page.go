package page

import (
	"context"
	"github.com/google/uuid"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var logger = logging.Logger("notion-page")

const (
	ObjectType     = "page"
	pageSize       = 100
	workerPoolSize = 5
)

type Service struct {
	blockService    *block.Service
	client          *client.Client
	propertyService *property.Service
}

// New is a constructor for Service
func New(client *client.Client) *Service {
	return &Service{
		blockService:    block.New(client),
		client:          client,
		propertyService: property.New(client),
	}
}

// Page represents Page object from notion https://developers.notion.com/reference/page
type Page struct {
	Object         string              `json:"object"`
	ID             string              `json:"id"`
	CreatedTime    string              `json:"created_time"`
	LastEditedTime string              `json:"last_edited_time"`
	CreatedBy      api.User            `json:"created_by,omitempty"`
	LastEditedBy   api.User            `json:"last_edited_by,omitempty"`
	Parent         api.Parent          `json:"parent"`
	Properties     property.Properties `json:"properties"`
	Archived       bool                `json:"archived"`
	Icon           *api.Icon           `json:"icon,omitempty"`
	Cover          *api.FileObject     `json:"cover,omitempty"`
	URL            string              `json:"url,omitempty"`
}

func (p *Page) GetObjectType() string {
	return ObjectType
}

// GetPages transform Page objects from Notion to snaphots
func (ds *Service) GetPages(ctx context.Context,
	apiKey string,
	mode pb.RpcObjectImportRequestMode,
	pages []Page,
	request *block.MapRequest,
	progress process.Progress) (*converter.Response, map[string]string, converter.ConvertError) {
	var (
		notionPagesIdsToAnytype = make(map[string]string, 0)
	)

	progress.SetProgressMessage("Start creating pages from notion")

	convertError := ds.createIDsForPages(pages, progress, notionPagesIdsToAnytype)
	if convertError != nil {
		return nil, nil, convertError
	}

	progress.SetProgressMessage("Start creating blocks")
	request.PageNameToID = ds.extractTitleFromPages(pages)
	request.NotionPageIdsToAnytype = notionPagesIdsToAnytype
	pool := NewPool(workerPoolSize, len(pages))

	wg := &sync.WaitGroup{}
	ds.addWorkToPool(pages, pool, wg)
	pool.Start(ctx, apiKey, mode, request, progress)
	wg.Wait()

	converterError := pool.ConvertError()
	allSnapshots := pool.AllSnapshots()
	relationsToPageID := pool.RelationsToPageID()
	if converterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Relations: relationsToPageID}, notionPagesIdsToAnytype, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, notionPagesIdsToAnytype, converterError
}

func (ds *Service) addWorkToPool(pages []Page, pool *WorkerPool, wg *sync.WaitGroup) {
	for _, p := range pages {
		pool.AddWork(&Task{
			propertyService: ds.propertyService,
			blockService:    ds.blockService,
			p:               p,
			wg:              wg,
		})
		wg.Add(1)
	}
}

func (ds *Service) extractTitleFromPages(pages []Page) map[string]string {
	// Need to collect pages title and notion ids mapping for such blocks as ChildPage and ChildDatabase,
	// because we only get title in those blocks from API
	pageNameToID := make(map[string]string, 0)
	for _, p := range pages {
		for _, v := range p.Properties {
			if t, ok := v.(*property.TitleItem); ok {
				pageNameToID[p.ID] = t.GetTitle()
			}
		}
	}
	return pageNameToID
}

func (ds *Service) createIDsForPages(pages []Page, progress *process.Progress, notionPagesIdsToAnytype map[string]string) converter.ConvertError {
	for _, p := range pages {
		if err := progress.TryStep(1); err != nil {
			ce := converter.NewFromError(p.ID, err)
			return ce
		}

		notionPagesIdsToAnytype[p.ID] = uuid.New().String()
	}
	return nil
}
