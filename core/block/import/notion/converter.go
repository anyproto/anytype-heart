package notion

import (
	"context"
	"errors"
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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func (n *Notion) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, importID string) (*converter.Response, *converter.ConvertError) {
	ce := converter.NewError()
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
		return nil, converter.NewFromError(converter.ErrCancel)
	}
	if len(db) == 0 && len(pages) == 0 {
		return nil, converter.NewFromError(converter.ErrNoObjectsToImport)
	}

	notionImportContext := api.NewNotionImportContext()
	dbSnapshots, relations, dbErr := n.dbService.GetDatabase(context.TODO(), req.Mode, db, progress, notionImportContext)
	if dbErr != nil {
		logger.With("err", dbErr.Error()).Warnf("import from notion db failed")
	}
	if errors.Is(dbErr.GetResultError(req.Type), converter.ErrCancel) {
		return nil, converter.NewFromError(converter.ErrCancel)
	}
	if dbErr != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(dbErr)
		return nil, ce
	}

	pgSnapshots, pgErr := n.pgService.GetPages(ctx, apiKey, req.Mode, pages, notionImportContext, relations, progress)
	if pgErr != nil {
		logger.With("err", pgErr.Error()).Warnf("import from notion pages failed")
	}
	if errors.Is(pgErr.GetResultError(req.Type), converter.ErrCancel) {
		return nil, converter.NewFromError(converter.ErrCancel)
	}
	if pgErr != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(pgErr)
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

	rootCollectionSnapshot, rootObjects, err := n.dbService.AddObjectsToNotionRootCollection(notionImportContext, db, pages)
	if err != nil {
		ce.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, ce
		}
	}
	if rootCollectionSnapshot != nil {
		dbs = append(dbs, rootCollectionSnapshot)
	}

	n.injectImportImportID(dbs, pgs, rootObjects, importID)
	allSnapshots := make([]*converter.Snapshot, 0, len(pgs)+len(dbs))
	allSnapshots = append(allSnapshots, pgs...)
	allSnapshots = append(allSnapshots, dbs...)

	if pgErr != nil {
		ce.Merge(pgErr)
	}

	if dbErr != nil {
		ce.Merge(dbErr)
	}
	if !ce.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, ce
	}

	return &converter.Response{Snapshots: allSnapshots}, nil
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

func (n *Notion) injectImportImportID(dbs []*converter.Snapshot, pages []*converter.Snapshot, rootObjects []string, importID string) {
	rootObjectSet := make(map[string]struct{}, len(rootObjects))
	for _, rootObject := range rootObjects {
		rootObjectSet[rootObject] = struct{}{}
	}

	for _, db := range dbs {
		snapshotID := db.Id
		if _, ok := rootObjectSet[snapshotID]; ok {
			db.Snapshot.Data.Details.Fields[bundle.RelationKeyImportID.String()] = pbtypes.String(importID)
		}
	}

	for _, p := range pages {
		snapshotID := p.Id
		if _, ok := rootObjectSet[snapshotID]; ok {
			p.Snapshot.Data.Details.Fields[bundle.RelationKeyImportID.String()] = pbtypes.String(importID)
		}
	}
}
