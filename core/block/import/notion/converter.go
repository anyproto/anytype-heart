package notion

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/page"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/search"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const (
	name                      = "Notion"
	pageSize                  = 100
	retryDelay                = time.Second
	retryAmount               = 5
	numberOfStepsForPages     = 3 // 2 cycles to get snapshots and 1 cycle to create objects
	numberOfStepsForDatabases = 2 // 1 cycles to get snapshots and 1 cycle to create objects
)

func init() {
	converter.RegisterFunc(New)
}

type Notion struct {
	search    *search.Service
	dbService *database.Service
	pgService *page.Service
}

func New(_ core.Service, c *collection.Service) converter.Converter {
	cl := client.NewClient()
	return &Notion{
		search:    search.New(cl),
		dbService: database.New(c),
		pgService: page.New(cl),
	}
}

func (n *Notion) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
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

	progress.SetTotal(int64(len(db)*numberOfStepsForDatabases + len(pages)*numberOfStepsForPages))
	dbSnapshots, notionIdsToAnytype, dbNameToID, dbErr := n.dbService.GetDatabase(context.TODO(), req.Mode, db, progress)

	if dbErr != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(dbErr)
		return nil, ce
	}

	r := &block.MapRequest{
		NotionDatabaseIdsToAnytype: notionIdsToAnytype,
		DatabaseNameToID:           dbNameToID,
	}
	pgSnapshots, notionPageIDToAnytype, pgErr := n.pgService.GetPages(context.TODO(), apiKey, req.Mode, pages, r, progress)
	if pgErr != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(pgErr)
		return nil, ce
	}

	n.dbService.AddPagesToCollections(dbSnapshots, pages, db, notionPageIDToAnytype, notionIdsToAnytype)

	n.dbService.AddPagesToRootCollections(dbSnapshots, pgSnapshots)

	n.dbService.MapProperties(dbSnapshots, pgSnapshots.Relations, pages, db, notionPageIDToAnytype, notionIdsToAnytype)

	allSnapshots := make([]*converter.Snapshot, 0, len(pgSnapshots.Snapshots)+len(dbSnapshots.Snapshots))
	allSnapshots = append(allSnapshots, pgSnapshots.Snapshots...)
	allSnapshots = append(allSnapshots, dbSnapshots.Snapshots...)
	relations := mergeMaps(dbSnapshots.Relations, pgSnapshots.Relations)

	if pgErr != nil {
		ce.Merge(pgErr)
	}

	if dbErr != nil {
		ce.Merge(dbErr)
	}
	if !ce.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Relations: relations}, ce
	}

	return &converter.Response{Snapshots: allSnapshots, Relations: relations}, nil
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

func mergeMaps(first, second map[string][]*converter.Relation) map[string][]*converter.Relation {
	res := make(map[string][]*converter.Relation, 0)

	for pageID, rel := range first {
		res[pageID] = rel
	}

	for pageID, rel := range second {
		res[pageID] = rel
	}

	return res
}
