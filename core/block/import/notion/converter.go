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
	"github.com/anyproto/anytype-heart/core/session"
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

func (n *Notion) GetSnapshots(ctx session.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	ce := converter.NewError()
	apiKey := n.getParams(req)
	if apiKey == "" {
		ce.Add("apiKey", fmt.Errorf("failed to extract apikey"))
		return nil, ce
	}
	db, pages, err := search.Retry(n.search.Search, retryAmount, retryDelay)(context.TODO(), apiKey, pageSize)
	if err != nil {
		ce.Add("/search", fmt.Errorf("failed to get pages and databases %s", err))
		return nil, ce
	}
	progress.SetTotal(int64(len(db)*numberOfStepsForDatabases+len(pages)*numberOfStepsForPages) + stepForSearch)

	if err = progress.TryStep(1); err != nil {
		return nil, converter.NewFromError("", converter.ErrCancel)
	}
	if len(db) == 0 && len(pages) == 0 {
		return nil, converter.NewFromError("", converter.ErrNoObjectsToImport)
	}

	notionImportContext := block.NewNotionImportContext()
	dbSnapshots, dbErr := n.dbService.GetDatabase(context.TODO(), req.Mode, db, progress, notionImportContext)
	if errors.Is(dbErr.GetResultError(req.Type), converter.ErrCancel) {
		return nil, converter.NewFromError("", converter.ErrCancel)
	}
	if dbErr != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(dbErr)
		return nil, ce
	}

	pgSnapshots, pgErr := n.pgService.GetPages(context.TODO(), apiKey, req.Mode, pages, notionImportContext, progress)
	if errors.Is(pgErr.GetResultError(req.Type), converter.ErrCancel) {
		return nil, converter.NewFromError("", converter.ErrCancel)
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
		ce.Add("", err)
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
