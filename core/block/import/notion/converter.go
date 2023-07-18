package notion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/search"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	name                      = "Notion"
	pageSize                  = 100
	retryDelay                = time.Second
	retryAmount               = 5
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
		ce.Add("", converter.ErrFailedToReceiveListOfObjects)
		logger.With("err", ce.Error()).With("pages", len(pages)).With("dbs", len(db)).Error("import from notion failed")
		return nil, ce
	}
	logger.With("pages", len(pages)).With("dbs", len(db)).Warnf("import from notion started")
	progress.SetTotal(int64(len(db)*numberOfStepsForDatabases+len(pages)*numberOfStepsForPages) + stepForSearch)

	if err = progress.TryStep(1); err != nil {
		return nil, converter.NewFromError(converter.ErrCancel)
	}
	if len(db) == 0 && len(pages) == 0 {
		return nil, converter.NewFromError(converter.ErrNoObjectsToImport)
	}

	notionImportContext := block.NewNotionImportContext()
	dbSnapshots, dbErr := n.dbService.GetDatabase(context.TODO(), req.Mode, db, progress, notionImportContext)
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

	pgSnapshots, pgErr := n.pgService.GetPages(ctx, apiKey, req.Mode, pages, notionImportContext, progress)
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

	dbs, err = n.dbService.AddObjectsToNotionCollection(dbs, pgs)
	if err != nil {
		ce.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, ce
		}
	}

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

func (n *Notion) getParams(param *pb.RpcObjectImportRequest) string {
	if p := param.GetNotionParams(); p != nil {
		return p.GetApiKey()
	}
	return ""
}

func (n *Notion) Name() string {
	return name
}
