package notion

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/search"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
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

func New(c *collection.Service) converter.Converter {
	cl := client.NewClient()
	return &Notion{
		search:    search.New(cl),
		dbService: database.New(c),
		pgService: page.New(cl),
	}
}

func (n *Notion) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	ce := converter.NewError(req.Mode)
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
		ce.Add(fmt.Errorf("failed to get pages and databases %s", err))

		// always add this error because it's mean that we need to return error to user, even in case IGNORE_ERRORS is turned on
		// see shouldReturnError
		ce.Add(converter.ErrFailedToReceiveListOfObjects)
		logger.With("err", ce.Error()).With("pages", len(pages)).With("dbs", len(db)).Error("import from notion failed")
		return nil, ce
	}
	logger.With("pages", len(pages)).With("dbs", len(db)).Warnf("import from notion started")
	allProperties := n.getUniqueProperties(db, pages)
	progress.SetTotal(int64(len(db)*numberOfStepsForDatabases+len(pages)*numberOfStepsForPages+len(allProperties)) + stepForSearch)

	if err = progress.TryStep(1); err != nil {
		return nil, converter.NewFromError(converter.ErrCancel, req.Mode)
	}
	if len(db) == 0 && len(pages) == 0 {
		return nil, converter.NewFromError(converter.ErrNoObjectsToImport, req.Mode)
	}

	notionImportContext := api.NewNotionImportContext()
	dbSnapshots, relations, dbErr := n.dbService.GetDatabase(context.TODO(), req.Mode, db, progress, notionImportContext)
	if dbErr != nil {
		logger.With("err", dbErr.Error()).Warnf("import from notion db failed")
		ce.Merge(dbErr)
	}
	if ce.ShouldAbortImport(0, req.Type) {
		return nil, ce
	}

	pgSnapshots, pgErr := n.pgService.GetPages(ctx, apiKey, req.Mode, pages, notionImportContext, relations, progress)
	if pgErr != nil {
		logger.With("err", pgErr.Error()).Warnf("import from notion pages failed")
		ce.Merge(pgErr)
	}
	if ce.ShouldAbortImport(0, req.Type) {
		return nil, ce
	}

	var pgs, dbs []*converter.Snapshot

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
	allSnapshots := make([]*converter.Snapshot, 0, len(pgs)+len(dbs))
	allSnapshots = append(allSnapshots, pgs...)
	allSnapshots = append(allSnapshots, dbs...)

	if !ce.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, ce
	}

	return &converter.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, nil
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
