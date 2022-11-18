package notion

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/page"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/search"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const (
	name        = "Notion"
	pageSize    = 100
	retryDelay  = time.Second
	retryAmount = 5
)

func init() {
	converter.RegisterFunc(New)
}

type Notion struct {
	search          *search.Service
	databaseService *database.Service
	pageService     *page.Service
}

func New(core.Service) converter.Converter {
	cl := client.NewClient()
	return &Notion{
		search:          search.New(cl),
		databaseService: database.New(),
		pageService:     page.New(cl),
	}
}

func (n *Notion) GetSnapshots(req *pb.RpcObjectImportRequest) *converter.Response {
	ce := converter.NewError()
	apiKey := n.getParams(req)
	if apiKey == "" {
		ce.Add("apiKey", fmt.Errorf("failed to extract apikey"))
		return &converter.Response{
			Error: ce,
		}
	}
	databases, pages, err := search.Retry(n.search.Search, retryAmount, retryDelay)(context.TODO(), apiKey, pageSize)

	if err != nil {
		ce.Add("/search", fmt.Errorf("failed to get pages and databases %s", err))
		return &converter.Response{
			Error: ce,
		}
	}
	databasesSnapshots := n.databaseService.GetDatabase(context.TODO(), req.Mode, databases)
	if databasesSnapshots.Error != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(databasesSnapshots.Error)
		return &converter.Response{
			Error: ce,
		}
	}

	pagesSnapshots := n.pageService.GetPages(context.TODO(), apiKey, req.Mode, pages)
	if pagesSnapshots.Error != nil && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		ce.Merge(pagesSnapshots.Error)
		return &converter.Response{
			Error: ce,
		}
	}

	allSnaphots := make([]*converter.Snapshot, 0, len(pagesSnapshots.Snapshots)+len(databasesSnapshots.Snapshots))
	allSnaphots = append(allSnaphots, pagesSnapshots.Snapshots...)
	allSnaphots = append(allSnaphots, databasesSnapshots.Snapshots...)
	if pagesSnapshots.Error != nil {
		ce.Merge(pagesSnapshots.Error)
	}
	if databasesSnapshots.Error != nil {
		ce.Merge(databasesSnapshots.Error)
	}
	if !ce.IsEmpty() {
		return &converter.Response{
			Snapshots: allSnaphots,
			Error:     ce,
		}
	}
	return &converter.Response{
		Snapshots: allSnaphots,
		Error:     nil,
	}
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
