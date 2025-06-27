package notion

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/search"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	name                      = "Notion"
	pageSize                  = 100
	numberOfStepsForPages     = 4 // 3 cycles to get snapshots and 1 cycle to create objects
	numberOfStepsForDatabases = 2 // 1 cycles to get snapshots and 1 cycle to create objects
	stepForSearch             = 1
)

type Notion struct {
	search    *search.Service
	dbService *database.Service
	pgService *page.Service
}

func New(c *collection.Service) common.Converter {
	cl := client.NewClient()
	return &Notion{
		search:    search.New(cl),
		dbService: database.New(c),
		pgService: page.New(cl),
	}
}

func (n *Notion) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	ce := common.NewError(req.Mode)
	apiKey := n.getParams(req)
	if apiKey == "" {
		ce.Add(fmt.Errorf("failed to extract apikey"))
		return nil, ce
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-progress.Canceled():
			cancel()
		case <-progress.Done():
			cancel()
		}
	}()
	db, pages, err := n.search.Search(ctx, apiKey, pageSize)
	if err != nil {
		ce.Add(fmt.Errorf("failed to get pages and databases %w", err))

		// always add this error because it's mean that we need to return error to user, even in case IGNORE_ERRORS is turned on
		// see shouldReturnError
		log.With("error", ce.Error()).With("pages", len(pages)).With("dbs", len(db)).Error("import from notion failed")
		return nil, ce
	}
	log.With("pages", len(pages)).With("dbs", len(db)).Warnf("import from notion started")
	allProperties := n.getUniqueProperties(db, pages)
	progress.SetTotal(int64(len(db)*numberOfStepsForDatabases+len(pages)*numberOfStepsForPages+len(allProperties)) + stepForSearch)

	if err = progress.TryStep(1); err != nil {
		return nil, common.NewFromError(common.ErrCancel, req.Mode)
	}
	if len(db) == 0 && len(pages) == 0 {
		return nil, common.NewFromError(common.ErrNoObjectInIntegration, req.Mode)
	}

	fileDownloader := files.NewFileDownloader(progress)
	err = fileDownloader.Init(ctx)
	if err != nil {
		return nil, common.NewFromError(err, req.Mode)
	}
	go fileDownloader.ProcessDownloadedFiles()
	defer fileDownloader.StopDownload()
	notionImportContext := api.NewNotionImportContext()
	dbSnapshots, relations, dbErr := n.dbService.GetDatabase(ctx, req.Mode, db, progress, notionImportContext, fileDownloader)
	if dbErr != nil {
		log.With("error", dbErr).Warnf("import from notion db failed")
		ce.Merge(dbErr)
	}
	if ce.ShouldAbortImport(0, req.Type) {
		return nil, ce
	}

	pgSnapshots, pgErr := n.pgService.GetPages(ctx, apiKey, req.Mode, pages, notionImportContext, relations, progress, fileDownloader)
	if pgErr != nil {
		log.With("error", pgErr).Warnf("import from notion pages failed")
		ce.Merge(pgErr)
	}
	if ce.ShouldAbortImport(0, req.Type) {
		return nil, ce
	}

	var pgs, dbs []*common.Snapshot

	if pgSnapshots != nil {
		pgs = pgSnapshots.Snapshots
	}

	if dbSnapshots != nil {
		dbs = dbSnapshots.Snapshots
	}

	n.dbService.AddPagesToCollections(dbs, pages, db, notionImportContext.NotionPageIdsToAnytype, notionImportContext.NotionDatabaseIdsToAnytype)

	rootCollectionSnapshot, err := n.dbService.AddObjectsToNotionCollection(notionImportContext, db, pages)
	if err != nil {
		ce.Add(err)
		if ce.ShouldAbortImport(0, req.Type) {
			return nil, ce
		}
	}
	var rootCollectionID string
	if rootCollectionSnapshot != nil {
		dbs = append(dbs, rootCollectionSnapshot)
		rootCollectionID = rootCollectionSnapshot.Id
	}
	allSnapshots := make([]*common.Snapshot, 0, len(pgs)+len(dbs))
	allSnapshots = append(allSnapshots, pgs...)
	allSnapshots = append(allSnapshots, dbs...)

	if !ce.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots, RootObjectID: rootCollectionID, RootObjectWidgetType: model.BlockContentWidget_CompactList}, ce
	}

	return &common.Response{Snapshots: allSnapshots, RootObjectID: rootCollectionID, RootObjectWidgetType: model.BlockContentWidget_CompactList}, nil
}

func (n *Notion) getUniqueProperties(db []database.Database, pages []page.Page) []string {
	var allProperties []string
	for _, d := range db {
		keys := lo.MapToSlice(d.Properties, func(key string, value property.DatabasePropertyHandler) string { return key })
		uniqueKeys := lo.Filter(keys, func(item string, index int) bool { return !lo.Contains(allProperties, item) })
		allProperties = append(allProperties, uniqueKeys...)
	}

	for _, pg := range pages {
		keys := lo.MapToSlice(pg.Properties, func(key string, value property.Object) string { return key })
		uniqueKeys := lo.Filter(keys, func(item string, index int) bool { return !lo.Contains(allProperties, item) })
		allProperties = append(allProperties, uniqueKeys...)
	}
	return allProperties
}

func (n *Notion) getParams(param *pb.RpcObjectImportRequest) string {
	if p := param.GetNotionParams(); p != nil {
		return p.GetApiKey()
	}
	return ""
}

func (n *Notion) Name() string {
	return name
}
