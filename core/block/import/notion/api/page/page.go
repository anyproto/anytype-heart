package page

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/import/workerpool"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var logger = logging.Logger("notion-page")

const (
	ObjectType     = "page"
	pageSize       = 100
	workerPoolSize = 10
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
	notionImportContext *api.NotionImportContext,
	relations *property.PropertiesStore,
	progress process.Progress) (*converter.Response, *converter.ConvertError) {
	progress.SetProgressMessage("Start creating pages from notion")
	convertError := ds.fillNotionImportContext(pages, progress, notionImportContext)
	if convertError != nil {
		return nil, convertError
	}
	numWorkers := workerPoolSize
	if len(pages) < workerPoolSize {
		numWorkers = 1
	}
	pool := workerpool.NewPool(numWorkers)

	go ds.addWorkToPool(pages, pool)

	do := NewDataObject(ctx, apiKey, mode, notionImportContext, relations)
	go pool.Start(do)

	allSnapshots, converterError := ds.readResultFromPool(pool, mode, progress)
	if converterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, converterError
}

func (ds *Service) readResultFromPool(pool *workerpool.WorkerPool, mode pb.RpcObjectImportRequestMode, progress process.Progress) ([]*converter.Snapshot, *converter.ConvertError) {
	allSnapshots := make([]*converter.Snapshot, 0)
	ce := converter.NewError()

	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			pool.Stop()
			return nil, converter.NewCancelError(err)
		}
		res := r.(*Result)
		if res.ce != nil {
			ce.Merge(res.ce)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				pool.Stop()
				return nil, ce
			}
		}
		allSnapshots = append(allSnapshots, res.snapshot...)
	}
	return allSnapshots, ce
}

func (ds *Service) addWorkToPool(pages []Page, pool *workerpool.WorkerPool) {
	var (
		relMutex    = &sync.Mutex{}
		relOptMutex = &sync.Mutex{}
	)
	for _, p := range pages {
		stop := pool.AddWork(&Task{
			relationCreateMutex:    relMutex,
			relationOptCreateMutex: relOptMutex,
			propertyService:        ds.propertyService,
			blockService:           ds.blockService,
			p:                      p,
		})
		if stop {
			break
		}
		time.Sleep(time.Millisecond * 5) // to avoid rate limit error
	}
	pool.CloseTask()
}

func (ds *Service) extractTitleFromPages(page Page) string {
	// Need to collect pages title and notion ids mapping for such blocks as ChildPage and ChildDatabase,
	// because we only get title in those blocks from API
	for _, v := range page.Properties {
		if t, ok := v.(*property.TitleItem); ok {
			return t.GetTitle()
		}
	}
	return ""
}

func (ds *Service) fillNotionImportContext(pages []Page, progress process.Progress, importContext *api.NotionImportContext) *converter.ConvertError {
	for _, p := range pages {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(err)
		}
		importContext.NotionPageIdsToAnytype[p.ID] = uuid.New().String()
		if p.Parent.PageID != "" {
			importContext.PageTree.ParentPageToChildIDs[p.Parent.PageID] = append(importContext.PageTree.ParentPageToChildIDs[p.Parent.PageID], p.ID)
		}
		if p.Parent.DatabaseID != "" {
			importContext.PageTree.ParentPageToChildIDs[p.Parent.DatabaseID] = append(importContext.PageTree.ParentPageToChildIDs[p.Parent.DatabaseID], p.ID)
		}
		if p.Parent.BlockID != "" {
			importContext.PageTree.ParentPageToChildIDs[p.Parent.BlockID] = append(importContext.PageTree.ParentPageToChildIDs[p.Parent.BlockID], p.ID)
		}
		if p.Parent.PageID == "" && p.Parent.DatabaseID == "" && p.Parent.BlockID == "" {
			importContext.PageTree.ParentPageToChildIDs[""] = append(importContext.PageTree.ParentPageToChildIDs[""], p.ID)
		}
		importContext.PageNameToID[p.ID] = ds.extractTitleFromPages(p)
	}
	return nil
}
