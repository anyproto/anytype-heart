package notion

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/database"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const name = "Notion"

func init() {
	converter.RegisterFunc(New)
}

type Notion struct {
	database *database.DatabaseService
}

func New(core.Service) converter.Converter {
	return &Notion{
		database: database.New(),
	}
}

func (n *Notion) GetSnapshots(req *pb.RpcObjectImportRequest) *converter.Response {
	ce := converter.NewError()
	apiKey := n.getParams(req.Params)
	if apiKey == "" {
		ce.Add("apiKey", fmt.Errorf("failed to extract apikey"))
		return &converter.Response{
			Error: ce,
		}
	}
	return n.database.GetDatabase(context.TODO(), req.Mode, apiKey)
}

func (n *Notion) getParams(param pb.IsRpcObjectImportRequestParams) string {
	if p, ok := param.(*pb.RpcObjectImportRequestParamsOfNotionParams); ok {
		return p.NotionParams.GetApiKey()
	}
	return ""
}

func (n *Notion) Name() string {
	return name
}

